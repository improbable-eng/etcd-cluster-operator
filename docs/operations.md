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

### Delete a Cluster

When you delete an `EtcdCluster` resource, the `etcd` data **will not** be deleted.
The `etcd-cluster-operator` **does not** set an `OwnerReference` on the `PersistentVolumeClaim` that it creates,
and this prevents `PersistentVolumeClaim` and the `PersistentVolume` resources being automatically garbage collected.
This is done deliberately, to avoid the risk of data loss if you accidentally delete an `EtcdCluster`.

When you delete an `EtcdCluster`, the `EtcdPeer`, `ReplicaSet`, `Pod`, and `Service` API objects *will* be deleted.
They are garbage collected because the etcd-cluster-operator does set an `OwnerReference` on these API objects.

#### Restore a Deleted Cluster

You can recreate the deleted etcd cluster and restore its original data
by recreating an *identical*  `EtcdCluster` resource.
The etcd-cluster-operator will recreate the `EtcdPeer`, `ReplicaSet` and `Service` resources
and it will re-use the original `PersistentVolumeClaim` resources.

#### Locate the Data for a Deleted Cluster

You can locate the data for a deleted `EtcdCluster` by using a label selector to locate all the the `PersistentVolumeClaim` objects:

```
$ kubectl get pvc --selector etcd.improbable.io/cluster-name=cluster1
NAME         STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
cluster1-0   Bound    pvc-71543b02-54ac-49d2-ad7d-35601ab5d48f   1Mi        RWO            standard       3m34s

```

You can then examine the `PersistentVolume` and find out where its data is stored.

```
$ kubectl describe pv pvc-71543b02-54ac-49d2-ad7d-35601ab5d48f
Name:            pvc-71543b02-54ac-49d2-ad7d-35601ab5d48f
Labels:          <none>
Annotations:     kubernetes.io/createdby: hostpath-dynamic-provisioner
                 pv.kubernetes.io/bound-by-controller: yes
                 pv.kubernetes.io/provisioned-by: kubernetes.io/host-path
Finalizers:      [kubernetes.io/pv-protection]
StorageClass:    standard
Status:          Bound
Claim:           teste2e-parallel-persistence/cluster1-0
Reclaim Policy:  Delete
Access Modes:    RWO
VolumeMode:      Filesystem
Capacity:        1Mi
Node Affinity:   <none>
Message:
Source:
    Type:          HostPath (bare host directory volume)
    Path:          /tmp/hostpath_pv/1026efed-5405-4112-82e2-c06951f64017
    HostPathType:
Events:            <none>
```

#### Delete the Data for a Deleted Cluster

If you are sure that you no longer need the data for a deleted `EtcdCluster` you can delete the `PersistentVolumeClaim` resources,
which will allow the `PersistentVolume` resources to be automatically deleted, recycled or retained for manual inspection and deletion later.

```
$ kubectl delete pvc --selector etcd.improbable.io/cluster-name=cluster1
persistentvolumeclaim "cluster1-0" deleted
```

The exact behaviour depends on the "Reclaim Policy" of the `PersistentVolume` and on the capabilities of the volume provisioners which are being used in the cluster.
You can read more about this in the Kubernetes documentation:  [Lifecycle of a volume and claim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#lifecycle-of-a-volume-and-claim).


### Scale down a cluster

How do you scale down an `EtcdCluster` and what operations does the `etcd-cluster-operator` perform?

To scale down a cluster you update the `EtcdCluster` setting a lower `.Spec.Replicas` field value.
For example, you might reduce the size of the sample cluster (used in the examples above) from 3-nodes to 1-node,
by editing the `EtcdCluster` manifest file, as follows:

```
...
spec:
  replicas: 1
...
```

And then applying it:
```
kubectl apply -f config/samples/etcd_v1alpha1_etcdcluster.yaml
```

You could also use `kubectl scale` to do this. E.g.

```
kubectl scale etcdcluster my-cluster --replicas 1
```

#### Scale-down operations

The `etcd-cluster-operator` will first connect to the Etcd API and [remove one Etcd member by runtime configuration of the cluster](https://github.com/etcd-io/etcd/blob/master/Documentation/op-guide/runtime-configuration.md#remove-a-member).
The member with the name containing largest ordinal will be removed first.
So in the example above, "my-cluster-2" will be removed first.

If "my-cluster-2" was the `etcd` leader, a leader election will take place and the cluster will briefl
[Leader Failure](https://github.com/etcd-io/etcd/blob/master/Documentation/op-guide/failures.md#leader-failure)
during which the cluster will *not be able to process write requests*.

The `etcd` process running in Pod "my-cluster-2" will exit with exit-code 0,
and the Pod will be marked as "Complete".

Next, the `etcd-cluster-operator` will remove the `EtcdPeer` resource for the removed `etcd` member.
This will trigger the deletiong of the `Replicaset` and the `Pod`  for "my-cluster-2",
but **not** the `PersistentVolumeClaim` or the `PersistentVolume`,
for the reasons described in "### Delete a Cluster" (above).

When all these operations are complete the `EtcdCluster.Status` will be updated to show the new number of replicas
and the new list of `etcd` members.

Additionally, the `etcd-cluster-operator` will generate an `Event` for each operation it successfully performs,
which allows you to track the progress of the scale down operations.

If you want to scale up an `EtcdCluster` which has previously been scaled down,
you must first remove the `PersistentVolumeClaim` associated with any `EtcdPeer` that was removed during the scale down operations.
Otherwise new `EtcdPeer` and `Pods` will be started with `/var/lib/etcd` data corresponding to an old `etcd` member ID,
and the node will fail to join the cluster.
