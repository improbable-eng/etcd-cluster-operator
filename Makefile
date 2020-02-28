MAKEFLAGS += --warn-undefined-variables
SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
.SUFFIXES:
.DEFAULT_GOAL := help
# The version which will be reported by the --version argument of each binary
# and which will be used as the Docker image tag
VERSION ?= $(shell git describe --tags)

# Set DEBUG=TRUE to use debug Docker images
# This is required by the E2E backup tests so that the test can exec `ls
# /tmp/backup_file` in the controller-manager container.
DEBUG ?= $(if $(filter e2e,${MAKECMDGOALS}),TRUE,)

# Set ARGS to specify extra go test arguments
ARGS ?=

# Docker image configuration
# Docker images are published to https://quay.io/repository/improbable-eng/etcd-cluster-operator
DOCKER_TAG ?= ${VERSION}
DOCKER_REPO ?= quay.io/improbable-eng
DOCKER_IMAGES ?= controller controller-debug proxy backup-agent restore-agent
DOCKER_IMAGE_NAME_PREFIX ?= etcd-cluster-operator-
# The Docker image for the controller-manager which will be deployed to the cluster in tests
DOCKER_IMAGE_CONTROLLER = ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}controller$(if ${DEBUG},-debug,):${DOCKER_TAG}
DOCKER_IMAGE_PROXY = ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}proxy:${DOCKER_TAG}
DOCKER_IMAGE_RESTORE_AGENT = ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}restore-agent:${DOCKER_TAG}
DOCKER_IMAGE_BACKUP_AGENT = ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}backup-agent:${DOCKER_TAG}

OS := $(shell go env GOOS)
ARCH := $(shell go env GOARCH)
BIN := ${CURDIR}/bin
# Kind
KIND_VERSION := 0.7.0
KIND := ${BIN}/kind-${KIND_VERSION}
K8S_CLUSTER_NAME := etcd-e2e

E2E_ARTIFACTS_DIRECTORY ?= /tmp/${K8S_CLUSTER_NAME}

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Limit the number of parallel end-to-end tests
# A higher number will result in more etcd nodes being deployed in the test
# cluster at once and will require more CPU and memory.
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

# from https://suva.sh/posts/well-documented-makefiles/
.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[0-9a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: all
all: verify test manager

bin/kubebuilder: ## Get binary dependencies
	hack/download-kubebuilder-local.sh

.PHONY: verify
verify: ## Run all static checks
verify: verify-gomod verify-manifests verify-generate verify-protobuf verify-fmt vet

.PHONY: test
test: ## Run unit tests
test: bin/kubebuilder
	KUBEBUILDER_ASSETS="$(shell pwd)/bin/kubebuilder/bin" go test ./... -coverprofile cover.out $(ARGS)

.PHONY: e2e-kind
e2e-kind: ## Run end to end tests - creates a new Kind cluster called etcd-e2e
e2e-kind: DEBUG=TRUE
e2e-kind: docker-build kind-cluster kind-load deploy-e2e e2e

.PHONY: e2e
e2e: ## Run the end-to-end tests - uses the current KUBE_CONFIG and context
e2e: DEBUG=TRUE
e2e:
	go test -v -parallel ${TEST_PARALLEL_E2E} -timeout 20m ./internal/test/e2e --e2e-enabled --repo-root ${CURDIR} --output-directory ${E2E_ARTIFACTS_DIRECTORY} $(ARGS)

.PHONY: manager
manager: ## Build manager binary
	go build -o bin/manager -ldflags="-X 'github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}'" main.go

# Use 'DISABLE_WEBHOOKS=1` to run the controller-manager without the
# webhook server, and to skip the loading of webhook TLS keys, since these are
# difficult to set up locally.
.PHONY: run
run: ## Run against the configured Kubernetes cluster in ~/.kube/config
	DISABLE_WEBHOOKS=1 go run ./main.go

.PHONY: install
install: ## Install CRDs into a cluster
	kustomize build config/crd | kubectl apply -f -

.PHONY: deploy-minio
deploy-minio: ## Deploy MinIO in the cluster for backups and restores
deploy-minio:
	kubectl apply -k config/test/e2e/minio
# We can't wait on the `minio` service: https://github.com/kubernetes/kubernetes/issues/80828
# Nor can we wait on a statefulset: https://github.com/kubernetes/kubernetes/issues/79606
	count=0; until [[ $$(kubectl get -n minio statefulset minio -o jsonpath="{.status.readyReplicas}") == 1 ]]; do  [[ $${count} -le 60 ]] || exit 1; ((count+=1)); sleep 1; done

.PHONY: deploy-cert-manager
deploy-cert-manager: ## Deploy cert-manager in the configured Kubernetes cluster in ~/.kube/config
	kubectl apply --validate=false --filename=https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml
	kubectl wait --for=condition=Available --timeout=300s apiservice v1beta1.webhook.cert-manager.io

.PHONY: deploy-controller
deploy-controller: ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	cd config/test/e2e && kustomize edit set image controller=${DOCKER_IMAGE_CONTROLLER}
	cd config/test/e2e && kustomize edit set image proxy=${DOCKER_IMAGE_PROXY}
	kustomize build config/test/e2e | kubectl apply -f -
	kubectl --namespace eco-system wait --for=condition=Available --timeout=60s deploy eco-controller-manager
	kubectl --namespace eco-system wait --for=condition=Available --timeout=60s deploy eco-proxy

.PHONY: deploy
deploy: ## Deploy the operator, including dependencies
deploy: deploy-controller

.PHONY: deploy-e2e
deploy-e2e: ## Deploy the operator, including all dependencies needed to run E2E tests
deploy-e2e: deploy-cert-manager deploy-minio deploy

.PHONY: protoc-docker
protoc-docker: ## Build a Docker image which can be used for generating protobuf code (below)
	docker build --quiet - -t protoc < hack/grpc-protoc.Dockerfile

.PHONY: protobuf
protobuf: ## Generate a go implementation of the protobuf proxy API
protobuf: protoc-docker
	docker run -v `pwd`:/eco -w /eco protoc:latest -I=api/proxy --go_out=plugins=grpc:api/proxy api/proxy/v1/proxy.proto

.PHONY: verify-protobuf-lint
verify-protobuf-lint: ## Run protobuf static checks
	docker run --volume ${CURDIR}:/workspace:ro --workdir /workspace bufbuild/buf check lint

.PHONY: manifests
manifests: ## Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: fmt
fmt: ## Run go fmt against code
	gofmt -w .

.PHONY: verify-fmt
verify-fmt: ## Check go code formatting
	gofmt -d .

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: generate
generate: ## Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

.PHONY: gomod
gomod: ## Update the go.mod and go.sum files
	go mod tidy

.PHONY: go-get-patch
go-get-patch: ## Update Golang dependencies to latest patch versions
	go get -u=patch -t

.PHONY: docker-build
docker-build: ## Build the all the docker images
docker-build: $(addprefix docker-build-,$(DOCKER_IMAGES))

docker-build-%: FORCE
	docker build . --target $* \
		--build-arg VERSION=$(VERSION) \
		--build-arg RESTORE_AGENT_IMAGE=${DOCKER_IMAGE_RESTORE_AGENT} \
		--tag ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}$*:${DOCKER_TAG}
FORCE:

.PHONY: docker-push
docker-push: ## Push all the docker images
docker-push: $(addprefix docker-push-,$(DOCKER_IMAGES))

docker-push-%: FORCE
	docker push ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}$*:${DOCKER_TAG}
FORCE:

.PHONY: controller-gen
controller-gen: ## find or download controller-gen
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

${KIND}: ${BIN}
	curl -sSL -o ${KIND} https://github.com/kubernetes-sigs/kind/releases/download/v${KIND_VERSION}/kind-${OS}-${ARCH}
	chmod +x ${KIND}

.PHONY: kind-cluster
kind-cluster: ## Use Kind to create a Kubernetes cluster for E2E tests
kind-cluster: TERM=dumb
kind-cluster: ${KIND}
	 ${KIND} get clusters | grep ${K8S_CLUSTER_NAME} || ${KIND} create cluster --name ${K8S_CLUSTER_NAME}

.PHONY: kind-cluster
kind-load: ## Load all the Docker images into Kind
kind-load: $(addprefix kind-load-,$(DOCKER_IMAGES))

kind-load-%: FORCE ${KIND}
	${KIND}	load docker-image --name ${K8S_CLUSTER_NAME} ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}$*:${DOCKER_TAG}
FORCE:

.PHONY: kind-export-logs
kind-export-logs:
	${KIND} export logs --name ${K8S_CLUSTER_NAME} ${E2E_ARTIFACTS_DIRECTORY}

${BIN}:
	mkdir -p ${BIN}
