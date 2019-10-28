# etcd-cluster-operator

![Go Report Card](https://goreportcard.com/badge/github.com/improbable-eng/etcd-cluster-operator)

A controller to deploy and manage [etcd](https://etcd.io) clusters inside of Kubernetes.

## Introduction

etcd-cluster-operator is a Kubernetes controller for automating the bootstrapping, scaling, and operation of
etcd inside of Kubernetes. It provides a custom resource definition (CRD) based API to allow developers to define their
cluster in Kubernetes resources, and manage clusters with native Kubernetes tooling.

## Get started

To build and run the images yourself, you'll need a Docker registry that you have push access to and your Kubernetes
nodes have pull access from. First clone this git repository to your local disk.

You will need `make`, `docker`, `go`, `kustomize`, and `kubectl` installed.

### Building Docker Images

```bash
export IMG=my.registry.address/etcd-cluster-operator:latest
make docker-build
make docker-push
```

The actual build is performed inside Docker and will use the go version in the `Dockerfile`, but the `docker-build`
target will first run the tests locally which will use your system `go`.

The `IMG` environment variable is used by the `Makefile`.

### Deploying the Operator to a cluster

Make sure that your desired cluster is your default context in `kubectl`, and that `IMG` from above is still exported.

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

Use `make kind` to run the [Kubernetes in Docker (KIND)](https://github.com/kubernetes-sigs/kind) end to end tests. You
don't need to have the `kind` binary installed to run these, but you do need `docker`. These tests build the controller
Docker Image from the `Dockerfile`, push it into the KIND cluster, and then ensure that the etcd cluster actually comes
up.

## Design

Designs are documented in [docs/design](https://github.com/improbable-eng/etcd-cluster-operator/tree/master/docs/design).

## Contributing

Contribution guidelines are found in [docs/contributing.md](https://github.com/improbable-eng/etcd-cluster-operator/blob/master/docs/contributing.md)

## Status 

- [x] In-memory ETCD clusters
- [ ] Persisted data ETCD clusters
- [ ] Scaling up/down of existing clusters
- [ ] Upgrading existing clusters
- [ ] Backup/restore of cluster data
- [ ] TLS between cluster peers

## Alternatives

### [coreos/etcd-operator](https://github.com/coreos/etcd-operator)  

➕ Resizing  
➕ Failover  
➕ Rolling upgrades  
➖ No longer actively maintained  
➖ No data persistence  

### [cilium/cilium-etcd-operator](https://github.com/cilium/cilium-etcd-operator)

➕ Automated data compacting  
➕ TLS generation  
➖ Built on coreos/etcd-operator   

### [bitnami/charts/etcd](https://github.com/bitnami/charts/tree/master/bitnami/etcd)

➕ Stateful deployments  
➕ TLS support  
➕ Automated backups  
➖ No support for resizing  

