# Backup Operator

[![CI](https://github.com/g-holali-david/backup-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/g-holali-david/backup-operator/actions/workflows/ci.yml)

A Kubernetes Operator that automates PersistentVolumeClaim backups using VolumeSnapshots, with cron scheduling and retention policies.

## Custom Resource Definition

```yaml
apiVersion: backup.hdgavi.dev/v1alpha1
kind: BackupSchedule
metadata:
  name: postgres-daily
spec:
  schedule: "0 2 * * *"          # Every day at 2am
  pvcSelector:
    matchLabels:
      app: postgres
  retention:
    keepLast: 7                   # Keep last 7 snapshots
  storageClass: gp3               # VolumeSnapshot class
```

## How It Works

```
┌──────────────────────────┐
│   BackupSchedule CR      │
│   (user creates)         │
└───────────┬──────────────┘
            │ watch
┌───────────▼──────────────┐
│   Reconciliation Loop    │
│                          │
│   1. Parse cron schedule │
│   2. Find matching PVCs  │
│   3. Create snapshots    │
│   4. Apply retention     │
│   5. Update status       │
└───────────┬──────────────┘
            │ creates
┌───────────▼──────────────┐
│   VolumeSnapshots        │
│   (managed by operator)  │
└──────────────────────────┘
```

## Features

- **Cron scheduling** — standard cron expressions for backup timing
- **Label-based PVC selection** — backup multiple PVCs with one CR
- **Retention policies** — automatically delete old snapshots
- **Suspend/resume** — pause backups without deleting the schedule
- **Status tracking** — last backup, next backup, backup count, conditions
- **Leader election** — safe to run multiple replicas
- **Minimal RBAC** — only permissions needed for snapshots and PVCs

## Installation

### Helm

```bash
helm install backup-operator ./deploy/helm \
  --namespace backup-operator-system \
  --create-namespace
```

### Manual

```bash
# Install CRD
kubectl apply -f config/crd/bases/

# Install RBAC
kubectl apply -f config/rbac/

# Install operator
kubectl apply -f config/manager/
```

## Usage

```bash
# Create a backup schedule
kubectl apply -f config/samples/backup_v1alpha1_backupschedule.yaml

# Check status
kubectl get backupschedules
NAME             SCHEDULE      LAST BACKUP              NEXT BACKUP              BACKUPS   AGE
postgres-daily   0 2 * * *     2026-03-31T02:00:00Z     2026-04-01T02:00:00Z     7         7d

# View details
kubectl describe backupschedule postgres-daily

# List created snapshots
kubectl get volumesnapshots -l backup.hdgavi.dev/schedule=postgres-daily
```

## Project Structure

```
.
├── api/v1alpha1/              # CRD type definitions
│   ├── backupschedule_types.go
│   └── groupversion_info.go
├── internal/
│   ├── controller/            # Reconciliation loop
│   └── snapshot/              # VolumeSnapshot helpers
├── config/
│   ├── crd/bases/             # Generated CRD manifests
│   ├── rbac/                  # RBAC roles
│   ├── manager/               # Operator deployment
│   └── samples/               # Example CRs
├── deploy/helm/               # Helm chart
├── main.go                    # Entry point
└── Dockerfile                 # Multi-stage distroless
```

## Prerequisites

- Kubernetes 1.27+
- CSI driver with snapshot support (e.g., EBS CSI, GCE PD CSI)
- VolumeSnapshot CRDs installed (`snapshot.storage.k8s.io/v1`)

## Development

```bash
# Build
go build -o manager .

# Run locally (connected to a cluster)
./manager

# Run tests
go test -v ./...

# Build Docker image
docker build -t backup-operator:dev .
```

## Tech Stack

- **Language**: Go 1.22
- **Framework**: Kubebuilder v4 / controller-runtime
- **CRD**: `BackupSchedule` (backup.hdgavi.dev/v1alpha1)
- **Distribution**: Helm Chart + raw manifests
- **CI**: GitHub Actions (test → lint → build → push GHCR)

## License

MIT
