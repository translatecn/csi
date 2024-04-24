# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM registry.cn-hangzhou.aliyuncs.com/acejilam/mygo:v1.21.5 as build
WORKDIR /app
COPY pkg pkg
COPY go.mod go.mod
COPY go.sum go.sum
COPY main.go main.go
RUN /usr/local/go1.21.5/bin/go mod vendor
RUN /usr/local/go1.21.5/bin/go build -o csi-bin .

FROM centos:7
WORKDIR /app
RUN yum clean all && yum update -y && yum install socat xfsprogs -y
COPY --from=build /app/csi-bin /usr/bin/csi-bin
COPY ./scripts ./scripts
ENTRYPOINT ["/usr/bin/csi-bin"]
