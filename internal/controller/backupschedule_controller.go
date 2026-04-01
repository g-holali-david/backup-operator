package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	backupv1alpha1 "github.com/g-holali-david/backup-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	snapshotv1 "github.com/g-holali-david/backup-operator/internal/snapshot"
)

// BackupScheduleReconciler reconciles a BackupSchedule object.
type BackupScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=backup.hdgavi.dev,resources=backupschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=backup.hdgavi.dev,resources=backupschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=backup.hdgavi.dev,resources=backupschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;watch;create;delete

func (r *BackupScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the BackupSchedule
	var schedule backupv1alpha1.BackupSchedule
	if err := r.Get(ctx, req.NamespacedName, &schedule); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling BackupSchedule", "name", schedule.Name)

	// 2. Check if suspended
	if schedule.Spec.Suspend {
		logger.Info("BackupSchedule is suspended, skipping")
		return ctrl.Result{}, nil
	}

	// 3. Parse cron schedule and calculate next backup time
	nextBackup, err := getNextSchedule(schedule.Spec.Schedule, schedule.Status.LastBackupTime)
	if err != nil {
		logger.Error(err, "Failed to parse cron schedule")
		return ctrl.Result{}, err
	}

	now := time.Now()
	schedule.Status.NextBackupTime = &metav1.Time{Time: nextBackup}

	// 4. Check if it's time to backup
	if now.Before(nextBackup) {
		requeueAfter := nextBackup.Sub(now)
		logger.Info("Not yet time for backup", "nextBackup", nextBackup, "requeueAfter", requeueAfter)
		if err := r.Status().Update(ctx, &schedule); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// 5. Find matching PVCs
	pvcs, err := r.findMatchingPVCs(ctx, schedule.Namespace, schedule.Spec.PVCSelector)
	if err != nil {
		logger.Error(err, "Failed to find matching PVCs")
		return ctrl.Result{}, err
	}

	if len(pvcs) == 0 {
		logger.Info("No PVCs matching selector")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// 6. Create VolumeSnapshot for each PVC
	for _, pvc := range pvcs {
		snapshotName := fmt.Sprintf("%s-%s-%s",
			schedule.Name,
			pvc.Name,
			now.Format("20060102-150405"),
		)

		logger.Info("Creating VolumeSnapshot", "snapshot", snapshotName, "pvc", pvc.Name)

		snapshot := snapshotv1.NewVolumeSnapshot(
			snapshotName,
			schedule.Namespace,
			pvc.Name,
			schedule.Spec.StorageClass,
			schedule.Name,
		)

		if err := r.Create(ctx, snapshot); err != nil {
			if !errors.IsAlreadyExists(err) {
				logger.Error(err, "Failed to create VolumeSnapshot", "snapshot", snapshotName)
				return ctrl.Result{}, err
			}
		}
	}

	// 7. Update status
	schedule.Status.LastBackupTime = &metav1.Time{Time: now}
	schedule.Status.BackupCount++

	// 8. Apply retention policy
	if err := r.applyRetention(ctx, &schedule); err != nil {
		logger.Error(err, "Failed to apply retention policy")
	}

	// 9. Update condition
	setCondition(&schedule, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "BackupCompleted",
		Message:            fmt.Sprintf("Backup completed at %s", now.Format(time.RFC3339)),
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, &schedule); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue for next backup
	nextBackup, _ = getNextSchedule(schedule.Spec.Schedule, schedule.Status.LastBackupTime)
	return ctrl.Result{RequeueAfter: nextBackup.Sub(time.Now())}, nil
}

func (r *BackupScheduleReconciler) findMatchingPVCs(
	ctx context.Context,
	namespace string,
	selector metav1.LabelSelector,
) ([]corev1.PersistentVolumeClaim, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %w", err)
	}

	var pvcList corev1.PersistentVolumeClaimList
	if err := r.List(ctx, &pvcList, &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return nil, err
	}

	// Only return bound PVCs
	var bound []corev1.PersistentVolumeClaim
	for _, pvc := range pvcList.Items {
		if pvc.Status.Phase == corev1.ClaimBound {
			bound = append(bound, pvc)
		}
	}

	return bound, nil
}

func (r *BackupScheduleReconciler) applyRetention(
	ctx context.Context,
	schedule *backupv1alpha1.BackupSchedule,
) error {
	logger := log.FromContext(ctx)

	keepLast := schedule.Spec.Retention.KeepLast
	if keepLast <= 0 {
		keepLast = 7
	}

	// List all snapshots owned by this schedule
	snapshots, err := r.listOwnedSnapshots(ctx, schedule)
	if err != nil {
		return err
	}

	if int32(len(snapshots)) <= keepLast {
		schedule.Status.BackupCount = int32(len(snapshots))
		return nil
	}

	// Sort by creation time (oldest first)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].GetCreationTimestamp().Before(&snapshots[j].GetCreationTimestamp())
	})

	// Delete oldest snapshots beyond retention
	toDelete := snapshots[:len(snapshots)-int(keepLast)]
	for _, snap := range toDelete {
		logger.Info("Deleting old snapshot (retention policy)", "snapshot", snap.GetName())
		if err := r.Delete(ctx, &snap); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	schedule.Status.BackupCount = keepLast
	return nil
}

func (r *BackupScheduleReconciler) listOwnedSnapshots(
	ctx context.Context,
	schedule *backupv1alpha1.BackupSchedule,
) ([]snapshotv1.VolumeSnapshotWrapper, error) {
	return snapshotv1.ListByOwner(ctx, r.Client, schedule.Namespace, schedule.Name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1alpha1.BackupSchedule{}).
		Complete(r)
}

// --- Helpers ---

func getNextSchedule(cronExpr string, lastBackup *metav1.Time) (time.Time, error) {
	// Simplified cron parsing for common patterns
	// In production, use github.com/robfig/cron/v3
	now := time.Now()

	if lastBackup == nil {
		return now, nil // Run immediately on first reconcile
	}

	// Default: requeue 24h after last backup
	// Full cron parsing would be added with robfig/cron dependency
	return lastBackup.Time.Add(24 * time.Hour), nil
}

func setCondition(schedule *backupv1alpha1.BackupSchedule, condition metav1.Condition) {
	for i, c := range schedule.Status.Conditions {
		if c.Type == condition.Type {
			schedule.Status.Conditions[i] = condition
			return
		}
	}
	schedule.Status.Conditions = append(schedule.Status.Conditions, condition)
}

// Ensure labels is used (compilation check)
var _ = labels.Everything
var _ = types.NamespacedName{}
