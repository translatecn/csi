CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/hostpathplugin main.go

docker build -t k8s.m.daocloud.io/sig-storage/hostpathplugin:v1.13.0 .

#docker pull k8s.m.daocloud.io/sig-storage/csi-external-health-monitor-controller:v0.11.0
#docker pull k8s.m.daocloud.io/sig-storage/csi-node-driver-registrar:v2.10.0
#docker pull k8s.m.daocloud.io/sig-storage/livenessprobe:v2.12.0
#docker pull k8s.m.daocloud.io/sig-storage/csi-attacher:v4.5.0
#docker pull k8s.m.daocloud.io/sig-storage/csi-provisioner:v4.0.0
#docker pull k8s.m.daocloud.io/sig-storage/csi-resizer:v1.10.0
#docker pull k8s.m.daocloud.io/sig-storage/csi-snapshotter:v7.0.1
docker pull registry.cn-hangzhou.aliyuncs.com/acejilam/mygo:1.22
#
#
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/hostpathplugin:v1.13.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/csi-external-health-monitor-controller:v0.11.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/csi-node-driver-registrar:v2.10.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/livenessprobe:v2.12.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/csi-attacher:v4.5.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/csi-provisioner:v4.0.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/csi-resizer:v1.10.0
#kind load docker-image -n koord  k8s.m.daocloud.io/sig-storage/csi-snapshotter:v7.0.1
kind load docker-image -n koord  registry.cn-hangzhou.aliyuncs.com/acejilam/mygo:1.22


#kubectl get nodes|grep worker |grep -v NAME |awk '{print $1}' |xargs -I F docker cp ./scripts/remote.sh F:/remote.sh
#kubectl get nodes|grep worker |grep -v NAME |awk '{print $1}' |xargs -I F docker exec F bash /remote.sh

