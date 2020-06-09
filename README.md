# Etcd Cluster Operator

![Go Report Card](https://goreportcard.com/badge/github.com/improbable-eng/etcd-cluster-operator)
[![Improbable Engineering](https://circleci.com/gh/improbable-eng/etcd-cluster-operator.svg?style=shield)](https://app.circleci.com/github/improbable-eng/etcd-cluster-operator/pipelines)

Etcd Cluster Operator is an [Operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator) for automating
the creation and management of etcd inside of Kubernetes. It provides a
[custom resource definition (CRD)](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources)
based API to define etcd clusters with Kubernetes resources, and enable management with native Kubernetes tooling.

## Quick Start

See the [installation instructions](docs/installing.md) for installing the operator to your Kubernetes cluster. If you
want to experiment with the operator, considering using [kind](https://github.com/kubernetes-sigs/kind) to run a local
cluster.

Once installed you can create an etcd cluster by using `kubectl` to apply an `EtcdCluster`.

```yaml
apiVersion: etcd.improbable.io/v1alpha1
kind: EtcdCluster
metadata:
  name: my-first-etcd-cluster
spec:
  replicas: 3
  version: 3.2.30
```

## Further Reading

* See the [operations guide](docs/operations.md) for full details on the options available and how to perform further
  tasks.
* See [design documentation](docs/design) for background reading.
* See the [release notes](docs/release-notes) for feature update information.
* See the [contributing guide](docs/contributing.md) for details on how to get involved.

