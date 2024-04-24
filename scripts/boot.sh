set -x

docker build -t registry.cn-hangzhou.aliyuncs.com/acejilam/csi-hostpath:v2 .

export Master=`kubectl get node -l node-role.kubernetes.io/control-plane= -ojson|jq '.items[0].metadata.name' -r`

docker exec $Master mv /etc/kubernetes/manifests/kube-scheduler.yaml          /tmp/kube-scheduler.yaml
docker exec $Master mv /etc/kubernetes/manifests/kube-controller-manager.yaml /tmp/kube-controller-manager.yaml
docker exec $Master rm -rf /etc/apt/sources.list.d
docker cp ./scripts/sources.list $Master:/etc/apt/sources.list

kubectl apply -f ./scripts/k8s.yaml

docker rm yq --force
docker run --name yq -d registry.cn-hangzhou.aliyuncs.com/acejilam/yq:v4.43.1 sleep 1d
docker cp yq:/yq /tmp/yq
docker cp /tmp/yq $Master:/usr/bin/yq
docker exec $Master chmod 777 /usr/bin/yq

docker exec $Master bash -c 'Master=`kubectl get node -l node-role.kubernetes.io/control-plane= -ojson|jq ".items[0].metadata.name" -r` yq e ".spec.nodeName = strenv(Master)" -i  /tmp/kube-scheduler.yaml'
docker exec $Master bash -c 'SA=me yq e ".spec.serviceAccountName =strenv(SA)" -i  /tmp/kube-scheduler.yaml'
docker exec $Master bash -c 'Master=`kubectl get node -l node-role.kubernetes.io/control-plane= -ojson|jq ".items[0].metadata.name" -r` yq e ".spec.nodeName = strenv(Master)" -i  /tmp/kube-controller-manager.yaml'
docker exec $Master bash -c 'SA=me yq e ".spec.serviceAccountName =strenv(SA)" -i  /tmp/kube-controller-manager.yaml'

docker exec $Master kubectl delete -f /tmp/kube-scheduler.yaml -f /tmp/kube-controller-manager.yaml
docker exec $Master kubectl apply -f /tmp/kube-scheduler.yaml -f /tmp/kube-controller-manager.yaml

docker pull registry.cn-hangzhou.aliyuncs.com/acejilam/csi-provisioner:v4.0.0
docker pull registry.cn-hangzhou.aliyuncs.com/acejilam/csi-attacher:v4.5.0
docker pull registry.cn-hangzhou.aliyuncs.com/acejilam/csi-resizer:v1.10.0
docker pull registry.cn-hangzhou.aliyuncs.com/acejilam/livenessprobe:v2.12.0
docker pull registry.cn-hangzhou.aliyuncs.com/acejilam/csi-node-driver-registrar:v2.5.0
docker pull registry.cn-hangzhou.aliyuncs.com/acejilam/mygo:v1.21.5

kind load docker-image -n koord registry.cn-hangzhou.aliyuncs.com/acejilam/csi-provisioner:v4.0.0
kind load docker-image -n koord registry.cn-hangzhou.aliyuncs.com/acejilam/csi-attacher:v4.5.0
kind load docker-image -n koord registry.cn-hangzhou.aliyuncs.com/acejilam/csi-resizer:v1.10.0
kind load docker-image -n koord registry.cn-hangzhou.aliyuncs.com/acejilam/livenessprobe:v2.12.0
kind load docker-image -n koord registry.cn-hangzhou.aliyuncs.com/acejilam/csi-node-driver-registrar:v2.5.0

kind load docker-image -n koord registry.cn-hangzhou.aliyuncs.com/acejilam/mygo:v1.21.5
kind load docker-image -n koord registry.cn-hangzhou.aliyuncs.com/acejilam/csi-hostpath:v2

docker rmi $(docker images -f "dangling=true" -q)


kubectl get node -o jsonpath="{range .items[*]}{.metadata.name}{'\n'}{end}" |grep worker | xargs -I F docker cp ./scripts/host.sh F:/host.sh
kubectl get node -o jsonpath="{range .items[*]}{.metadata.name}{'\n'}{end}" |grep worker | xargs -I F docker cp ./scripts/sources.list F:/etc/apt/sources.list
kubectl get node -o jsonpath="{range .items[*]}{.metadata.name}{'\n'}{end}" |grep worker | xargs -I F docker exec F rm -rf /etc/apt/sources.list.d
kubectl get node -o jsonpath="{range .items[*]}{.metadata.name}{'\n'}{end}" |grep worker | xargs -I F docker exec F pkill -9 apt
kubectl get node -o jsonpath="{range .items[*]}{.metadata.name}{'\n'}{end}" |grep worker | xargs -I F docker exec F apt update
kubectl get node -o jsonpath="{range .items[*]}{.metadata.name}{'\n'}{end}" |grep worker | xargs -I F docker exec F apt install xfsprogs -y
kubectl get node -o jsonpath="{range .items[*]}{.metadata.name}{'\n'}{end}" |grep worker | xargs -I F docker exec F bash -c 'for i in {1..10}; do umount /csi-data-dir/csi-driver-host-path;done;'
sleep 1
kubectl get node -o jsonpath="{range .items[*]}{.metadata.name}{'\n'}{end}" |grep worker | xargs -I F docker exec F bash +x /host.sh
