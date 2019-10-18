# etcd-cluster-operator
A controller to deploy and manage ETCD clusters inside of Kubernetes.

## Introduction

etcd-cluster-operator is a Kubernetes controller for automating the bootstrapping, scaling and operation of ETCD clusters inside of Kubernetes.
It provides a CRD based API to allow developers to define their cluster layout in Kubernetes configuration resources, and manage their clusters with native Kubernetes tooling.

## Get started

```bash
# Clone the repository
git clone https://github.com/improbable-eng/etcd-cluster-operator

# Deploy the operator
kubectl apply -f examples/operator.yaml

# Deploy a 3-node ETCD cluster
kubectl apply -f examples/cluster.yaml

# Ensure the cluster has bootstrapped 
ETCDCTL_API=3 etcdctl --endpoints $SERVICE_IP member list
```

## Designs

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

