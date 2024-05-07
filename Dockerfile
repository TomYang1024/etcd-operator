# Build the manager binary
FROM golang:1.22 AS builder

ARG TARGETOS
ARG TARGETARCH

RUN echo "deb http://archive.ubuntu.com/ubuntu/ focal main restricted universe multiverse" > /etc/apt/sources.list
RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 3B4FE6ACC0B21F32 871920D1991BC93C

# 压缩镜像
RUN apt-get update -y --allow-unauthenticated && apt-get install -y upx

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY cmd/backup/main.go cmd/backup/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
COPY pkg/operator/ pkg/operator/


# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.

ENV CGO_ENABLED=0 
ENV GOOS=${TARGETOS:-linux} 
ENV GOARCH=${TARGETARCH}
# ENV GOPROXY=https://goproxy.cn,direct

RUN go build -x -a -o manager cmd/main.go
RUN go build -a -o backup cmd/backup/main.go
RUN upx manager backup
# RUN go mod init etcd-operator
# RUN go mod download && \
#     go build -a -o manager cmd/main.go && \
#     go build -a -o backup cmd/backup/main.go && \
#     upx manager backup 

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot as manager
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]



FROM gcr.io/distroless/static:nonroot as backup
WORKDIR /
COPY --from=builder /workspace/backup .
USER 65533:65533

ENTRYPOINT ["/backup"]


