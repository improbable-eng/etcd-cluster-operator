#
# BUILD ENVIRONMENT
# -----------------
ARG GO_VERSION
FROM golang:${GO_VERSION} as builder

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
ARG BACKUP_AGENT_IMAGE
ARG RESTORE_AGENT_IMAGE

# Compile all the binaries
RUN go build \
    -mod=readonly \
    -ldflags="-X=github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}\
              -X=main.defaultBackupAgentImage=${BACKUP_AGENT_IMAGE}\
              -X=main.defaultRestoreAgentImage=${RESTORE_AGENT_IMAGE}" \
    -o manager main.go
RUN go build -mod=readonly "-ldflags=-s -w -X=github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}" -o proxy cmd/proxy/main.go
RUN go build -mod=readonly "-ldflags=-s -w -X=github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}" -o backup-agent cmd/backup-agent/main.go
RUN go build -mod=readonly "-ldflags=-s -w -X=github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}" -o restore-agent cmd/restore-agent/main.go

RUN upx manager proxy backup-agent restore-agent

#
# IMAGE TARGETS
# -------------
FROM gcr.io/distroless/static:nonroot as controller
WORKDIR /
COPY --from=builder /workspace/manager .
USER nonroot:nonroot
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

# restore-agent must run as root
# See https://github.com/improbable-eng/etcd-cluster-operator/issues/139
FROM gcr.io/distroless/static as restore-agent
WORKDIR /
COPY --from=builder /workspace/restore-agent .
USER root:root
ENTRYPOINT ["/restore-agent"]
