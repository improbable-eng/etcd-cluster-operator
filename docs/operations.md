# Operations Guide

This guide provides instructions aimed at Kubernetes cluster administrators who wish to manage etcd clusters. It
does not cover initial setup or installation of the operator, and instead assumes that it is already present and working
correctly.

The etcd cluster controller uses the Kubernetes API, via
[Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), to drive
administration of etcd clusters. You can administer your cluster manually using `kubectl`, or through any other
mechanism capable of modifying and viewing Kubernetes resources.

## Creating a new Cluster

To create a new cluster, create an `EtcdCluster` resource in the namespace you want the cluster's pods to run in. The
`spec` field of the resource is used to configure the desired properties of the cluster.

### Replicas

The `spec.replicas` field determines the number of pods that are run in the etcd cluster. It is [strongly suggested that
this be an odd number](https://etcd.io/docs/v3.4.0/faq/#why-an-odd-number-of-cluster-members). In etcd [adding more
replicas will degrade write performance](https://etcd.io/docs/v3.4.0/faq/#what-is-maximum-cluster-size). This can be `1`
for a testing environment, but for durability it is suggested that this is at least `3`. In most situations `5` is the
highest sensible setting, although neither etcd nor this operator impose a limit.

### Storage

The `spec.storage` field determines the storage options that will be used on the etcd pods. This configuration is highly
dependant on your environment but should be durable. For production use the [at least 80GiB of
storage](https://etcd.io/docs/v3.4.0/faq/#system-requirements) on each member is suggested.

Note that properties of the storage volumes, including size, cannot be amended after the cluster has been created.

There is a sample configuration file for a three pod etcd cluster at `config/samples/etcd_v1alpha1_etcdcluster.yaml`.
In this example each pod has 50Mi storage and uses a
[Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/#the-storageclass-resource) called
`standard`.


## Understanding Cluster Status

The `status` field of the `EtcdCluster` resource contains information about the running etcd cluster. This information
is 'best effort', and may be out of date in the case of a non-quorate etcd cluster or network disruption between the
operator pod and the etcd cluster.

This is an example status field

```yaml
status:
  members:
  - id: 51938e7f648adbc2
    name: my-cluster-2
  - id: 8eca1236bfa86a0a
    name: my-cluster-0
  - id: c1586ccb976e37c7
    name: my-cluster-1
  - id: c3080818d3bbf60b
    name: my-cluster-3
  - id: e1d0e0e643b65168
    name: my-cluster-4
  replicas: 5
```

This shows a five member etcd cluster. Showing not only the number of replicas but all of their names and internal IDs.

### Members

The `status.members` list shows the number of members as seen by etcd itself. In the case of early bootstrapping this
list may be blank (as the operator may have not established communication with the cluster yet). If the operator looses
contact with the cluster (e.g., due to network disruption) then this list will not be updated and therefore may be
stale.

### Replicas

The replicas list is the count of `EtcdPeer` resources managed by this cluster. See the [design
documentation](design/resources.md) for deeper discussion of peer resources and how the operator creates new peers. As a
result, during some operations (e.g., scale up, scale down, bootstrapping, etc.) this replicas count may be different
from the number of entries in the `status.members` and from the number of etcd pods currently running.

## Delete a Cluster

When you delete an `EtcdCluster` resource, the `etcd` data **will not** be deleted.
The `etcd-cluster-operator` **does not** set an `OwnerReference` on the `PersistentVolumeClaim` that it creates,
and this prevents `PersistentVolumeClaim` and the `PersistentVolume` resources being automatically garbage collected.
This is done deliberately, to avoid the risk of data loss if you accidentally delete an `EtcdCluster`.

When you delete an `EtcdCluster`, the `EtcdPeer`, `ReplicaSet`, `Pod`, and `Service` API objects *will* be deleted.
They are garbage collected because the etcd-cluster-operator does set an `OwnerReference` on these API objects.

### Restore a Deleted Cluster

You can recreate the deleted etcd cluster and restore its original data
by recreating an *identical*  `EtcdCluster` resource.
The etcd-cluster-operator will recreate the `EtcdPeer`, `ReplicaSet` and `Service` resources
and it will re-use the original `PersistentVolumeClaim` resources.

### Locate the Data for a Deleted Cluster

You can locate the data for a deleted `EtcdCluster` by using a label selector to locate all the the
`PersistentVolumeClaim` objects:

```bash
$ kubectl get pvc --selector etcd.improbable.io/cluster-name=cluster1
NAME         STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
cluster1-0   Bound    pvc-71543b02-54ac-49d2-ad7d-35601ab5d48f   1Mi        RWO            standard       3m34s
```

You can then examine the `PersistentVolume` and find out where its data is stored.

```bash
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

### Delete the Data for a Deleted Cluster

If you are sure that you no longer need the data for a deleted `EtcdCluster` you can delete the `PersistentVolumeClaim`
resources, which will allow the `PersistentVolume` resources to be automatically deleted, recycled or retained for
manual inspection and deletion later.

```bash
$ kubectl delete pvc --selector etcd.improbable.io/cluster-name=cluster1
persistentvolumeclaim "cluster1-0" deleted
```

The exact behaviour depends on the "Reclaim Policy" of the `PersistentVolume` and on the capabilities of the volume
provisioners which are being used in the cluster. You can read more about this in the Kubernetes documentation:
[Lifecycle of a volume and
claim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#lifecycle-of-a-volume-and-claim).

## Scale up a cluster

To increase the number of pods running etcd (a.k.a. "Scale Up") use `kubectl scale`. For example to scale the cluster
`my-cluster` in namespace `my-cluster-namespace` to five nodes:

```bash
$ kubectl --namespace my-cluster-namespace scale EtcdCluster my-cluster --replicas 5
```

The operator will automatically create new pods and begin data replication.

Any standard way of changing the underlying resource to declare more replicas will also work. For example by editing a
local YAML file for the cluster resource and running `kubectl apply -f my-cluster.yaml`.

## Scale down a cluster

To scale down a cluster you update the `EtcdCluster` setting a lower `.Spec.Replicas` field value. For example, you
might reduce the size of the sample cluster (used in the examples above) from 3-nodes to 1-node, by editing the
`EtcdCluster` manifest file, as follows:

```yaml
spec:
  replicas: 1
```

And then applying it:
```bash
$ kubectl apply -f config/samples/etcd_v1alpha1_etcdcluster.yaml
```

You could also use `kubectl scale` to do this, e.g.,

```bash 
$ kubectl scale etcdcluster my-cluster --replicas 1
```

### Scale-down operations

The `etcd-cluster-operator` will first connect to the etcd API and [remove one etcd member by runtime configuration of
the
cluster](https://github.com/etcd-io/etcd/blob/master/Documentation/op-guide/runtime-configuration.md#remove-a-member).
The member with the name containing largest ordinal will be removed first. So in the example above, "my-cluster-2" will
be removed first.

If "my-cluster-2" was the etcd leader, a leader election will take place and the cluster will briefly
[Leader Failure](https://github.com/etcd-io/etcd/blob/master/Documentation/op-guide/failures.md#leader-failure)
during which the cluster will *not be able to process write requests*.

The etcd process running in Pod "my-cluster-2" will exit with exit-code 0, and the Pod will be marked as "Complete".

Next, the operator will remove the `EtcdPeer` resource for the removed `etcd` member. This will trigger the deletion of
the `Replicaset` and the `Pod`  for "my-cluster-2", but **not** the `PersistentVolumeClaim` or the `PersistentVolume`,
for the reasons described in "### Delete a Cluster" (above).

When all these operations are complete the `EtcdCluster.Status` will be updated to show the new number of replicas
and the new list of `etcd` members.

Additionally, the `etcd-cluster-operator` will generate an `Event` for each operation it successfully performs,
which allows you to track the progress of the scale down operations.

If you want to scale up an `EtcdCluster` which has previously been scaled down,
you must first remove the `PersistentVolumeClaim` associated with any `EtcdPeer` that was removed during the scale down
operations. Otherwise new `EtcdPeer` and `Pods` will be started with `/var/lib/etcd` data corresponding to an old etcd
member ID, and the node will fail to join the cluster.
