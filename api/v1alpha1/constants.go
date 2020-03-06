package v1alpha1

const (
	etcdSupportedVersionMajor = 3
	// EtcdBackupReasonSucceeded is the event generated after a successful backup.
	EtcdBackupReasonSucceeded = "BackupSucceeded"
	// EtcdBackupReasonFailed is the event generated after a failed backup
	EtcdBackupReasonFailed = "BackupFailed"
)
