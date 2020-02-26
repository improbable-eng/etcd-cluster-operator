# Backup & Restore

In particularly disastrous situations, we would like to be able to revert the etcd cluster to a previously known healthy state.
This document lays out the design of how we want the healthy state to be captured, securely stored & safely restored to operator-managed etcd clusters.

## Backup Resource

We will introduce a new custom resource: `EtcdBackup`.

```yaml
apiVersion: etcd.improbable.io/v1alpha1
kind: EtcdBackup
metadata:
  name: my-cluster-backup
  namespace: default
spec:
  clusterEndpoints: 
  - scheme: http
    host: my-cluster.default.svc
    port: 2379
  type: snapshot
  destination:
    gcsBucket:
      bucketName: my-cluster-backup-bucket
      creds:
        secretRef: 
          name: my-cluster-backup-bucket-credentials
          namespace: default
status:
  phase: Completed
  startTime: 2019-10-31T12:00:00Z
  completionTime: 2019-10-31T12:02:21Z
```

This resource specifies that we desire a backup to be immediately taken as a [snapshot](https://github.com/etcd-io/etcd/blob/master/Documentation/op-guide/recovery.md#snapshotting-the-keyspace) from the etcd API of the given cluster.
This snapshot will be saved to a GCS bucket, accessed with credentials read from a given secret.
The file would be named with a timestamp when the backup was started, for uniqueness.
For the initial implementation, backups will be taken by `etcd-cluster-operator`, rather than a `Job` or `Pod` acting on behalf of the controller.
This is simply because we do not feel a need to separate this to an external process for the time being.

Backup types could include
  * `snapshot`: a `snapshot.db` file taken from the etcd API
  * `keys`: a zip file containing all key-values as files
  * `keys-json` a json file containing all key-values as a JSON file

Only `snapshot` will be initially implemented.

Destinations could include
  * `socket`: data would be sent to a TCP socket
  * `gcsBucket`: pushed to a a Google Cloud Storage bucket
  * `s3Bucket`: similar to gcsBucket, but for AWS.
  * ... other hosted blob storage services

Only `gcsBucket` will be initially implemented.

A controller will be watching for these resources.
When an `EtcdBackup` resource is deployed the controller will start taking the backup in the specified way, publishing an event to notify that the backup has started. 
Once the backup has been taken, the an event will be created to notify that the backup is being uploaded, and it will be pushed to the given destination.
Once this is complete, an event will notify completion, we'll also update the resource status to `Complete`.
If an error occured during the backup process, this will also be shown by setting the resource status to `Failed`.

## Backup Schedule Resource

We would like to be able to take automated backups on a given schedule. 
A second custom resource will be added to support this: 

```yaml
apiVersion: etcd.improbable.io/v1alpha1
kind: EtcdBackupSchedule
metadata:
  name: my-backup-schedule
  namespace: default
spec:
  schedule: "*/1 * * * *"
  successfulBackupsHistoryLimit: 30
  failedBackupsHistoryLimit: 5
  backupTemplate:
    type: snapshot
    destination:
      gcsBucket:
        bucketName: my-cluster-backup-bucket
        creds:
          secretRef: 
            name: my-cluster-backup-bucket-credentials
            namespace: default
```

This resource appears very similar to the `EtcdBackup` resource, with the addition of a `schedule` field:
This field allows backups to be taken on a crontab-notation schedule, behaving similarly to the Kubernetes [CronJob](https://kubernetes.io/docs/tasks/job/automated-tasks-with-cron-jobs/#schedule) resource.

The controller will maintain a pool of go routines, one for each backup schedule resource.
These go routines will simply sleep until the crontab fires, at which point they will create a labelled `EtcdBackup` resource from the given configuration.
The controller reconcile process will make sure that the pool of goroutines are a reflection of the desired configuration.
New routines will be created when a resource is applied, routines will be recreated when a resource is changed, and deleted when a resource is removed.

Once a backup has finished, the controller will ensure that we are only keeping the specified number of `EtcdBackup` resources around, and will clean up any old resources respecting the `successfulBackupsHistoryLimit` and `failedBackupsHistoryLimit` fields.

## Backup Restoration

Restores are done via an `EtcdRestore` resource. This will create a new cluster from the restored data. Based on the
etcd documentation it is not possible to do a restore in-place. At least, not without stopping the `etcd` process and
replacing the data directory.

Traditionally a restore is done 'on the node' using `etcdctl`, before starting the `etcd` process. In Kubernetes we
instead pre-create the PVC for the peers ahead of time, then use a Pod to execute the restore (using the "restore
agent"). The cluster is then created to use those PVCs. Because it's a property of etcd to ignore bootstrap instructions
if the data directory already exists, the cluster will ignore bootstrap and simply start.

Because of this requirement to restore into the data directory, we can't do the restore from the operator pod like we
can with the backup. (The PVC location may be in a different zone to the operator pod.) So a Pod must be used.

## Alternatives

This behaviour could conceivably live inside of a Kubernetes `Job` / `CronJob` running some custom container to make the backup & push it to a remote.
We chose to avoid this approach as it limits the configurability of the backups. 
It would quickly become complex to support different backup strategies and storage destinations, and we'd likely end up implementing some configuration layer infront of the job.

The [Velero](https://github.com/vmware-tanzu/velero) project aims to provide tools to back up all important data in a Kubernetes cluster. 
We considered building an integration in to this project to do the ETCD backups for us - and there was no technical reason we found why this wouldn't be possible. 
However we would like to offer something out of the box for users who do not want to run Velero, so chose to build this in to the operator as a start.

