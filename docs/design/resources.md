# Operator Resources

The etcd cluster operator extends the Kubernetes API with two new
resource definitions:

* `EtcdCluster`, or the "cluster resource", defines an entire
  operating cluster.
* `EtcdPeer`, or the "peer resource", defines a single peer of the
  etcd cluster. This principally concerns a Pod, but also includes
  attached storage and network identity.

## Operator Deployment

The operator is compiled into a single binary
(`etcd-cluster-operator`) and deployed as a single Docker image
(`improbable/etcd-cluter-operator` on Docker Hub). This single
operator contains the controllers for both the peer resource and the
cluster resource, and is multi-tennant for clusters defined in any
namespace.

## The Peer Resource

The peer resource represents a single peer in an etcd cluster.
Encoding its bootstrap instructions, persistence settings, and extra
pod settings (e.g., overridden CPU limits). Its `status` field exposes
information about the peer including its hostname. In future the
status may also include the liveness status if it's currently the
leader of the etcd cluster.

When reconciling the etcd peer resource the operator will do the
following:

1. If a correctly named persistent volume claim (PVC) does not exist,
   create one with the settings provided on the peer resource.

   We never delete a persistent volume claim automatically, so this
   behaviour ensures that the operator will resume stopped clusters if
   the storage remains even if the peer resource was deleted. In
   future, automatic cluster recovery tools **may** delete PVC
   resources if they determine it is required and safe to do so,
   although this is outside of the scope of this document.

   If the storage options on the peer resource are different from an
   existing PVC, the operator makes no attempt to 'correct' the
   preexisting PVC.
2. If a correctly named replica set does not exist, one will be
   launched with the PVC from above mounted as a volume. The bootstrap
   configuration will always be provided to the pod as configuration,
   as etcd will ignore bootstrap configuration if a data directory
   already exists.
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
current state of the world. In particular the etcd cluster's member
list is used to define what the expected members should be.

When reconciling the cluster resource the operator will do the
following:

1. If a headless service for the cluster does not exist, create it.
2. Attempt to dial the cluster using the headless service. If
   successfully connected update the `status` field with information
   from the cluster.
3. If we have communication with the cluster and the cluster is
   quorate, check that the number of members the cluster expects is
   the same as the cluster resource's specified `size`. If the cluster
   has too few members, add a new one using the API. If too many,
   remove exactly one using the API.
4. List the `EtcdPeer` resources that are our children. If we have
   communication with the cluster, reconcile the peer resources with
   the member list in etcd:

   * If a member exists in the etcd member list, but does not have a
     corresponding `EtcdPeer`, then create the peer resource.
   * If a `EtcdPeer` resource exists that does not have a
     corresponding member in the etcd memeber list, then remove the
     peer resource.

   If we don't have communication with the cluster (or the cluster is
   nonquorate) and we don't have enough peer resources compared with
   our desired size, then add peer resources to make up the expected
   size.
5. Update the status resource based on our communication with the
   cluster. If the cluster cannot be reached we mark ourselves as not
   ready. If the cluster is not quorate we mark as non quorate.

## Common operations

### Bootstrapping

1. A user creates a `EtcdCluster` resource with size 3.
2. Using the logic above, the cluster controller creates a headless
   service, fails to dial the cluster, and creates three peer
   resources.
3. Using the logic above, the controller for the peer resource notices
   that the created peer resources do not have PVCs or pods, and so
   creates them.
4. The pods run etcd, bootstrap to each other, and then form a
   cluster.
5. The cluster resource eventually is able to communicate with the
   cluster and comes up.

### Scale-up

1. A user edits the `EtcdCluster` resource to increase the size by
   one.
2. Using the logic above, the cluster controller notes that the cluster
   is undersized by one and adds a new member to the etcd cluster
   using the etcd API.
3. The cluster controller then notices that the cluster expects a member
   that does not have a `EtcdPeer` resource, and so adds one.
4. The peer controller creates the PVC and replica set for the new
   peer, which then bootstraps and joins the cluster.

### Scale-down

1. A user edits the `EtcdCluster` resource to decrease the size by
   one.
2. Using the logic above, the cluster controller notes that the number
   of members in the etcd cluster is too big by one and removes one.
3. The cluster controller notices that it has a `EtcdPeer` resource for a
   peer that the cluster does not know about and deletes one.
4. A deletion hook for the peer resource removes the replica set, but
   not any PVCs associated with the pod.
