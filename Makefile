VERSION ?= $(shell git describe --tags)
# Image URL to use all building/pushing image targets
IMG ?= "controller:$(VERSION)"
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

TEST_PARALLEL_E2E ?= 2

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Make sure GOBIN is on the PATH
export PATH := $(GOBIN):$(PATH)

# Stop go build tools from silently modifying go.mod and go.sum
export GOFLAGS := -mod=readonly

ifeq ($(CIRCLECI),"true")
CLEANUP="false"
else
CLEANUP="true"
endif

.PHONY: all
all: verify test manager

# Get binary dependencies
bin/kubebuilder:
	hack/download-kubebuilder-local.sh

# Run all static checks
.PHONY: verify
verify: verify-gomod verify-manifests verify-generate verify-protobuf verify-fmt vet

# Run unit tests
.PHONY: test
test: bin/kubebuilder
	KUBEBUILDER_ASSETS="$(shell pwd)/bin/kubebuilder/bin" go test ./... -coverprofile cover.out $(ARGS)

# Run end to end tests in a local Kind cluster. We do not clean up after running the tests to
#  a) speed up the test run time slightly
#  b) allow debug sessions to be attached to figure out what caused failures
.PHONY: kind
kind:
	go test -parallel ${TEST_PARALLEL_E2E} -timeout 20m ./internal/test/e2e --kind --repo-root ${CURDIR} -v --cleanup=${CLEANUP} $(ARGS)

# Build manager binary
.PHONY: manager
manager:
	go build -o bin/manager -ldflags="-X 'github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}'" main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
# Use 'DISABLE_WEBHOOKS=1` to run the controller-manager without the
# webhook server, and to skip the loading of webhook TLS keys, since these are
# difficult to set up locally.
.PHONY: run
run:
	DISABLE_WEBHOOKS=1 go run ./main.go

# Install CRDs into a cluster
.PHONY: install
install:
	kustomize build config/crd | kubectl apply -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy:
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

.PHONY: protoc-docker
protoc-docker:
	docker build --quiet - -t protoc < hack/grpc-protoc.Dockerfile

.PHONY: protobuf
protobuf: protoc-docker
	docker run -v `pwd`:/eco -w /eco protoc:latest -I=api/proxy --go_out=plugins=grpc:api/proxy api/proxy/v1/proxy.proto

.PHONY: verify-protobuf-lint
verify-protobuf-lint:
	docker run --volume ${CURDIR}:/workspace:ro --workdir /workspace bufbuild/buf check lint

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: verify-fmt
verify-fmt:
	gofmt -d .

# Run go vet against code
.PHONY: vet
vet:
	go vet ./...

# Generate code
.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

.PHONY: gomod
gomod:
	go mod tidy

# go-get-patch updates Golang dependencies to latest patch versions
.PHONY: go-get-patch
go-get-patch:
	go get -u=patch -t

# Build the docker image. This should be used for release versions, and builds the image on top of distroless.
.PHONY: docker-build
docker-build:
	docker build . --target release --build-arg VERSION=$(VERSION) -t ${IMG}

# Build the docker image with debug tools installed.
.PHONY: docker-build-debug
docker-build-debug:
	docker build . --target debug --build-arg VERSION=$(VERSION) -t ${IMG}

.PHONY: docker-build-proxy
docker-build-proxy:
	docker build --build-arg VERSION=$(VERSION) --tag "eco-proxy:$(VERSION)" --file build/package/proxy.Dockerfile .

.PHONY: docker-build-backup-agent
docker-build-backup-agent:
	docker build --build-arg VERSION=$(VERSION) --tag "eco-backup-agent:$(VERSION)" --file build/package/backup-agent.Dockerfile .

# Push the docker image
.PHONY: docker-push
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
.PHONY: controller-gen
controller-gen:
ifeq (, $(shell which controller-gen))
# Prevents go get from modifying our go.mod file.
# See https://github.com/kubernetes-sigs/kubebuilder/issues/909
	cd /tmp; GO111MODULE=on go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

# Run the supplied make target argument in a temporary workspace and diff the results.
verify-%: FORCE
	./hack/verify.sh make -s $*
FORCE:
