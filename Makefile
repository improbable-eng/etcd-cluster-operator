# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

# Get binary dependencies
bin/kubebuilder:
	hack/download-kubebuilder-local.sh

# Run tests
test: generate fmt vet verify-manifests bin/kubebuilder
	KUBEBUILDER_ASSETS="$(shell pwd)/bin/kubebuilder/bin" go test ./... -coverprofile cover.out

kind: generate fmt vet manifests
	go test ./internal/test/e2e --kind --repo-root ${CURDIR} -v

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	./hack/update-manifests.sh

verify-manifests: controller-gen
	./hack/verify.sh ./hack/update-manifests.sh

# Run go fmt against code
fmt:
	go fmt ./...

verify-fmt:
	./hack/verify.sh make fmt

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	./hack/update-codegen.sh

verify-generate: controller-gen
	./hack/verify.sh ./hack/update-codegen.sh

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.1
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif
