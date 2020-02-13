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
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY internal/ internal/
COPY webhooks/ webhooks/
COPY version/ version/
COPY cmd/ cmd/

ARG VERSION

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
ENV GO111MODULE=on
ENV GOFLAGS=-ldflags=-X=github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}

# manager
RUN go build -o manager main.go

# backup-agent
RUN go build -o backup-agent cmd/backup-agent/main.go

FROM gcr.io/distroless/static:nonroot as release
WORKDIR /
COPY --from=builder /workspace/manager .
USER nonroot:nonroot

ENTRYPOINT ["/manager"]

FROM alpine:3.10.3 as debug
WORKDIR /
COPY --from=builder /workspace/manager .

RUN apk update && apk add ca-certificates bash curl drill jq

ENTRYPOINT ["/manager"]

FROM gcr.io/distroless/static:nonroot as backup-agent
WORKDIR /
COPY --from=builder /workspace/backup-agent .
USER nonroot:nonroot
ENTRYPOINT ["/backup-agent"]