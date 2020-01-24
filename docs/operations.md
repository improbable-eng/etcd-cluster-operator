# Operations Guide

This guide provides instructions aimed at Kubernetes cluster administrators who wish to manage etcd clusters. It
does not cover initial setup or installation of the operator, and instead assumes that it is already present and working
correctly. Please see [the project README](../README.md) for installation instructions.

The etcd cluster controller uses the Kubernetes API, via
[Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), to drive
administration of etcd clusters. You can administer your cluster manually using `kubectl`, or through any other
mechanism capable of modifying and viewing Kubernetes resources.

## Creating a new Cluster

To create a new cluster, create an `EtcdCluster` resource in the namespace you want the cluster's pods to run in. The
`spec` field of the resource is used to configure the desired properties of the cluster.

### Replicas

The `spec.replicas` field determines the number of pods that are run in the etcd cluster.

For availability reasons it is [strongly suggested that this be an odd
number](https://etcd.io/docs/v3.4.0/faq/#why-an-odd-number-of-cluster-members), and an etcd cluster with an even number
of replicas is actually worse for availability than a cluster one smaller to be odd. Note also that [adding more
replicas will degrade write performance](https://etcd.io/docs/v3.4.0/faq/#what-is-maximum-cluster-size).

This can be `1` for a testing environment, but for durability it is suggested that this is at least `3`. In most
situations `5` is the highest sensible setting, although neither etcd nor this operator impose a limit.

### Storage

The `spec.storage` field determines the storage options that will be used on the etcd pods. This configuration is highly
dependant on your environment but should be durable. For production use the [at least 80GiB of
storage](https://etcd.io/docs/v3.4.0/faq/#system-requirements) on each member is suggested.

Note that properties of the storage volumes, including size, cannot be amended after the cluster has been created.

There is a sample configuration file for a three pod etcd cluster at `config/samples/etcd_v1alpha1_etcdcluster.yaml`.
In this example each pod has 50Mi storage and uses a
[Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/#the-storageclass-resource) called
`standard`.

### Pod Template

The `spec.podTemplate` field can be optionally used to specify annotations and resource requirements that should be applied to the underlying pods
running etcd. This can be used to configure annotations for Prometheous metrics, or any other requirement. Note that
annotation names prefixed with `etcd.improbable.io/` are reserved, and cannot be applied with this feature.

Note that the pod template cannot be changed once the cluster has been created.

#### Configuring Prometheus annotations

The etcd pods expose metrics in Prometheus' standard format. If you use annotation-based metrics discovery in your
cluster, you can apply the following to the `EtcdCluster`:

```yaml
spec:
  podTemplate:
    metadata:
      annotations:
        "prometheus.io/path":   "/metrics",
        "prometheus.io/scrape": "true",
        "prometheus.io/scheme": "http",
        "prometheus.io/port":   "2379",
```

#### Configuring Resource Requirements

You can set [resource requests and limits](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/) for the etcd container which runs inside each `EtcdPeer` Pod.
In the sample configuration file, the resource requests are set as follows:

```yaml
spec:
  podTemplate:
    resources:
      requests:
        cpu: 200m
        memory: 200Mi
      limits:
        cpu: 200m
        memory: 200Mi
```

In a production cluster you should set these requests higher;
refer to the [Etcd Hardware Recommendations](https://github.com/etcd-io/etcd/blob/release-3.4/Documentation/op-guide/hardware.md).

If you supply a CPU limit, the `etcd-cluster-operator` will also set an [environment variable named `GOMAXPROCS`](https://golang.org/pkg/runtime/#hdr-Environment_Variables)
which governs the number of threads used by the Golang runtime running the Etcd process.
The minimum value is 1 and if the CPU limit is greater than 1 core, the value will be the rounded down to the nearest integer.

Alternatively, you can omit the resource requirements altogether and rely on [Limit Ranges](https://kubernetes.io/docs/concepts/policy/limit-range/) to set default requests and limits to the containers that are created by the operator.

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

Next, the operator will remove the `EtcdPeer` resource for the removed `etcd` member.
This will trigger the deletion of the `ReplicaSet` and the `Pod` and the `PersistentVolumeClaim` for "my-cluster-2".
The `PersistentVolume` (and the data) for the removed `etcd` node *may* be deleted
depending on the "Reclaim Policy" of the `StorageClass` associated with the `PersistentVolume`.

When all these operations are complete the `EtcdCluster.Status` will be updated to show the new number of replicas
and the new list of `etcd` members.

Additionally, the `etcd-cluster-operator` will generate an `Event` for each operation it successfully performs,
which allows you to track the progress of the scale down operations.

### Backup a cluster

The operator includes two controllers to help taking scheduled backups of etcd data, responding to `EtcdBackup` and `EtcdBackupSchedule` resources.

#### The `EtcdBackup` resource

A backup can be taken at any time by deploying an `EtcdBackup` resource:

```
$ kubectl apply -f config/samples/etcd_v1alpha1_etcdbackup.yaml
```

When the operator detects this resource has been applied, it will take a snapshot of the etcd state from the given cluster endpoints.
This snapshot is then uploaded to the given destination where it can be persisted.

#### The `EtcdBackupSchedule` resource

Backups can be scheduled to be taken at given intervals:

```
$ kubectl apply -f config/samples/etcd_v1alpha1_etcdbackupschedule.yaml
```

The resource specifies a crontab-style schedule defining how often the backup should be taken.
It includes a spec similar to the  `EtcdBackup` resource to define how the backup should be taken, and where it should be placed.

## Frequently Asked Questions

### Why aren't there Liveness Probes for the Etcd Pods?

Etcd exits with an non-zero exit status if it encounters unrecoverable errors
or if it fails to join the cluster.
And we do not know of any Etcd deadlock conditions.
So the Liveness Probe seems unnecessary.

And furthermore [Liveness Probes may cause more problems than they solve](https://srcco.de/posts/kubernetes-liveness-probes-are-dangerous.html).
In [our experiments](https://github.com/improbable-eng/etcd-cluster-operator/pull/83#issuecomment-561111653),
using the a [Liveness Probe based on the Etcd health endpoint, as configured by Kubeadm](https://github.com/kubernetes/kubernetes/pull/81385),
the Liveness Probe regularly failed:
during scaling operations due to cluster leader elections, and
at times of high network latency between Etcd peers.
This caused Etcd containers to be restarted, which made the situation even worse.

If you disagree with this or if you find a valid use-case for Liveness Probes,
please [create an issue](https://github.com/improbable-eng/etcd-cluster-operator/issues).

### Why aren't there Readiness Probes for the Etcd Pods?

Our current thinking is that Readiness Probes are unnecessary
because we assume that clients will connect to multiple Etcd nodes via a Headless Service
and perform their own health checks.

Additionally, it's not clear how to configure the Readiness Probe.
An HTTP Readiness Probe, configured to GET the Etcd health endpoint would fail whenever the cluster was unhealthy,
and all Etcd Pod Readiness Probes would fail at the same time.
A client connecting to the Service for these pods would have to deal with empty DNS responses
because all the Endpoints for the service would be removed when the Readiness Probe failed.

If you disagree with this or if you find a valid use-case for Readiness Probes,
please [create an issue](https://github.com/improbable-eng/etcd-cluster-operator/issues).
