#
# BUILD ENVIRONMENT
# -----------------
FROM golang:1.13.1 as builder

RUN apt-get -y update && apt-get -y install upx

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY internal/ internal/
COPY webhooks/ webhooks/
COPY version/ version/
COPY cmd/ cmd/

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
ENV GO111MODULE=on

# Do an initial compilation before setting the version so that there is less to
# re-compile when the version changes
RUN go build -mod=readonly "-ldflags=-s -w" ./...

ARG VERSION

# Compile all the binaries
RUN go build -mod=readonly "-ldflags=-s -w -X=github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}" -o manager main.go
RUN go build -mod=readonly "-ldflags=-s -w -X=github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}" -o proxy cmd/proxy/main.go
RUN go build -mod=readonly "-ldflags=-s -w -X=github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}" -o backup-agent cmd/backup-agent/main.go

RUN upx manager proxy backup-agent

#
# IMAGE TARGETS
# -------------
FROM gcr.io/distroless/static:nonroot as controller
WORKDIR /
COPY --from=builder /workspace/manager .
USER nonroot:nonroot
ENTRYPOINT ["/manager"]

FROM alpine:3.10.3 as controller-debug
WORKDIR /
COPY --from=builder /workspace/manager .
RUN apk update && apk add ca-certificates bash curl drill jq
ENTRYPOINT ["/manager"]

FROM gcr.io/distroless/static:nonroot as proxy
WORKDIR /
COPY --from=builder /workspace/proxy .
USER nonroot:nonroot
ENTRYPOINT ["/proxy"]

FROM gcr.io/distroless/static:nonroot as backup-agent
WORKDIR /
COPY --from=builder /workspace/backup-agent .
USER nonroot:nonroot
ENTRYPOINT ["/backup-agent"]
