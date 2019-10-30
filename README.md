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

### Deploying webhooks / Installing Cert-Manager

This Operator uses [defaulting and validating webhooks](https://book.kubebuilder.io/reference/webhook-overview.html)
to validate any `EtcdCluster` and `EtcdPeer` custom resources that you create,
and to assign default values for any optional fields in those APIs.

**You are strongly advised to install these webhooks** in order to prevent unsupported `EtcdCluster` configurations,
and to prevent unsupported configuration changes.
[The webhook APIs used by this Operator were introduced in Kubernetes 1.9](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG-1.9.md#api-machinery).

Running `make deploy`  will add [Webhook configuration](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#webhook-configuration) such that,
for any CREATE or UPDATE operation on any `EtcdCluster` or `EtcdPeer` custom resource,
the Kubernetes API server will initiate an HTTPS webhook request to the `controller-manager`,
so that it can perform defaulting and validation of the resource before it is stored by the Kubernetes API server.

The API server connects to the webhook server using HTTPS
and this requires SSL certificates to be configured for the client and the server.
The easiest way to set this up is to [install Cert-Manager](https://book.kubebuilder.io/cronjob-tutorial/running-webhook.html#cert-manager)
before you deploy the Operator.
The `config/default/` directory contains Kustomize patches which add [Cert-Manager cainjector annotations](https://docs.cert-manager.io/en/latest/reference/cainjector.html) to the webhook configuration,
and a [self signing Issuer](https://docs.cert-manager.io/en/latest/tasks/issuers/setup-selfsigned.html).
With these, Cert-manager will automatically generate self-signed certificates for the webhook client and server.

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

