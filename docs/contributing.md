# Development guide

## Testing

Testing is done using "Vanilla" golang testing frameworks, with [require](https://godoc.org/github.com/stretchr/testify/require) for making assertions.
Prefer to test a package's exported behaviour, rather than the implementation details.

### Test naming

Test names should be of the form `$testObject_$testAction_$expectedResult`, for example:
* `TestPeerController_OnCreation_CreatesReplicaSet`
* `TestClusterController_WithInalidConfig_FailsToApply`
