#CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/hostpathplugin main.go

#docker build -t registry.cn-hangzhou.aliyuncs.com/acejilam/hostpathplugin:v1 .

#docker exec koord-control-plane mv /etc/kubernetes/manifests/kube-scheduler.yaml          /tmp/kube-scheduler.yaml
#docker exec koord-control-plane mv /etc/kubernetes/manifests/kube-controller-manager.yaml /tmp/kube-controller-manager.yaml


#docker pull k8s.m.daocloud.io/sig-storage/csi-provisioner:v4.0.0
#docker pull k8s.m.daocloud.io/sig-storage/csi-attacher:v4.5.0
#docker pull k8s.m.daocloud.io/sig-storage/csi-resizer:v1.10.0
#docker pull k8s.m.daocloud.io/sig-storage/livenessprobe:v2.12.0
#docker pull registry.cn-hangzhou.aliyuncs.com/acejilam/mygo:v1.21.5

#kind load docker-image -n koord  registry.cn-hangzhou.aliyuncs.com/acejilam/mygo:v1.21.5
#kind load docker-image -n koord  registry.cn-hangzhou.aliyuncs.com/acejilam/hostpathplugin:v1
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/csi-provisioner:v4.0.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/csi-attacher:v4.5.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/csi-resizer:v1.10.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/livenessprobe:v2.12.0


kubectl get nodes|grep worker |grep -v NAME |awk '{print $1}' |xargs -I F docker cp ./scripts/remote.sh F:/remote.sh
kubectl get nodes|grep worker |grep -v NAME |awk '{print $1}' |xargs -I F docker exec F bash /remote.sh

kubectl apply -f ./scripts/k8s.yaml