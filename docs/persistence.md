# Persistence

## Configuring Persistence Settings

The persistence settings for an `EtcdCluster` are configured with the `volumeClaimTemplate` field.
The value of this field should be a [PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.16/#persistentvolumeclaim-v1-core).

Here's an example of the configuration for a 3-node Etcd cluster where each Etcd node has 100Gi of local SSD storage.

```
apiVersion: etcd.improbable.io/v1alpha1
kind: EtcdCluster
metadata:
  name: my-cluster
  namespace: default
spec:
  replicas: 3
  volumeClaimTemplate:
    spec:
      accessModes: [ "ReadWriteOnce" ]
      storageClassName: "local-ssd"
      resources:
        requests:
          storage: 100Gi
```

**NOTE**: The `volumeClaimTemplate` field is immutable, because the ``improbable-eng/etcd-cluster-operator`` can not currently reconcile changes to the node storage settings.


## Operations

Here are some examples of day-to-day operations and how the Etcd data is handled in each case.

### Bootstrap a new cluster

Given the ``EtcdCluster`` above and assuming that this is a new cluster, the ``improbable-eng/etcd-cluster-operator`` will perform the following operations.

The operator will create new ``EtcdPeer``, ``PersistentVolumeClaim``, ``ReplicaSet`` resources for each ``EtcdCluster`` node, in the same namespace as the ``EtcdCluster``.
Here is an example of these resources for a single Etcd node:


```
apiVersion: etcd.improbable.io/v1alpha1
kind: EtcdPeer
metadata:
  name: my-cluster-0
  namespace: default
spec:
  clusterName: my-cluster
  volumeClaimTemplate:
    spec:
      accessModes: [ "ReadWriteOnce" ]
      storageClassName: "local-ssd"
      resources:
        requests:
          storage: 100Gi
```

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
if you reboot a Kubernetes node, what happens to the ``EtcdCluster`` and the pods and data that are on that server?

Kubernetes will delete the current `Pod` (it will be scheduled for deletion), but the PVC, the PV, will remain.

Kubernetes ``ReplicaSet`` controller will ensure that a new ``Pod`` is created, but it will remain un-schedulable until the Kubernetes node reboots.
This is because the Kubernetes Scheduler knows that the `Pod` placement is constrained by the location of the existing PVC and bound PV.

When the server has rebooted, Kubernetes will be able to schedule the ``Pod`` it will start up, with the existing data and rejoin the Etcd cluster.

**NOTE:** This operation   **does not**  require the ``improbable-eng/etcd-cluster-operator``to be running.
The Etcd peer ``ReplicaSets`` ensure that the cluster can function even without the operator which created them.

### Drain a Kubernetes Node

TODO

## FAQs

### Why doesn't ``improbable-eng/etcd-cluster-operator``  StatefulSet

TODO
