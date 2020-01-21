# Build the manager binary
FROM golang:1.13.1 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY internal/ internal/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o restoreagent cmd/restoreagent/main.go

FROM gcr.io/distroless/static as release
WORKDIR /
COPY --from=builder /workspace/restoreagent .
# Need to run as root so that we can write to the PVC as root.
# See https://github.com/improbable-eng/etcd-cluster-operator/issues/139
USER root:root

ENTRYPOINT ["/restoreagent"]
