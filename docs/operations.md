# Operations

## Configuring an `EtcdCluster`

There is an example of the configuration for a 3-node Etcd cluster in `config/samples/etcd_v1alpha1_etcdcluster.yaml`.
In this example each Etcd node has 50Mi of "standard" storage.

**NOTE**: The `etcd-cluster-operator` can not currently reconcile changes to the storage settings.
Do not make changes to this field after you have applied the manifest.
In future this field will be made immutable. See https://github.com/improbable-eng/etcd-cluster-operator/issues/49

## Examples

Here are some examples of day-to-day operations and explanations of Etcd data is handled in each case.

### Bootstrap a New Cluster

Apply the the manifest above, using `kubectl apply -f `.

The `etcd-cluster-operator` will create a the following API resources for each Etcd node:

1. EtcdPeer
2. PersistentVolumeClaim
3. ReplicaSet

Kubernetes will then dynamically provision a `PersistentVolume` of 50Mi,
using the [Provisioner](https://kubernetes.io/docs/concepts/storage/storage-classes/#provisioner) of "default" `StorageClass`.
It will "bind" that `PersistentVolume` to the `PersistentVolumeClaim` above.

Kubernetes will also create a `Pod` based on the `ReplicaSet` template above.
This `Pod` will remain unscheduled until the `PersistentVolumeClaim` is bound.
It will then be scheduled and started on the same Kubernetes node as the PV.

Eventually, you will have an empty Etcd cluster of three nodes, with a total of 300Gi SSD storage available.

### Reboot a Kubernetes Node

How do you **reboot** a Kubernetes node, without disrupting the `EtcdCluster`?

**NOTE:** This operation **does not** require the `etcd-cluster-operator` to be running.
The Etcd peer `ReplicaSet` ensures that the cluster can function even without the operator which created them.

**NOTE:** Ensure that your `EtcdCluster` has sufficient peers on other Kubernetes nodes, to maintain quorum.

1. Reboot the Kubernetes node
   1. Kubernetes will schedule the current `Pod` for deletion, but the `PersistentVolumeClaim` and the `PersistentVolume` will remain.
   1. Kubernetes will create a new `Pod`, but it will remain un-schedulable until the Kubernetes node reboots.
      1. This is because the Kubernetes Scheduler knows that the `Pod` placement is constrained
         by the location of the existing `PersistentVolumeClaim`and its bound `PersistentVolume`.
   1. When the server has rebooted, Kubernetes will be able to schedule the `Pod` and it will start.
      The Etcd process inside the `Pod` will use the existing data and it will rejoin the Etcd cluster.
