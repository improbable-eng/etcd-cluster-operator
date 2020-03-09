# Development guide

## Building Locally

To build and run the images yourself, you'll need a Docker registry that you have push access to and your Kubernetes
nodes have pull access from. First clone this git repository to your local disk.

You will need `make`, `docker`, `go`, `kustomize`, and `kubectl` installed.

### Building Docker Images

```bash
export DOCKER_REPO=example.com/my-project
make docker-build
make docker-push
```

The actual build is performed inside Docker and will use the go version in the `Dockerfile`, but the `docker-build`
target will first run the tests locally which will use your system `go`.

The `DOCKER_REPO` environment variable is used by the `Makefile`.

### Deploying the Operator to a cluster

Make sure that your desired cluster is your default context in `kubectl`, and that `DOCKER_REPO` from above is still
exported.

```bash
make deploy
```

This will leave changes on disk in your `config` directory, take care not to commit them.


### Deploying an etcd cluster

```bash
kubectl apply -f config/samples/etcd_v1alpha1_etcdcluster.yaml
```

You can check the status of the cluster by querying for the `EtcdCluster` resource.

### Testing Connectivity inside Kubernetes

If you launch another pod in the same namespace you can dial the cluster yourself using `my-cluster` as a DNS name.

```bash
kubectl run --image nixery.dev/shell/etcd --generator run-pod/v1 etcd-shell -- /bin/bash -c "while true; do sleep 30; done;"
kubectl exec etcd-shell -- etcdctl --endpoints "http://my-cluster:2379" member list
```

### Cleanup

You can remove the testing pod with `kubectl delete po etcd-shell`.

The cluster can be removed with `kubectl delete -f config/samples/etcd_v1alpha1_etcdcluster.yaml`, or by manually
specifying the `EtcdCluster` resource to delete.

## Developing

Use `make test` to run the tests. This will download the Kubebuilder binaries for your platform (only x86-64 Linux and
x86-64 macOS supported) to `bin/kubebuilder` and use those to run tests.

Use `make e2e-kind` to run the [Kubernetes in Docker (KIND)](https://github.com/kubernetes-sigs/kind) end to end tests.
You need both the `kind` and `docker` binaries installed and available. These tests build the controller Docker Images
from the `Dockerfile`, push it into the KIND cluster, and then ensure that the etcd cluster actually comes up.

## Generated code

This repository makes use of a lot of generated code. In particular Protobuf files under `api/proxy` and Kubebuilder
itself. In order to avoid changes caused by different versions of these generators, this repository contains ways to
use specific pinned versions via the `Makefile`.

For consistency, it is recommended that a developer uses these instead of the tools on their local machine to generate
code:

- `make protobuf` To update Protobuf and gRPC generated Go code.
- `make manifests` To update the YAML manifests from Kubebuilder.
- `make generate` To update the Kubebuilder generated Go code.

## Testing

Testing is done using "vanilla" golang testing frameworks, with
[Testify require](https://godoc.org/github.com/stretchr/testify/require) for making assertions. Require fails as soon as
an assertion fails and so is preferred to [Testify assert](https://godoc.org/github.com/stretchr/testify/assert),
although assert may be used where appropriate.

Nested `t.Run` assertions are preferred, as they lead to clear test names. For example:

```go
func TestFoo(t *testing.T) {

    t.Run("TestClusterController", func(t *testing.T) {
        // Init Controller
        t.Run("OnCreation", func(t *testing.T) {
            // Create cluster
            t.Run("CreatesService", func(t *testing.T) {
                // Assert service exists
                t.Fail()
            })
        })
    })
}
```

Would produce test output like the following:

```
--- FAIL: TestFoo/TestClusterController/OnCreation/CreatesService (0.00s)
```

### Unit Tests

Generally speaking the controllers will not include unit tests as their behaviour will be verified by the Kubebuilder
tests. Sometimes a particular chunk of logic may be non-trivial to test in Kubebuilder tests, and therefore a unit test
may be more appropriate. In this case it is preferred to avoid mocking, in particular mocking the Kubernetes Go client,
and instead pull the logic to be tested into a pure function.

Other packages within the project which are not controllers (e.g., `internal/test/try`) should have unit tests in the
usual way.

Here are some examples of commands which run the unit tests:
 * `make test` to run all the unit tests
 * `make test ARGS="-v -run TestEtcdCluster_ValidateCreate"`to supply "go test" arguments such as `-run` or `-v`.

### Kubebuilder Tests

The test suite provided by Kubebuilder will execute a local copy of a Kubernetes API Server to test against. These tests
should focus on the API as exposed via the Kubernetes API. For example by creating an `EtcdPeer` resource and asserting
that a `Service` resource is created as a result. Pods in this test mode are never actually created, so the interaction
with `etcd` itself cannot be tested. These tests should additionally not 'reach in' to internal behaviour of the
controller, or directly communicate with the controller in any way.

### End to End tests

The end-to-end tests are under `internal/test/e2e`. These tests should be limited in scope and should only focus on
externally visible changes to etcd itself. This is to avoid the tests causing the implementation becoming too rigid.

For example an end-to-end test may create an `EtcdCluster` and assert that it can connect to it from inside the cluster
using the expected DNS name. Elements of the Kuberentes API that a user might interact with, such as the `status` field
on an `EtcdCluster` resource, may also be interacted with.

#### Kind

Here are some examples of commands which run the end-to-end tests:
 * `make e2e-kind` will create a new Kind cluster, run all the the end-to-end tests.
 * `make e2e-kind 'ARGS=-run TestE2E/Parallel/Webhooks'` will run a subset of the end-to-end tests.

NB If a Kind cluster with the name "etcd-e2e" already exists, that cluster will be re-used.

#### Minikube

You can also run the end-to-end tests in an existing cluster.
For example here's how you might run the tests in a [minikube](https://minikube.sigs.k8s.io/) cluster:

```
minikube start --cpus=4 --memory=4000mb
eval $(minikube docker-env)
make docker-build deploy e2e
```

This will create a minikube cluster with sufficient memory to build the Docker images,
and with sufficient CPU to run multiple Etcd clusters in parallel.
It will then build the operator Docker images, inside the `minikube` virtual machine
and deploy the operator, using those images.
Finally it runs the e2e tests.

The `make` command can be re-run after any code or configuration changes,
to build, push and deploy new Docker images and re-run the end-to-end tests.

#### GKE

To run the end-to-end tests on a GKE cluster, you would authenticate `docker` to your GCP private registry
and push the images to that registry before deploying the operator and running the tests.

```
gcloud container clusters create e2e --zone europe-west2-b --preemptible --num-nodes 4
gcloud container clusters get-credentials e2e --zone europe-west2-b
gcloud auth configure-docker
make DOCKER_REPO=gcr.io/my-project docker-build docker-push deploy e2e
```

This will create a 4-node GKE cluster and set KUBE_CONFIG to use that cluster.
It will also allow docker to authenticate to your GCP private Docker image repository.
It will build the Docker images and push them to the GCP private Docker image repository.
It will deploy the operator.
Finally it runs the end-to-end tests.

The `make` command can be re-run after any code or configuration changes
to build, push and deploy new Docker images and re-run the end-to-end tests.

### Static checks

You can run ``make verify`` to perform static checks on the code and to ensure that the manifest files are up to date.

## Release Process

Releases of the operator occur whenever the team feel there is sufficient new features to produce a release. The version
string will be prefixed with `v` and use semver.

### Pre-release tasks

* Sanity check documentation under `docs` and `README.md`.
* Compile and prepare release notes under `docs/release-notes/$VERSION.md`. For example versions v0.1.X would share a
file `docs/release-notes/v0.1.md`.
* Some of the release steps have been automated. Run `make --dry-run release VERSION=0.0.0` to see the steps that will be automatically performed.

### Process

1. Run `make release VERSION=0.1.0`. This will:
   1. Build Docker images with the supplied VERSION.
   2. Push Docker images to the repo defined in DOCKER_REPO.
   3. Create a `release-$VERSION.yaml` containing an example deployment for this version.
   4. Create an annotated Git tag for the supplied VERSION.
2. Push the tag to GitHub.
3. Mark the tag as a release on GitHub.
4. Build a Docker Image from the repository at that tag and push it to Docker Hub.
5. Build a version of the deployment YAML with the image tag, attach it to the GitHub release as a YAML file.
