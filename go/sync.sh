
rm -rf /usr/local/go*
rm -rf ./go*
yum install wget vim -y
version=1.22.0
mkdir /usr/local/go$version


wget https://golang.google.cn/dl/go$version.linux-amd64.tar.gz
tar -xvf go$version.linux-amd64.tar.gz -C /usr/local/go$version --strip-components 1

#wget https://golang.google.cn/dl/go$version.linux-arm64.tar.gz
#tar -xvf go$version.linux-arm64.tar.gz -C /usr/local/go$version --strip-components 1


mkdir -p ~/.go/{bin,src,pkg}
chmod -R 777 /usr/local/go$version
cat <<EOF >>/etc/profile

export GOROOT="/usr/local/go$version"
export GOPATH=\$HOME/.go  #工作地址路径
export GOBIN=\$GOROOT/bin
export PATH=\$PATH:\$GOBIN
EOF
source /etc/profile
go version
go env
go env -w GO111MODULE=on
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOFLAGS="-buildvcs=false"

rm -rf go$version.linux-arm64.tar.gz

go install github.com/go-delve/delve/cmd/dlv@v1.20.0