mkdir -p "/csi-data-dir/csi-driver-host-path"

sleep 1
test -e /nfs/$(hostname) || {

dd if=/dev/zero of="/nfs/$(hostname)" bs=1M count=500
mkfs.xfs "/nfs/$(hostname)"
sleep 1
#xfs_quota -x -c 'project -s -p /var/lib/csi-hostpath-data/123/ 111111' /var/lib/csi-hostpath-data/
#xfs_quota -x -c 'limit -p bhard=10M 111111' /var/lib/csi-hostpath-data/
#dd if=/dev/zero of="/var/lib/csi-hostpath-data/123/quota.txt" bs=1M count=20
}
mount -o nouuid,loop,usrquota,grpquota,prjquota "/nfs/$(hostname)" /csi-data-dir/csi-driver-host-path
sleep 1
stat -f -c %T /csi-data-dir/csi-driver-host-path
mount | grep csi-data-dir


