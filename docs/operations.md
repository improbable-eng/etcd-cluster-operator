# Persistence

## Configuring Persistence Settings

The persistence settings for an `EtcdCluster` are configured with the `storage` field.

Here's an example of the configuration for a 3-node Etcd cluster where each Etcd node has 100Gi of local SSD storage.

```
apiVersion: etcd.improbable.io/v1alpha1
kind: EtcdCluster
metadata:
  name: my-cluster
  namespace: default
spec:
  replicas: 3
  storage:
    volumeClaimTemplate:
      storageClassName: "local-ssd"
      resources:
        requests:
          storage: 100Gi
```

**NOTE**: The `storage` field is immutable, because the ``improbable-eng/etcd-cluster-operator`` can not currently reconcile changes to the node storage settings.

## Operations

Here are some examples of day-to-day operations and how the Etcd data is handled in each case.

### Bootstrap a new cluster

Assuming you are using [Local Persistent Volumes](https://kubernetes.io/docs/concepts/storage/volumes/#local),
which are contstrained to a particular Kubernetes node.
how do you **bootstrap** a new ``EtcdCluster`` ??

1. On each target Kubernetes node, create a `/mnt/local-ssd` directory
1. Mount at least one 100Gi SSD partition at e.g. `/mnt/local-ssd/<partition-UUID>`
1. Configure and deploy the [Local Static Provisioner](https://github.com/kubernetes-sigs/sig-storage-local-static-provisioner/blob/master/docs/getting-started.md), with a `classes` configuration which contains a `local-ssd` name and which  scans the ``/mnt/local-ssd` directory.
1. Wait for a StorageClass called `local-ssd`
1. Wait for unbound PVs with this class to appear in the Kubernetes cluster.
1. `kubectl apply -f ` the manifest above.

The operator will create a new a collection of resources for each Etcd node:

#### EtcdPeer

```
apiVersion: etcd.improbable.io/v1alpha1
kind: EtcdPeer
metadata:
  name: my-cluster-0
  namespace: default
spec:
  clusterName: my-cluster
  storage:
    volumeClaimTemplate:
      storageClassName: "local-ssd"
      resources:
        requests:
          storage: 100Gi
```

#### PersistentVolumeClaim

```
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-cluster-0
  namespace: default
spec:
  storageClassName: "local-ssd"
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 100Gi
```

#### ReplicaSet

```
apiVersion: v1
kind: ReplicaSet
metadata:
  name: my-cluster-0
  namespace: default
spec:
  podTemplate:
    volumes:
      - name: etcd-storage
        persistentVolumeClaim:
          claimName: my-cluster-0
    containers:
      - name: etcd
        volumeMounts:
          - mountPath: "/var/lib/etcd"
            name: etcd-storage
```

Kubernetes will then dynamically provision a ``PersistentVolume`` of 100Gi backed by an SSD,
and "bind" that PV to the PVC above.
(or it will bind per-provisioned PV to that PVC).

Kubernetes will also create a ``Pod`` based on the ``ReplicaSet`` template above.

This ``Pod`` will remain unscheduled until the PVC is bound to a PV.
It will then be scheduled and started on the same Kubernetes node as the PV.

Eventually, you will have an empty Etcd cluster of three nodes, with a total of 300Gi SSD storage available.

### Reboot a Kubernetes Node

Assuming you are using [Local Persistent Volumes](https://kubernetes.io/docs/concepts/storage/volumes/#local),
which are contstrained to a particular Kubernetes node.
how do you **reboot** a Kubernetes node, without disrupting the ``EtcdCluster`` ??

**NOTE:** This operation   **does not**  require the ``improbable-eng/etcd-cluster-operator``to be running.
The Etcd peer ``ReplicaSets`` ensure that the cluster can function even without the operator which created them.

**NOTE:** Ensure that your ``EtcdCluster`` has sufficient peers on other Kubernetes nodes, to maintain quorum.

1. Reboot the Kubernetes node
   1. Kubernetes will delete the current `Pod` (it will be scheduled for deletion), but the PVC, the PV, will remain.
   1. Kubernetes ``ReplicaSet`` controller will ensure that a new ``Pod`` is created, but it will remain un-schedulable until the Kubernetes node reboots.
   1. This is because the Kubernetes Scheduler knows that the `Pod` placement is constrained by the location of the existing PVC and bound PV.
   1. When the server has rebooted, Kubernetes will be able to schedule the ``Pod`` it will start up, with the existing data and rejoin the Etcd cluster.

### Decommission a Kubernetes Node that has an EtcdPeer

Assuming you are using [Local Persistent Volumes](https://kubernetes.io/docs/concepts/storage/volumes/#local),
which are contstrained to a particular Kubernetes node.
how do you **decommission** a Kubernetes node, without disrupting the ``EtcdCluster``?

**NOTE:** This operation  **requires** the ``improbable-eng/etcd-cluster-operator``to be running.

**NOTE:** Ensure that your ``EtcdCluster`` has sufficient peers on other Kubernetes nodes, to maintain quorum.

1. Add a new Kubernetes node somewhere that supports the StorageClass used by Etcd.
1. Cordon the old Kubernetes node.
1. Scale out the Etcd cluster by one.
1. Wait for the new Etcd peer to be up and healthy on the new Kubernetes node.
1. `kubectl patch` the the old EtcdPeer resource to mark it as decommissioned.
   1. EtcdPeer Replicaset will be scaled to 0
   1. EtcdPeer pod will be scheduled for deletion and then be deleted.
   1. EtcdPeer PVC and PV **will not** be deleted.
1. Wait for the EtcdPeer.Status to report that it has been successfully decommissioned.
1. Drain the old Kubernetes node.
1. Shutdown the old Kubernetes node.
1. Delete the PV and PVC

## FAQs

### Why doesn't ``improbable-eng/etcd-cluster-operator``  StatefulSet

TODO
