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
			//create cluster
			t.Run("CreatesService", func(t *testing.T) {
				//assert service exists
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

Unit tests should only be written for pure functions, and should not mock things like contexts or the Go Kubernetes
client. Due to the nature of the project, unit tests are relatively rare, but encouraged for pure functions that do a
lot of work or handle corner cases.

### Kubebuilder Tests

The test suite provided by kubebuilder will execute a local copy of a Kubernetes API Server to test against. These tests
should focus on the API as exposed via the Kubernetes API. For example by creating an `EtcdPeer` resource and asserting
that a `Service` resource is created as a result. Pods in this test mode are never actually created, so the interaction
with `etcd` itself cannot be tested. These tests should additionally not 'reach in' to internal behaviour of the
controller, or directly communicate with the controller in any way.

### End to End tests

The end to end tests run, by default, using Kubernetes in Docker (KIND) and are capable of actually executing `etcd` 
pods. These tests are under `internal/test/e2e`. These tests should focus on externally visible changes to etcd itself.
For example creating an `EtcdCluster` and asserting that it can connect to it from inside the cluster using the expected
DNS name. Elements of the Kuberentes API that a user might interact with, such as the `status` field on an `EtcdCluster`
resource, may also be interacted with.
