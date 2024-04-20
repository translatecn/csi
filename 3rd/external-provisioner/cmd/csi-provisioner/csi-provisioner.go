/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package csi_provisioner

import (
	"context"
	flag "github.com/spf13/pflag"
	"k8s.io/utils/pointer"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	storagelistersv1 "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-base/metrics/legacyregistry"
	_ "k8s.io/component-base/metrics/prometheus/clientgo/leaderelection" // register leader election in the default legacy registry
	_ "k8s.io/component-base/metrics/prometheus/workqueue"               // register work queues in the default legacy registry
	csitrans "k8s.io/csi-translation-lib"
	klog "k8s.io/klog/v2"
	"local/3rd/external-provisioner/pkg/capacity"
	"local/3rd/external-provisioner/pkg/capacity/topology"
	ctrl "local/3rd/external-provisioner/pkg/controller"
	"local/3rd/external-provisioner/pkg/features"
	"local/3rd/external-provisioner/pkg/owner"
	snapclientset "local/3rd/external-snapshotter/client/clientset/versioned"
	gatewayclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gatewayInformers "sigs.k8s.io/gateway-api/pkg/client/informers/externalversions"
	referenceGrantv1beta1 "sigs.k8s.io/gateway-api/pkg/client/listers/apis/v1beta1"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/v8/controller"
)

var (
	master                      = pointer.String("")
	kubeconfig                  = flag.StringP("kubeconfig", "c", "", "")
	csiEndpoint                 = pointer.String("/csi/csi.sock")
	volumeNamePrefix            = pointer.String("pvc")
	volumeNameUUIDLength        = pointer.Int(-1)
	showVersion                 = pointer.Bool(false)
	retryIntervalStart          = pointer.Duration(time.Second)
	retryIntervalMax            = pointer.Duration(5 * time.Minute)
	workerThreads               = pointer.Uint(100)
	finalizerThreads            = pointer.Uint(1)
	capacityThreads             = pointer.Uint(1)
	operationTimeout            = pointer.Duration(10 * time.Second)
	enableLeaderElection        = pointer.Bool(false)
	leaderElectionNamespace     = pointer.String("")
	strictTopology              = pointer.Bool(false)
	immediateTopology           = pointer.Bool(true)
	extraCreateMetadata         = pointer.Bool(false)
	metricsAddress              = pointer.String("")
	httpEndpoint                = pointer.String("")
	metricsPath                 = pointer.String("/metrics")
	enableProfile               = pointer.Bool(false)
	leaderElectionLeaseDuration = pointer.Duration(15 * time.Second)
	leaderElectionRenewDeadline = pointer.Duration(10 * time.Second)
	leaderElectionRetryPeriod   = pointer.Duration(5 * time.Second)
	defaultFSType               = pointer.String("")
	kubeAPIQPS                  = pointer.Float32(5)
	kubeAPIBurst                = pointer.Int(10)
	kubeAPICapacityQPS          = pointer.Float32(1)
	kubeAPICapacityBurst        = pointer.Int(5)
	enableCapacity              = pointer.Bool(true)
	capacityImmediateBinding    = pointer.Bool(false)
	capacityPollInterval        = pointer.Duration(time.Minute)
	capacityOwnerrefLevel       = pointer.Int(2)
	controllerPublishReadOnly   = pointer.Bool(false)
	preventVolumeModeConversion = pointer.Bool(true)
	provisionController         *controller.ProvisionController
)

func Main() {
	time.Sleep(time.Second * 5)
	var config *rest.Config
	var err error

	ctx := context.Background()

	if *metricsAddress != "" && *httpEndpoint != "" {
		klog.Error("only one of `--metrics-address` and `--http-endpoint` can be set.")
		return
	}
	addr := *metricsAddress
	if addr == "" {
		addr = *httpEndpoint
	}

	// get the KUBECONFIG from env if specified (useful for local/debug cluster)
	kubeconfigEnv := os.Getenv("KUBECONFIG")

	if kubeconfigEnv != "" {
		klog.Infof("Found KUBECONFIG environment variable set, using that..")
		kubeconfig = &kubeconfigEnv
	}

	if *master != "" || *kubeconfig != "" {
		klog.Infof("Either master or kubeconfig specified. building kube config from that..")
		config, err = clientcmd.BuildConfigFromFlags(*master, *kubeconfig)
	} else {
		klog.Infof("Building kube configs for running in cluster...")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		klog.Fatalf("Failed to create config: %v", err)
	}

	config.QPS = *kubeAPIQPS
	config.Burst = *kubeAPIBurst

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create client: %v", err)
	}

	// snapclientset.NewForConfig creates a new Clientset for  VolumesnapshotV1Client
	snapClient, err := snapclientset.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create snapshot client: %v", err)
	}

	var gatewayClient gatewayclientset.Interface
	if utilfeature.DefaultFeatureGate.Enabled(features.CrossNamespaceVolumeDataSource) {
		// gatewayclientset.NewForConfig creates a new Clientset for GatewayClient
		gatewayClient, err = gatewayclientset.NewForConfig(config)
		if err != nil {
			klog.Fatalf("Failed to create gateway client: %v", err)
		}
	}

	metricsManager := metrics.NewCSIMetricsManagerWithOptions("", /* driverName */
		// Will be provided via default gatherer.
		metrics.WithProcessStartTime(false),
		metrics.WithSubsystem(metrics.SubsystemSidecar),
	)

	grpcClient, err := ctrl.Connect(*csiEndpoint, metricsManager)
	if err != nil {
		klog.Error(err.Error())
		return
	}

	err = ctrl.Probe(grpcClient, *operationTimeout)
	if err != nil {
		klog.Error(err.Error())
		return
	}

	// Autodetect provisioner name
	provisionerName, err := ctrl.GetDriverName(grpcClient, *operationTimeout)
	if err != nil {
		klog.Errorf("Error getting CSI driver name: %s", err)
		return
	}
	klog.V(2).Infof("Detected CSI driver %s", provisionerName)
	metricsManager.SetDriverName(provisionerName)

	translator := csitrans.New()
	supportsMigrationFromInTreePluginName := ""
	if translator.IsMigratedCSIDriverByName(provisionerName) { // k8s 内置的
		supportsMigrationFromInTreePluginName, err = translator.GetInTreeNameFromCSIName(provisionerName)
		if err != nil {
			klog.Fatalf("Failed to get InTree plugin name for migrated CSI plugin %s: %v", provisionerName, err)
		}
		klog.V(2).Infof("Supports migration from in-tree plugin: %s", supportsMigrationFromInTreePluginName)

		// Create a new connection with the metrics manager with migrated label
		metricsManager = metrics.NewCSIMetricsManagerWithOptions(provisionerName,
			// Will be provided via default gatherer.
			metrics.WithProcessStartTime(false),
			metrics.WithMigration())
		migratedGrpcClient, err := ctrl.Connect(*csiEndpoint, metricsManager)
		if err != nil {
			klog.Error(err.Error())
			return
		}
		grpcClient.Close()
		grpcClient = migratedGrpcClient

		err = ctrl.Probe(grpcClient, *operationTimeout)
		if err != nil {
			klog.Error(err.Error())
			return
		}
	}

	pluginCapabilities, controllerCapabilities, err := ctrl.GetDriverCapabilities(grpcClient, *operationTimeout)
	if err != nil {
		klog.Fatalf("Error getting CSI driver capabilities: %s", err)
	}

	// Generate a unique ID for this provisioner
	timeStamp := time.Now().UnixNano() / int64(time.Millisecond)
	identity := strconv.FormatInt(timeStamp, 10) + "-" + strconv.Itoa(rand.Intn(10000)) + "-" + provisionerName

	factory := informers.NewSharedInformerFactory(clientset, ctrl.ResyncPeriodOfCsiNodeInformer)
	var factoryForNamespace informers.SharedInformerFactory // usually nil, only used for CSIStorageCapacity

	// -------------------------------
	// Listers
	// Create informer to prevent hit the API server for all resource request
	scLister := factory.Storage().V1().StorageClasses().Lister()
	claimLister := factory.Core().V1().PersistentVolumeClaims().Lister()

	var vaLister storagelistersv1.VolumeAttachmentLister
	if controllerCapabilities[csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME] {
		klog.Info("CSI driver supports PUBLISH_UNPUBLISH_VOLUME, watching VolumeAttachments")
		vaLister = factory.Storage().V1().VolumeAttachments().Lister()
	} else {
		klog.Info("CSI driver does not support PUBLISH_UNPUBLISH_VOLUME, not watching VolumeAttachments")
	}

	var nodeLister listersv1.NodeLister
	var csiNodeLister storagelistersv1.CSINodeLister
	if ctrl.SupportsTopology(pluginCapabilities) {
		csiNodeLister = factory.Storage().V1().CSINodes().Lister()
		nodeLister = factory.Core().V1().Nodes().Lister()
	}

	var referenceGrantLister referenceGrantv1beta1.ReferenceGrantLister
	var gatewayFactory gatewayInformers.SharedInformerFactory
	if utilfeature.DefaultFeatureGate.Enabled(features.CrossNamespaceVolumeDataSource) { // pvc 存储 是否支持跨 namespace
		gatewayFactory = gatewayInformers.NewSharedInformerFactory(gatewayClient, ctrl.ResyncPeriodOfReferenceGrantInformer)
		referenceGrants := gatewayFactory.Gateway().V1beta1().ReferenceGrants()
		referenceGrantLister = referenceGrants.Lister()
	}

	// -------------------------------
	// PersistentVolumeClaims informer
	rateLimiter := workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax)
	claimQueue := workqueue.NewNamedRateLimitingQueue(rateLimiter, "claims")
	claimInformer := factory.Core().V1().PersistentVolumeClaims().Informer()

	// Setup options
	provisionerOptions := []func(*controller.ProvisionController) error{
		controller.LeaderElection(false), // Always disable leader election in provisioner lib. Leader election should be done here in the CSI provisioner level instead.
		controller.FailedProvisionThreshold(0),
		controller.FailedDeleteThreshold(0),
		controller.RateLimiter(rateLimiter),
		controller.Threadiness(int(*workerThreads)),
		controller.CreateProvisionedPVLimiter(workqueue.DefaultControllerRateLimiter()),
		controller.ClaimsInformer(claimInformer),
		controller.NodesLister(nodeLister),
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.HonorPVReclaimPolicy) {
		provisionerOptions = append(provisionerOptions, controller.AddFinalizer(true))
	}

	if supportsMigrationFromInTreePluginName != "" {
		provisionerOptions = append(provisionerOptions, controller.AdditionalProvisionerNames([]string{supportsMigrationFromInTreePluginName}))
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	csiProvisioner := ctrl.NewCSIProvisioner(
		clientset,
		*operationTimeout,
		identity,
		*volumeNamePrefix,
		*volumeNameUUIDLength,
		grpcClient,
		snapClient,
		provisionerName,
		pluginCapabilities,
		controllerCapabilities,
		supportsMigrationFromInTreePluginName,
		*strictTopology,
		*immediateTopology,
		translator,
		scLister,
		csiNodeLister,
		nodeLister,
		claimLister,
		vaLister,
		referenceGrantLister,
		*extraCreateMetadata,
		*defaultFSType,
		nil,
		*controllerPublishReadOnly,
		*preventVolumeModeConversion,
	)

	var capacityController *capacity.Controller
	if *enableCapacity {
		// Publishing storage capacity information uses its own client
		// with separate rate limiting.
		config.QPS = *kubeAPICapacityQPS
		config.Burst = *kubeAPICapacityBurst
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			klog.Fatalf("Failed to create client: %v", err)
		}

		namespace := os.Getenv("NAMESPACE")
		if namespace == "" {
			klog.Fatal("need NAMESPACE env variable for CSIStorageCapacity objects")
		}
		var controller *metav1.OwnerReference
		if *capacityOwnerrefLevel >= 0 {
			podName := os.Getenv("POD_NAME")
			if podName == "" {
				klog.Fatal("need POD_NAME env variable to determine CSIStorageCapacity owner")
			}
			var err error
			controller, err = owner.Lookup(config, namespace, podName,
				schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				}, *capacityOwnerrefLevel)
			if err != nil {
				klog.Fatalf("look up owner(s) of pod: %v", err)
			}
			klog.Infof("using %s/%s %s as owner of CSIStorageCapacity objects", controller.APIVersion, controller.Kind, controller.Name)
		}

		var topologyInformer topology.Informer
		topologyInformer = topology.NewNodeTopology(
			provisionerName,
			clientset,
			factory.Core().V1().Nodes(),
			factory.Storage().V1().CSINodes(),
			workqueue.NewNamedRateLimitingQueue(rateLimiter, "csitopology"),
		)
		go topologyInformer.RunWorker(ctx)

		managedByID := "external-provisioner"

		// We only need objects from our own namespace. The normal factory would give
		// us an informer for the entire cluster. We can further restrict the
		// watch to just those objects with the right labels.
		factoryForNamespace = informers.NewSharedInformerFactoryWithOptions(clientset,
			ctrl.ResyncPeriodOfCsiNodeInformer,
			informers.WithNamespace(namespace),
			informers.WithTweakListOptions(func(lo *metav1.ListOptions) {
				lo.LabelSelector = labels.Set{
					capacity.DriverNameLabel: provisionerName,
					capacity.ManagedByLabel:  managedByID,
				}.AsSelector().String()
			}),
		)

		// We use the V1 CSIStorageCapacity API if available.
		clientFactory := capacity.NewV1ClientFactory(clientset)
		cInformer := factoryForNamespace.Storage().V1().CSIStorageCapacities()

		// This invalid object is used in a v1 Create call to determine
		// based on the resulting error whether the v1 API is supported.
		invalidCapacity := &storagev1.CSIStorageCapacity{
			ObjectMeta: metav1.ObjectMeta{
				Name: "#%123-invalid-name",
			},
		}
		createdCapacity, err := clientset.StorageV1().CSIStorageCapacities(namespace).Create(ctx, invalidCapacity, metav1.CreateOptions{})
		switch {
		case err == nil:
			klog.Fatalf("creating an invalid v1.CSIStorageCapacity didn't fail as expected, got: %s", createdCapacity)
		case apierrors.IsNotFound(err):
			// We need to bridge between the v1beta1 API on the
			// server and the v1 API expected by the capacity code.
			klog.Info("using the CSIStorageCapacity v1beta1 API")
			clientFactory = capacity.NewV1beta1ClientFactory(clientset)
			cInformer = capacity.NewV1beta1InformerBridge(factoryForNamespace.Storage().V1beta1().CSIStorageCapacities())
		case apierrors.IsInvalid(err):
			klog.Info("using the CSIStorageCapacity v1 API")
		default:
			klog.Fatalf("unexpected error when checking for the V1 CSIStorageCapacity API: %v", err)
		}

		capacityController = capacity.NewCentralCapacityController(
			csi.NewControllerClient(grpcClient),
			provisionerName,
			clientFactory,
			// Metrics for the queue is available in the default registry.
			workqueue.NewNamedRateLimitingQueue(rateLimiter, "csistoragecapacity"),
			controller,
			managedByID,
			namespace,
			topologyInformer,
			factory.Storage().V1().StorageClasses(),
			cInformer,
			*capacityPollInterval,
			*capacityImmediateBinding,
			*operationTimeout,
		)
		legacyregistry.CustomMustRegister(capacityController)

		// Wrap Provision and Delete to detect when it is time to refresh capacity.
		csiProvisioner = capacity.NewProvisionWrapper(csiProvisioner, capacityController)
	}

	provisionController = controller.NewProvisionController(
		clientset,
		provisionerName,
		csiProvisioner,
		provisionerOptions...,
	)

	csiClaimController := ctrl.NewCloningProtectionController(
		clientset,
		claimLister,
		claimInformer,
		claimQueue,
		controllerCapabilities,
	)

	run := func(ctx context.Context) {
		factory.Start(ctx.Done())
		if factoryForNamespace != nil {
			// Starting is enough, the capacity controller will
			// wait for sync.
			factoryForNamespace.Start(ctx.Done())
		}
		cacheSyncResult := factory.WaitForCacheSync(ctx.Done())
		for _, v := range cacheSyncResult {
			if !v {
				klog.Fatalf("Failed to sync Informers!")
			}
		}

		if utilfeature.DefaultFeatureGate.Enabled(features.CrossNamespaceVolumeDataSource) {
			if gatewayFactory != nil {
				gatewayFactory.Start(ctx.Done())
			}
			gatewayCacheSyncResult := gatewayFactory.WaitForCacheSync(ctx.Done())
			for _, v := range gatewayCacheSyncResult {
				if !v {
					klog.Fatalf("Failed to sync Informers for gateway!")
				}
			}
		}

		if capacityController != nil {
			go capacityController.Run(ctx, int(*capacityThreads))
		}
		if csiClaimController != nil {
			go csiClaimController.Run(ctx, int(*finalizerThreads))
		}
		provisionController.Run(ctx)
	}

	if !*enableLeaderElection {
		run(ctx)
	} else {
		// this lock name pattern is also copied from sigs.k8s.io/sig-storage-lib-external-provisioner/controller
		// to preserve backwards compatibility
		lockName := strings.Replace(provisionerName, "/", "-", -1)

		// create a new clientset for leader election
		leClientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			klog.Fatalf("Failed to create leaderelection client: %v", err)
		}

		le := leaderelection.NewLeaderElection(leClientset, lockName, run)

		if *leaderElectionNamespace != "" {
			le.WithNamespace(*leaderElectionNamespace)
		}

		le.WithLeaseDuration(*leaderElectionLeaseDuration)
		le.WithRenewDeadline(*leaderElectionRenewDeadline)
		le.WithRetryPeriod(*leaderElectionRetryPeriod)
		le.WithIdentity(identity)

		if err := le.Run(); err != nil {
			klog.Fatalf("failed to initialize leader election: %v", err)
		}
	}
}
