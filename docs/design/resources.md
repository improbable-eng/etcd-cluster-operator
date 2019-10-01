# Operator Resources

The etcd cluster operator extends the Kubernetes API with two new
resource definitions:

* `EtcdCluster`, or the "cluster resource", defines an entire
  operating cluster.
* `EtcdPeer`, or the "peer resource", defines a single peer of the
  etcd cluster. This principally concerns a Pod, but may also involve
  attached storage and network identity.

## The Peer Resource

The peer resource represents a single peer in an etcd
cluster. Encoding it's bootstrap instructions, persistence settings,
and extra pod settings (e.g., overridden CPU limits). Its `status`
field exposes information about the peer including its hostname,
liveness status, the peer's current generation, and if it's currently
the leader of the etcd cluster.

When reconciling the etcd peer resource the operator will do the
following:

1. If storage is enabled and a correctly named persistent volume claim
   (PVC) does not exist, create one with the settings provided on the
   peer resource.

   We never delete a persistent volume claim automatically, so this
   behaviour ensures that the operator will resume stopped clusters if
   the storage remains even if the peer resource was deleted.

   If the storage options on the peer resource are different from an
   existing PVC, the operator makes no attempt to 'correct' the
   preexisting PVC.
2. If a correctly named pod does not exist, an etcd pod will be
   launched with the PVC from above mounted as a volume if storage is
   enabled. The bootstrap configuration will always be provided to the
   pod as configuration, as etcd will ignore bootstrap configuration
   if a data directory already exists.
3. If the pod is in a ready state, the operator will attempt to
   contact it using the gRPC endpoint and retrieve information to
   populate the status field. Otherwise, it will report the peer as
   not-ready on the status field.

## The Cluster Resource

The cluster resource provides management of the entire cluster. It
will ensure that the correct number of `EtcdPeer` resources exist,
deploys a "headless" service to allow connectivity to the cluster, and
keeps its status field updated with the information about the cluster.

The operator treats the cluster itself as the canonical view of the
current state of the world. In particular the etcd cluster's peer list
is used to define what the expected members should be.

When reconciling the cluster resource the operator will do the
following:

1. If a headless service for the cluster does not exist, create it.
2. Attempt to dial the cluster using the headless service. If
   successfully connected update the `status` field with information
   from the cluster.
3. If we have communication with the cluster and the cluster is
   quorate, check that the number of peers the cluster expects is the
   same as the cluster resource's specified `size`. If the cluster has
   too few peers, add a new one using the API. If too many, remove
   exactly using the API.
3. List the `EtcdPeer` resources that are our children. If we have
   communication with the cluster reconcile the peer resources with
   the peer list in etcd. Peers that the exist in Kuberentes that the
   cluster does not know about should be deleted, peers that exist in
   the cluster and are not in Kubernetes should be created.

   If we don't have communication with the cluster (or the cluster is
   nonquorate) and we don't have enough peer resources compared with
   our desired size, then add peer resources to make up the expected
   size.
4. Update the status resource based on our communication with the
   cluster. If the cluster cannot be reached we mark ourselves as not
   ready. If the cluster is not quorate we mark as non quorate.

## Common operations

### Bootstrapping

1. A user creates a `EtcdCluster` resource with size 3.
2. Using the logic above, the cluster operator creates a headless
   service, fails to dial the cluster, and creates three peer
   resources.
3. Each peer resource creates a persistent volume claim and launches a
   pod, mounting the volume claim.
4. The pods run etcd, bootstrap to each other, and then form a
   cluster.
5. The cluster resource eventually is able to communicate with the
   cluster and comes up.

