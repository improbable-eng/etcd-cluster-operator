// Package e2e holds an end-to-end test framework for testing the operator. The goal is for it to deploy the operator
// and some custom resources in to a kubernetes cluster, and observe that a reachable ETCD cluster is created.
// Flags include:
//   * `--kind': starts a local Kind cluster to run the tests against
//   * `--current-context': runs the tests against the local kubernetes context. This context must contain a running
//      operator & have custom resource definitions applied.
//   * `--repo-root': path to the root of the etcd-cluster-operator repo.
//   * `--cleanup': defaulting to true, specifies if the Kind cluster should be automatically destroyed when the test
//      finishes. It can be useful to disable this to debug why the e2e tests are failing.
package e2e
