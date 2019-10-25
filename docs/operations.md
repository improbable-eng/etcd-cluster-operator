# Operations

## Configuring an `EtcdCluster`

There is a configuration file for a 3-node Etcd cluster at `config/samples/etcd_v1alpha1_etcdcluster.yaml`.
In this example each Etcd node has 50Mi storage and uses a [Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/#the-storageclass-resource) called "standard".

**NOTE**: The `etcd-cluster-operator` can not currently reconcile changes to the storage settings.
Do not make changes to this field after you have applied the manifest.
In future this field will be made immutable. See https://github.com/improbable-eng/etcd-cluster-operator/issues/49

## Examples

Here are some examples of day-to-day operations and explanations of how Etcd data is handled in each case.

### Bootstrap a New Cluster

Apply the the manifest above:

```
kubectl apply -f config/samples/etcd_v1alpha1_etcdcluster.yaml
```

The `etcd-cluster-operator` will create the following new API resources for each Etcd node: `EtcdPeer`, `PersistentVolumeClaim` and a `ReplicaSet`.

Kubernetes will then dynamically provision a `PersistentVolume` of 50Mi,
using the [Provisioner](https://kubernetes.io/docs/concepts/storage/storage-classes/#provisioner) of the `StorageClass` called "standard".
It will "bind" that `PersistentVolume` to the `PersistentVolumeClaim`.

Kubernetes will also create a `Pod` based on the `ReplicaSet` template.
This `Pod` will remain unscheduled until the `PersistentVolumeClaim` is bound.
It will then be scheduled and started on the same Kubernetes node as the `PersistentVolume`.

Eventually, you will have an empty Etcd cluster of three nodes, with a total of 50Mi storage available.

### Reboot a Kubernetes Node

How do you **reboot** a Kubernetes node, without disrupting the Etcd cluster?

**NOTE:** This operation **does not** require the `etcd-cluster-operator` to be running.
The `ReplicaSet` for each Etcd node ensures that the cluster can function even without the operator which created them.

**NOTE:** Ensure that your Etcd cluster has sufficient peers on other Kubernetes nodes, to maintain quorum.

1. Reboot the Kubernetes node
   1. Kubernetes will schedule the current `Pod` for deletion, but the `PersistentVolumeClaim` and the `PersistentVolume` will remain.
   2. Kubernetes will create a new `Pod`, but it will remain un-schedulable until the Kubernetes node reboots.
      1. This is because the Kubernetes Scheduler knows that the `Pod` placement is constrained
         by the location of the existing `PersistentVolumeClaim`and its bound `PersistentVolume`.
   3. When the server has rebooted, Kubernetes will be able to schedule the `Pod` and it will start.
      The Etcd process inside the `Pod` will use the existing data and it will rejoin the Etcd cluster.
