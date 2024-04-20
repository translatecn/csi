/*
Copyright 2018 The Kubernetes Authors.

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

package csi_resizer

import (
	"context"
	"flag"
	"k8s.io/utils/pointer"
	"net/http"
	"strings"
	"time"

	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"local/3rd/external-resizer/pkg/csi"

	"k8s.io/client-go/util/workqueue"

	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	csitrans "k8s.io/csi-translation-lib"
	"local/3rd/external-resizer/pkg/controller"
	"local/3rd/external-resizer/pkg/features"
	"local/3rd/external-resizer/pkg/modifier"
	"local/3rd/external-resizer/pkg/modifycontroller"
	"local/3rd/external-resizer/pkg/resizer"
	"local/3rd/external-resizer/pkg/util"

	"k8s.io/apimachinery/pkg/util/wait"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	cflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"

	"k8s.io/component-base/featuregate"
	logsapi "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
)

var (
	master                      = pointer.String("")
	kubeConfig                  = pointer.String("")
	resyncPeriod                = pointer.Duration(time.Minute * 10)
	workers                     = pointer.Int(10)
	csiAddress                  = pointer.String("/csi/csi.sock")
	timeout                     = pointer.Duration(10 * time.Second)
	showVersion                 = pointer.Bool(false)
	retryIntervalStart          = pointer.Duration(time.Second)
	retryIntervalMax            = pointer.Duration(5 * time.Minute)
	enableLeaderElection        = pointer.Bool(false)
	leaderElectionNamespace     = pointer.String("")
	leaderElectionLeaseDuration = pointer.Duration(15 * time.Second)
	leaderElectionRenewDeadline = pointer.Duration(10 * time.Second)
	leaderElectionRetryPeriod   = pointer.Duration(5 * time.Second)
	metricsAddress              = pointer.String("")
	httpEndpoint                = pointer.String("")
	metricsPath                 = pointer.String("/metrics")
	kubeAPIQPS                  = pointer.Float64(5)
	kubeAPIBurst                = pointer.Int(10)
	handleVolumeInUseError      = pointer.Bool(true)
	featureGates                map[string]bool

	fg      = featuregate.NewFeatureGate()
	c       = logsapi.NewLoggingConfiguration()
	version = "unknown"
)

func init() {
	flag.Var(cflag.NewMapStringBool(&featureGates), "feature-gates", "A set of key=value paris that describe feature gates for alpha/experimental features for csi external resizer."+"Options are:\n"+strings.Join(utilfeature.DefaultFeatureGate.KnownFeatures(), "\n"))
	logsapi.AddFeatureGates(fg)
}
func Main() {

	if err := logsapi.ValidateAndApply(c, fg); err != nil {
		klog.ErrorS(err, "LoggingConfiguration is invalid")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	if *metricsAddress != "" && *httpEndpoint != "" {
		klog.ErrorS(nil, "Only one of `--metrics-address` and `--http-endpoint` can be set.")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
	addr := *metricsAddress
	if addr == "" {
		addr = *httpEndpoint
	}
	if err := utilfeature.DefaultMutableFeatureGate.SetFromMap(featureGates); err != nil {
		klog.ErrorS(err, "Failed to set feature gates")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	var config *rest.Config
	var err error
	if *master != "" || *kubeConfig != "" {
		config, err = clientcmd.BuildConfigFromFlags(*master, *kubeConfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		klog.ErrorS(err, "Failed to create cluster config")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	config.QPS = float32(*kubeAPIQPS)
	config.Burst = *kubeAPIBurst

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.ErrorS(err, "Failed to create kube client")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	informerFactory := informers.NewSharedInformerFactory(kubeClient, *resyncPeriod)

	mux := http.NewServeMux()

	metricsManager := metrics.NewCSIMetricsManager("" /* driverName */)

	csiClient, err := csi.New(*csiAddress, *timeout, metricsManager)
	if err != nil {
		klog.ErrorS(err, "Failed to create CSI client")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	driverName, err := getDriverName(csiClient, *timeout)
	if err != nil {
		klog.ErrorS(err, "Get driver name failed")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
	klog.V(2).InfoS("CSI driver name", "driverName", driverName)

	translator := csitrans.New()
	if translator.IsMigratedCSIDriverByName(driverName) {
		metricsManager = metrics.NewCSIMetricsManagerWithOptions(driverName, metrics.WithMigration())
		migratedCsiClient, err := csi.New(*csiAddress, *timeout, metricsManager)
		if err != nil {
			klog.ErrorS(err, "Failed to create MigratedCSI client")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
		csiClient.CloseConnection()
		csiClient = migratedCsiClient
	}

	csiResizer, err := resizer.NewResizerFromClient(
		csiClient,
		*timeout,
		kubeClient,
		driverName)
	if err != nil {
		klog.ErrorS(err, "Failed to create CSI resizer")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	csiModifier, err := modifier.NewModifierFromClient(
		csiClient,
		*timeout,
		kubeClient,
		informerFactory,
		driverName)
	if err != nil {
		klog.ErrorS(err, "Failed to create CSI modifier")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	// Start HTTP server for metrics + leader election healthz
	if addr != "" {
		metricsManager.RegisterToServer(mux, *metricsPath)
		metricsManager.SetDriverName(driverName)
		go func() {
			klog.InfoS("ServeMux listening", "address", addr)
			err := http.ListenAndServe(addr, mux)
			if err != nil {
				klog.ErrorS(err, "Failed to start HTTP server", "address", addr, "metricsPath", *metricsPath)
				klog.FlushAndExit(klog.ExitFlushTimeout, 1)
			}
		}()
	}

	resizerName := csiResizer.Name()
	rc := controller.NewResizeController(resizerName, csiResizer, kubeClient, *resyncPeriod, informerFactory,
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		*handleVolumeInUseError)
	modifierName := csiModifier.Name()
	var mc modifycontroller.ModifyController
	// Add modify controller only if the feature gate is enabled
	if utilfeature.DefaultFeatureGate.Enabled(features.VolumeAttributesClass) {
		mc = modifycontroller.NewModifyController(modifierName, csiModifier, kubeClient, *resyncPeriod, informerFactory,
			workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax))
	}

	run := func(ctx context.Context) {
		informerFactory.Start(wait.NeverStop)
		go rc.Run(*workers, ctx)
		if utilfeature.DefaultFeatureGate.Enabled(features.VolumeAttributesClass) {
			go mc.Run(*workers, ctx)
		}
		<-ctx.Done()
	}

	if !*enableLeaderElection {
		run(context.TODO())
	} else {
		lockName := "external-resizer-" + util.SanitizeName(resizerName)
		leKubeClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			klog.ErrorS(err, "Failed to create leKubeClient")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
		le := leaderelection.NewLeaderElection(leKubeClient, lockName, run)
		if *httpEndpoint != "" {
			le.PrepareHealthCheck(mux, leaderelection.DefaultHealthCheckTimeout)
		}

		if *leaderElectionNamespace != "" {
			le.WithNamespace(*leaderElectionNamespace)
		}

		le.WithLeaseDuration(*leaderElectionLeaseDuration)
		le.WithRenewDeadline(*leaderElectionRenewDeadline)
		le.WithRetryPeriod(*leaderElectionRetryPeriod)

		if err := le.Run(); err != nil {
			klog.ErrorS(err, "Error initializing leader election")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
	}
}

func getDriverName(client csi.Client, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.GetDriverName(ctx)
}
