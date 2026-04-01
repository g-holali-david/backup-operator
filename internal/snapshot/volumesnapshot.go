// Package snapshot provides helpers for VolumeSnapshot operations.
// This is a wrapper since the actual VolumeSnapshot CRD types come from
// the external-snapshotter project.
package snapshot

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var volumeSnapshotGVR = schema.GroupVersionResource{
	Group:    "snapshot.storage.k8s.io",
	Version:  "v1",
	Resource: "volumesnapshots",
}

// VolumeSnapshotWrapper wraps an unstructured VolumeSnapshot.
type VolumeSnapshotWrapper struct {
	unstructured.Unstructured
}

// NewVolumeSnapshot creates an unstructured VolumeSnapshot object.
func NewVolumeSnapshot(name, namespace, pvcName, storageClass, ownerSchedule string) *unstructured.Unstructured {
	snapshot := &unstructured.Unstructured{}
	snapshot.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "snapshot.storage.k8s.io",
		Version: "v1",
		Kind:    "VolumeSnapshot",
	})
	snapshot.SetName(name)
	snapshot.SetNamespace(namespace)
	snapshot.SetLabels(map[string]string{
		"app.kubernetes.io/managed-by": "backup-operator",
		"backup.hdgavi.dev/schedule":   ownerSchedule,
	})

	spec := map[string]interface{}{
		"source": map[string]interface{}{
			"persistentVolumeClaimName": pvcName,
		},
	}

	if storageClass != "" {
		spec["volumeSnapshotClassName"] = storageClass
	}

	snapshot.Object["spec"] = spec

	return snapshot
}

// ListByOwner returns all VolumeSnapshots owned by a specific BackupSchedule.
func ListByOwner(ctx context.Context, c client.Client, namespace, scheduleName string) ([]VolumeSnapshotWrapper, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "snapshot.storage.k8s.io",
		Version: "v1",
		Kind:    "VolumeSnapshotList",
	})

	labelSelector, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			"backup.hdgavi.dev/schedule": scheduleName,
		},
	})

	if err := c.List(ctx, list, &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return nil, err
	}

	var wrappers []VolumeSnapshotWrapper
	for _, item := range list.Items {
		wrappers = append(wrappers, VolumeSnapshotWrapper{item})
	}

	return wrappers, nil
}
