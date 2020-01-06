# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

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

all: verify test manager

# Get binary dependencies
bin/kubebuilder:
	hack/download-kubebuilder-local.sh

# Run all static checks
verify: verify-gomod verify-manifests verify-generate verify-fmt vet

# Run unit tests
test: bin/kubebuilder
	KUBEBUILDER_ASSETS="$(shell pwd)/bin/kubebuilder/bin" go test ./... -coverprofile cover.out $(ARGS)

# Run end to end tests in a local Kind cluster. We do not clean up after running the tests to
#  a) speed up the test run time slightly
#  b) allow debug sessions to be attached to figure out what caused failures
# We use -parallel 2 to try and avoid overloading the Circle CI server with too
# many parallel EtcdClusters
kind:
	go test -parallel 2 ./internal/test/e2e --kind --repo-root ${CURDIR} -v --cleanup=${CLEANUP} $(ARGS)

# Build manager binary
manager:
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
# Use 'DISABLE_WEBHOOKS=1` to run the controller-manager without the
# webhook server, and to skip the loading of webhook TLS keys, since these are
# difficult to set up locally.
run:
	DISABLE_WEBHOOKS=1 go run ./main.go

# Install CRDs into a cluster
install:
	kustomize build config/crd | kubectl apply -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy:
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -


# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

verify-manifests: controller-gen
	./hack/verify.sh make -s manifests

# Run go fmt against code
fmt:
	gofmt -w .

verify-fmt:
	gofmt -d .

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

verify-generate: controller-gen
	./hack/verify.sh make -s generate

gomod:
	go mod tidy

verify-gomod:
	./hack/verify.sh make -s gomod

# Build the docker image. This should be used for release versions, and builds the image on top of distroless.
docker-build: test
	docker build . --target release -t ${IMG}

# Build the docker image with debug tools installed.
docker-build-debug: test
	docker build . --target debug -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
# Prevents go get from modifying our go.mod file.
# See https://github.com/kubernetes-sigs/kubebuilder/issues/909
	cd /tmp; GO111MODULE=on go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.4
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif
