package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EtcdBackupScheduleSpec defines the desired state of EtcdBackupSchedule
type EtcdBackupScheduleSpec struct {
	// Schedule holds a crontab-like scheule holding defining the schedule in which backups will be started.
	Schedule string `json:"schedule"`
	// BackupTemplate describes the template used to create backup resources. Every time the schedule fires
	// an `EtcdBackup' will be created with this template.
	BackupTemplate EtcdBackupSpec `json:"backupTemplate"`
}

// EtcdBackupScheduleStatus defines the observed state of EtcdBackupSchedule
type EtcdBackupScheduleStatus struct {
}

// +kubebuilder:object:root=true

// EtcdBackupSchedule is the Schema for the etcdbackupschedules API
type EtcdBackupSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdBackupScheduleSpec   `json:"spec,omitempty"`
	Status EtcdBackupScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EtcdBackupScheduleList contains a list of EtcdBackupSchedule
type EtcdBackupScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdBackupSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdBackupSchedule{}, &EtcdBackupScheduleList{})
}
