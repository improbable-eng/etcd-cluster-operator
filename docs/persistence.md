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


## Bootstrap Operations

Given the ``EtcdCluster`` above and assuming that this is a new cluster, the ``improbable-eng/etcd-cluster-operator`` will perform the following operations:

### Create PersistentVolumeClaim

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

## Restart Operations

TODO

## FAQs

### Why doesn't ``improbable-eng/etcd-cluster-operator``  StatefulSet

TODO
