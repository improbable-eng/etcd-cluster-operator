# Development guide

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

### Kubebuilder Tests

The test suite provided by Kubebuilder will execute a local copy of a Kubernetes API Server to test against. These tests
should focus on the API as exposed via the Kubernetes API. For example by creating an `EtcdPeer` resource and asserting
that a `Service` resource is created as a result. Pods in this test mode are never actually created, so the interaction
with `etcd` itself cannot be tested. These tests should additionally not 'reach in' to internal behaviour of the
controller, or directly communicate with the controller in any way.

### End to End tests

The end to end tests run, by default, using Kubernetes in Docker (KIND) and are capable of actually executing `etcd` 
pods. These tests are under `internal/test/e2e`. These tests should be limited in scope and should only focus on
externally visible changes to etcd itself. This is to avoid the tests causing the implementation becoming too rigid.

For example an end to end test may create an `EtcdCluster` and assert that it can connect to it from inside the cluster
using the expected DNS name. Elements of the Kuberentes API that a user might interact with, such as the `status` field
on an `EtcdCluster` resource, may also be interacted with.

## Release Process

Releases of the operator occur whenever the team feel there is sufficient new features to produce a release. The version
string will be prefixed with `v` and use semver.

### Pre-release tasks

* Sanity check documentation under `docs` and `README.md`.
* Compile and prepare release notes under `docs/release-notes/$VERSION.md`. For example version v0.1.0 would have a file
  `docs/release-notes/v0.1.0.md`.

### Process

1. Tag the repository with the new version, e.g., `git tag v0.1.0` and push the tag to GitHub.
2. Mark the tag as a release on GitHub.
3. Build a Docker Image from the repository at that tag and push it to Docker Hub.
4. Build a version of the deployment YAML with the image tag, attach it to the GitHub release as a YAML file.