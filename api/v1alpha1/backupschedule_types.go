package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupScheduleSpec defines the desired state of BackupSchedule.
type BackupScheduleSpec struct {
	// Schedule in cron format (e.g., "0 2 * * *" for daily at 2am).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(\S+\s){4}\S+$`
	Schedule string `json:"schedule"`

	// PVCSelector selects PersistentVolumeClaims to backup.
	// +kubebuilder:validation:Required
	PVCSelector metav1.LabelSelector `json:"pvcSelector"`

	// Retention defines how many backups to keep.
	// +optional
	Retention RetentionPolicy `json:"retention,omitempty"`

	// StorageClass for the VolumeSnapshot (optional, uses default if empty).
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// Suspend prevents new backups from being scheduled.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// RetentionPolicy defines backup retention rules.
type RetentionPolicy struct {
	// KeepLast is the number of most recent backups to retain.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=7
	KeepLast int32 `json:"keepLast,omitempty"`
}

// BackupScheduleStatus defines the observed state of BackupSchedule.
type BackupScheduleStatus struct {
	// LastBackupTime is the timestamp of the last successful backup.
	// +optional
	LastBackupTime *metav1.Time `json:"lastBackup,omitempty"`

	// NextBackupTime is the scheduled time for the next backup.
	// +optional
	NextBackupTime *metav1.Time `json:"nextBackup,omitempty"`

	// BackupCount is the current number of stored backups.
	// +optional
	BackupCount int32 `json:"backupCount,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=bs
// +kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".spec.schedule"
// +kubebuilder:printcolumn:name="Last Backup",type="date",JSONPath=".status.lastBackup"
// +kubebuilder:printcolumn:name="Next Backup",type="date",JSONPath=".status.nextBackup"
// +kubebuilder:printcolumn:name="Backups",type="integer",JSONPath=".status.backupCount"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BackupSchedule is the Schema for the backupschedules API.
type BackupSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupScheduleSpec   `json:"spec,omitempty"`
	Status BackupScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BackupScheduleList contains a list of BackupSchedule.
type BackupScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupSchedule{}, &BackupScheduleList{})
}
