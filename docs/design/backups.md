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
  type: snapshot
  destination:
    gcsBucket:
      bucketName: my-cluster-backup-bucket
      creds:
        secretRef: 
          name: my-cluster-backup-bucket-credentials
          namespace: default
```

This resource specifies that we desire a backup to be immediately taken as a [snapshot](https://github.com/etcd-io/etcd/blob/master/Documentation/op-guide/recovery.md#snapshotting-the-keyspace) from the etcd API.
This snapshot will be saved to a GCS bucket, accessed with credentials read from a given secret.
The file would be named with a timestamp when the backup was started, for uniqueness.
Backups will be taken by the controller, rather than a Job or Pod acting on behalf of the controller.

Backup types could include
  * `snapshot`: a `snapshot.db` file taken from the etcd API
  * `keys`: a zip file containing all key-values as files
  * `keys-json` a json file containing all key-values as a JSON file

Only `snapshot` will be initially implemented.

Destinations culd include
  * `socket`: data would be sent to a TCP socket
  * `gcsBucket`: pushed to a a Google Cloud Storage bucket
  * `s3Bucket`: similar to gcsBucket, but for AWS.
  * ... other hosted blob storage services

Only `gcsBucket` will be initially implemented.

A controller will be watching for these resources.
When an `EtcdBackup` resource is deployed the controller will start taking the backup in the specified way, publishing an event to notify that the backup has started. 
Once the backup has been taken, the an event will be created to notify that the backup is being uploaded, and it will be pushed to the given destination.
Once this is complete, an event will notify completion, and the resource will be removed.

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
This field allows backups to be taken on a crontab-notation schedule.

The controller will maintain a pool of go routines, one for each backup schedule resource.
These go routines will simply sleep until the crontab fires, at which point they will create an `EtcdBackup` resource from the given configuration.
The controller reconcile process will make sure that the pool of goroutines are a reflection of the desired configuration.
New routines will be created when the resource is applied, routines will be recreated when a reources is changed, and deleted when a resource is removed.


