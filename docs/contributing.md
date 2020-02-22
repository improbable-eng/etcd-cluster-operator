# Development guide

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

#### Kind

The end to end tests run, by default, using Kubernetes in Docker (KIND) and are capable of actually executing `etcd`
pods. These tests are under `internal/test/e2e`. These tests should be limited in scope and should only focus on
externally visible changes to etcd itself. This is to avoid the tests causing the implementation becoming too rigid.

For example an end to end test may create an `EtcdCluster` and assert that it can connect to it from inside the cluster
using the expected DNS name. Elements of the Kuberentes API that a user might interact with, such as the `status` field
on an `EtcdCluster` resource, may also be interacted with.

Here are some examples of commands which run the end-to-end tests:
 * `make e2e-kind` will create a new Kind cluster, run all the the end-to-end tests and delete the cluster when the tests are complete.
 * `make e2e-kind CLEANUP=false` will create a Kind cluster, run the tests and leave the cluster running after the tests complete.
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

### Process

1. Tag the repository with the new version, e.g., `git tag v0.1.0` and push the tag to GitHub.
2. Mark the tag as a release on GitHub.
3. Build a Docker Image from the repository at that tag and push it to Docker Hub.
4. Build a version of the deployment YAML with the image tag, attach it to the GitHub release as a YAML file.
