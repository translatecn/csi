FROM centos:7
WORKDIR /
COPY . .
RUN yum install wget -y
RUN bash ./sync.sh

# docker buildx build --platform registry.cn-hangzhou.aliyuncs.com/acejilam/mygo:1.22 . --push