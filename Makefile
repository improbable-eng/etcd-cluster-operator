MAKEFLAGS += --warn-undefined-variables
SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
.SUFFIXES:
.DEFAULT_GOAL := help
# The version which will be reported by the --version argument of each binary
# and which will be used as the Docker image tag
VERSION ?= $(shell git describe --tags)

# Set ARGS to specify extra go test arguments
ARGS ?=

# BIN is the directory where build tools such controller-gen and kustomize will
# be installed.
# BIN is inherited and exported so that it gets passed down to the make process
# that is launched by verify.sh
# This ensures that test tools get installed in the original directory rather
# than in the temporary copy.
export BIN ?= ${CURDIR}/bin

# Make sure BIN is on the PATH
export PATH := $(BIN):$(PATH)

GO_VERSION ?= 1.14
# Look for a `go1.14` on the path but fall back to `go`.
# Allows me to use `go get golang.org/dl/go1.14` without having to replace my OS
# provided golang package.
GO := $(or $(shell which go${GO_VERSION}),$(shell which go))

# Docker image configuration
# Docker images are published to https://quay.io/repository/improbable-eng/etcd-cluster-operator
DOCKER_TAG ?= ${VERSION}
DOCKER_REPO ?= quay.io/improbable-eng
DOCKER_IMAGES ?= controller proxy backup-agent restore-agent
DOCKER_IMAGE_NAME_PREFIX ?= etcd-cluster-operator-
# The Docker image for the controller-manager which will be deployed to the cluster in tests
DOCKER_IMAGE_CONTROLLER = ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}controller:${DOCKER_TAG}
DOCKER_IMAGE_PROXY = ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}proxy:${DOCKER_TAG}
DOCKER_IMAGE_RESTORE_AGENT = ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}restore-agent:${DOCKER_TAG}
DOCKER_IMAGE_BACKUP_AGENT = ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}backup-agent:${DOCKER_TAG}
DOCKER_IMAGE_NAME = ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}${COMPONENT}:${DOCKER_TAG}

OS := $(shell ${GO} env GOOS)
ARCH := $(shell ${GO} env GOARCH)

# Kind
KIND_VERSION := 0.7.0
KIND := ${BIN}/kind-${KIND_VERSION}
K8S_CLUSTER_NAME := etcd-e2e

# controller-tools
CONTROLLER_GEN_VERSION := 0.2.5
CONTROLLER_GEN := ${BIN}/controller-gen-0.2.5

# Kustomize
KUSTOMIZE_VERSION := 3.5.4
KUSTOMIZE_DOWNLOAD_URL := https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_${OS}_${ARCH}.tar.gz
KUSTOMIZE_LOCAL_ARCHIVE := /tmp/kustomize_v${KUSTOMIZE_VERSION}_${OS}_${ARCH}.tar.gz
KUSTOMIZE := ${BIN}/kustomize-3.5.4
KUSTOMIZE_DIRECTORY_TO_EDIT := "config/test/e2e"
KUSTOMIZE_DIRECTORY_TO_DEPLOY := "config/test/e2e"

# Release files
RELEASE_NOTES := docs/release-notes/${VERSION}.md
RELEASE_MANIFEST := "release-${VERSION}.yaml"

E2E_ARTIFACTS_DIRECTORY ?= /tmp/${K8S_CLUSTER_NAME}

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Limit the number of parallel end-to-end tests
# A higher number will result in more etcd nodes being deployed in the test
# cluster at once and will require more CPU and memory.
TEST_PARALLEL_E2E ?= 2

# Prevent fancy TTY output from tools like kind
export TERM=dumb

# Stop go build tools from silently modifying go.mod and go.sum
export GOFLAGS := -mod=readonly

# from https://suva.sh/posts/well-documented-makefiles/
.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[0-9a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: all
all: verify test manager

.PHONY: verify
verify: ## Run all static checks
verify: verify-gomod verify-manifests verify-generate verify-protobuf verify-fmt vet

.PHONY: test
test: ## Run unit tests
test: bin/kubebuilder
	KUBEBUILDER_ASSETS="$(shell pwd)/bin/kubebuilder/bin" ${GO} test ./... -coverprofile cover.out $(ARGS)

.PHONY: e2e-kind
e2e-kind: ## Run end to end tests - creates a new Kind cluster called etcd-e2e
e2e-kind: docker-build kind-cluster kind-load deploy-e2e e2e

.PHONY: e2e
e2e: ## Run the end-to-end tests - uses the current KUBE_CONFIG and context
e2e:
	${GO} test -v -parallel ${TEST_PARALLEL_E2E} -timeout 20m ./internal/test/e2e --e2e-enabled --repo-root ${CURDIR} --output-directory ${E2E_ARTIFACTS_DIRECTORY} $(ARGS)

.PHONY: manager
manager: ## Build manager binary
	${GO} build -o bin/manager -ldflags="-X 'github.com/improbable-eng/etcd-cluster-operator/version.Version=${VERSION}'" main.go

# Use 'DISABLE_WEBHOOKS=1` to run the controller-manager without the
# webhook server, and to skip the loading of webhook TLS keys, since these are
# difficult to set up locally.
.PHONY: run
run: ## Run against the configured Kubernetes cluster in ~/.kube/config
	DISABLE_WEBHOOKS=1 ${GO} run ./main.go


# ===========================================================
# Deploy: Set image tags in manifests and deploy the operator
# ===========================================================

.PHONY: install
install: ## Install CRDs into a cluster
install: ${KUSTOMIZE}
	${KUSTOMIZE} build config/crd | kubectl apply -f -

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
deploy-controller: kustomize-edit-set-image
	${KUSTOMIZE} build ${KUSTOMIZE_DIRECTORY_TO_DEPLOY} | kubectl apply -f -
	kubectl --namespace eco-system wait --for=condition=Available --timeout=60s deploy eco-controller-manager
	kubectl --namespace eco-system wait --for=condition=Available --timeout=60s deploy eco-proxy

.PHONY: deploy
deploy: ## Deploy the operator, including dependencies
deploy: deploy-controller

.PHONY: deploy-e2e
deploy-e2e: ## Deploy the operator, including all dependencies needed to run E2E tests
deploy-e2e: deploy-cert-manager deploy-minio deploy

kustomize-edit-set-image-%: COMPONENT=$*
kustomize-edit-set-image-%: ${KUSTOMIZE} FORCE
	cd ${KUSTOMIZE_DIRECTORY_TO_EDIT} && ${KUSTOMIZE} edit set image ${COMPONENT}=${DOCKER_IMAGE_NAME}
FORCE:

.PHONY: kustomize-edit-set-image
kustomize-edit-set-image: $(addprefix kustomize-edit-set-image-,controller proxy)

.PHONY: protoc-docker
protoc-docker: ## Build a Docker image which can be used for generating protobuf code (below)
	docker build --build-arg GO_VERSION=${GO_VERSION} --quiet - -t protoc < hack/grpc-protoc.Dockerfile

.PHONY: protobuf
protobuf: ## Generate a go implementation of the protobuf proxy API
protobuf: protoc-docker
	docker run -v `pwd`:/eco -w /eco protoc:latest -I=api/proxy --go_out=plugins=grpc:api/proxy api/proxy/v1/proxy.proto

.PHONY: verify-protobuf-lint
verify-protobuf-lint: ## Run protobuf static checks
	docker run --volume ${CURDIR}:/workspace:ro --workdir /workspace bufbuild/buf check lint

.PHONY: manifests
manifests: ## Generate manifests e.g. CRD, RBAC etc.
manifests: ${CONTROLLER_GEN}
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: fmt
fmt: ## Run go fmt against code
	${GO} fmt .

.PHONY: vet
vet: ## Run go vet against code
	${GO} vet ./...

.PHONY: generate
generate: ## Generate code
generate: ${CONTROLLER_GEN}
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."

.PHONY: gomod
gomod: ## Update the go.mod and go.sum files
	${GO} mod tidy

.PHONY: go-get-patch
go-get-patch: ## Update Golang dependencies to latest patch versions
	${GO} get -u=patch -t

.PHONY: docker-build
docker-build: ## Build the all the docker images
docker-build: $(addprefix docker-build-,$(DOCKER_IMAGES))

docker-build-%: FORCE
	docker build --target $* \
		--build-arg GO_VERSION=${GO_VERSION} \
		--build-arg VERSION=$(VERSION) \
		--build-arg BACKUP_AGENT_IMAGE=${DOCKER_IMAGE_BACKUP_AGENT} \
		--build-arg RESTORE_AGENT_IMAGE=${DOCKER_IMAGE_RESTORE_AGENT} \
		--tag ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}$*:${DOCKER_TAG} \
		--file Dockerfile \
		${CURDIR}
FORCE:

.PHONY: docker-push
docker-push: ## Push all the docker images
docker-push: $(addprefix docker-push-,$(DOCKER_IMAGES))

docker-push-%: FORCE
	docker push ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}$*:${DOCKER_TAG}
FORCE:

# Run the supplied make target argument in a temporary workspace and diff the results.
verify-%: FORCE
	./hack/verify.sh ${MAKE} -s $*
FORCE:

.PHONY: kind-cluster
kind-cluster: ## Use Kind to create a Kubernetes cluster for E2E tests
kind-cluster: ${KIND}
	 ${KIND} get clusters | grep ${K8S_CLUSTER_NAME} || ${KIND} create cluster --name ${K8S_CLUSTER_NAME}

.PHONY: kind-cluster
kind-load: ## Load all the Docker images into Kind
kind-load: $(addprefix kind-load-,$(DOCKER_IMAGES))

kind-load-%: FORCE ${KIND}
	${KIND} load docker-image --name ${K8S_CLUSTER_NAME} ${DOCKER_REPO}/${DOCKER_IMAGE_NAME_PREFIX}$*:${DOCKER_TAG}
FORCE:

.PHONY: kind-export-logs
kind-export-logs:
	${KIND} export logs --name ${K8S_CLUSTER_NAME} ${E2E_ARTIFACTS_DIRECTORY}

# ===================================
# Release: Tag and Push Docker Images
# ===================================

.PHONY: .create-release-tag
.create-release-tag: KUSTOMIZE_DIRECTORY_TO_EDIT=config/manager
.create-release-tag: kustomize-edit-set-image
	git add ${KUSTOMIZE_DIRECTORY_TO_EDIT}
	git commit --message "Release ${VERSION}"
	git tag --annotate --message "Release ${VERSION}" ${VERSION}

${RELEASE_NOTES}:
	$(error "Release notes not found: ${RELEASE_NOTES}")

${RELEASE_MANIFEST}: KUSTOMIZE_DIRECTORY_TO_EDIT=config/manager
${RELEASE_MANIFEST}: kustomize-edit-set-image
	${KUSTOMIZE} build --output ${RELEASE_MANIFEST} ${KUSTOMIZE_DIRECTORY_TO_EDIT}

.PHONY: release
release: ## Create a tagged release
release: KUSTOMIZE_DIRECTORY_TO_EDIT=config/manager
release: version ${RELEASE_NOTES} docker-build docker-push ${RELEASE_MANIFEST} .create-release-tag

.PHONY: version
version:
	@echo $(or $(filter v%, $(firstword ${VERSION})), $(error Version must begin with v. Got: ${VERSION}))


# ==================================
# Download: tools in ${BIN}
# ==================================
${BIN}:
	mkdir -p ${BIN}

${CONTROLLER_GEN}: | ${BIN}
# Prevents go get from modifying our go.mod file.
# See https://github.com/kubernetes-sigs/kubebuilder/issues/909
	cd /tmp; GOBIN=${BIN} GO111MODULE=on ${GO} get sigs.k8s.io/controller-tools/cmd/controller-gen@v${CONTROLLER_GEN_VERSION}
	mv ${BIN}/controller-gen ${CONTROLLER_GEN}

${KUSTOMIZE}: | ${BIN}
	curl -sSL -o ${KUSTOMIZE_LOCAL_ARCHIVE} ${KUSTOMIZE_DOWNLOAD_URL}
	tar -C ${BIN} -x -f ${KUSTOMIZE_LOCAL_ARCHIVE}
	mv ${BIN}/kustomize ${KUSTOMIZE}

${KIND}: ${BIN}
	curl -sSL -o ${KIND} https://github.com/kubernetes-sigs/kind/releases/download/v${KIND_VERSION}/kind-${OS}-${ARCH}
	chmod +x ${KIND}

bin/kubebuilder:
	hack/download-kubebuilder-local.sh
