
test -e /usr/sbin/xfs_quota || {

export https_proxy=http://172.20.10.248:1080 http_proxy=http://172.20.10.248:1080 all_proxy=socks5://172.20.10.248:1080

cat > /etc/apt/sources.list << EOF
deb http://mirrors.aliyun.com/debian/ bullseye main non-free contrib
deb-src http://mirrors.aliyun.com/debian/ bullseye main non-free contrib
deb http://mirrors.aliyun.com/debian-security/ bullseye-security main
deb-src http://mirrors.aliyun.com/debian-security/ bullseye-security main
deb http://mirrors.aliyun.com/debian/ bullseye-updates main non-free contrib
deb-src http://mirrors.aliyun.com/debian/ bullseye-updates main non-free contrib
deb http://mirrors.aliyun.com/debian/ bullseye-backports main non-free contrib
deb-src http://mirrors.aliyun.com/debian/ bullseye-backports main non-free contrib
EOF

#umount /var/lib/csi-hostpath-data/
#umount /tmp/quota
#rm -rf /tmp/quota
#rm -rf /var/lib/csi-hostpath-data/

dd if=/dev/zero of=/tmp/quota bs=1M count=500
apt clean all
apt update

apt install xfsprogs -y

mkfs.xfs /tmp/quota
stat -f -c %T /tmp/quota

mkdir -p /var/lib/csi-hostpath-data/
mount -o loop,usrquota,grpquota,prjquota /tmp/quota /var/lib/csi-hostpath-data/
mount | grep quota

}
