# Peer Resource

A core component of the etcd cluster operator is the `EtcdPeer`
resource, or "peer resource". This is distinct from the `EtcdCluster`,
or "cluster resource", that a user will generally interact
with.

The peer resource represents a single peer in an etcd
cluster. Encoding it's bootstrap instructions, persistence settings,
and extra pod settings (e.g., overridden CPU limits). Its `status`
field exposes information about the peer including its hostname,
liveness status, the peer's current generation, and if it's currently
the leader of the etcd cluster.

All names used are deterministic, so for a cluster named
`test-cluster` the resources should be sequentially named, e.g.,
`test-cluster-1`, `test-cluster-2`, etc.

## Reconcile Behaviour

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

