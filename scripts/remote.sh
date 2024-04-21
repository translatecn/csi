
test -e /usr/sbin/xfs_quota || {

#export https_proxy=http://172.20.10.248:1080 http_proxy=http://172.20.10.248:1080 all_proxy=socks5://172.20.10.248:1080

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

dd if=/dev/zero of="/build_cache/${hostname}_xfs" bs=4K count=4096
apt clean all
apt update

apt install xfsprogs -y

mkfs.xfs "/build_cache/${hostname}_xfs"
stat -f -c %T "/build_cache/${hostname}_xfs"

mkdir -p /var/lib/csi-hostpath-data/
mount -o loop,usrquota,grpquota,prjquota "/build_cache/${hostname}_xfs" /var/lib/csi-hostpath-data/
mount | grep quota

}
