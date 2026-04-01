# Architecture — Backup Operator

## Vue d'ensemble

Un Kubernetes Operator qui gère un CRD `BackupSchedule` pour automatiser les snapshots de PersistentVolumeClaims selon un planning cron, avec gestion de rétention.

## Reconciliation Loop

```
                    ┌───────────────────┐
                    │ BackupSchedule CR │
                    │  (spec + status)  │
                    └────────┬──────────┘
                             │
                    ┌────────▼──────────┐
                    │   Reconcile()     │
                    └────────┬──────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
      ┌───────▼──────┐  ┌───▼──────┐  ┌───▼──────────┐
      │ Parse cron   │  │ Find PVCs│  │ Check time   │
      │ schedule     │  │ by label │  │ (next backup)│
      └───────┬──────┘  └───┬──────┘  └───┬──────────┘
              │              │              │
              └──────────────┼──────────────┘
                             │
                    ┌────────▼──────────┐
                    │  Time to backup?  │
                    └────────┬──────────┘
                             │
                   No ───────┼──────── Yes
                   │                    │
           ┌───────▼──────┐    ┌───────▼──────────┐
           │ Requeue with │    │ For each PVC:    │
           │ delay until  │    │ Create           │
           │ next backup  │    │ VolumeSnapshot   │
           └──────────────┘    └───────┬──────────┘
                                       │
                              ┌────────▼──────────┐
                              │ Apply retention   │
                              │ (delete old snaps)│
                              └────────┬──────────┘
                                       │
                              ┌────────▼──────────┐
                              │ Update status:    │
                              │ - lastBackup      │
                              │ - nextBackup      │
                              │ - backupCount     │
                              │ - conditions      │
                              └────────┬──────────┘
                                       │
                              ┌────────▼──────────┐
                              │ Requeue for next  │
                              │ scheduled backup  │
                              └───────────────────┘
```

## CRD: BackupSchedule

### Spec

| Champ | Type | Description |
|-------|------|-------------|
| `schedule` | string (cron) | Planning cron (ex: `0 2 * * *`) |
| `pvcSelector` | LabelSelector | Sélecteur de labels pour les PVCs |
| `retention.keepLast` | int32 | Nombre de snapshots à conserver (défaut: 7) |
| `storageClass` | string | VolumeSnapshotClass (optionnel) |
| `suspend` | bool | Suspendre les backups |

### Status

| Champ | Type | Description |
|-------|------|-------------|
| `lastBackup` | Time | Timestamp du dernier backup |
| `nextBackup` | Time | Prochain backup planifié |
| `backupCount` | int32 | Nombre de snapshots actuels |
| `conditions` | []Condition | État du CR (Ready, etc.) |

### Printer Columns (`kubectl get bs`)

```
NAME             SCHEDULE      LAST BACKUP              NEXT BACKUP              BACKUPS   AGE
postgres-daily   0 2 * * *     2026-03-31T02:00:00Z     2026-04-01T02:00:00Z     7         7d
redis-hourly     0 * * * *     2026-04-01T10:00:00Z     2026-04-01T11:00:00Z     24        3d
```

## RBAC

Le controller utilise le **principe du moindre privilège** :

| Ressource | Verbes | Justification |
|-----------|--------|---------------|
| `backupschedules` | all | Gérer les CR |
| `backupschedules/status` | get, update, patch | Mettre à jour le status |
| `persistentvolumeclaims` | get, list, watch | Trouver les PVCs ciblés |
| `volumesnapshots` | get, list, watch, create, delete | Créer et supprimer les snapshots |
| `events` | create, patch | Émettre des événements K8s |
| `leases` | all | Leader election |

## Labels sur les VolumeSnapshots

Chaque snapshot créé est étiqueté :

```yaml
labels:
  app.kubernetes.io/managed-by: backup-operator
  backup.hdgavi.dev/schedule: postgres-daily
```

Permet de :
- Lister les snapshots d'un schedule spécifique
- Appliquer la politique de rétention
- Identifier les snapshots gérés par l'operator

## Sécurité

- **Distroless image** : pas de shell, surface minimale
- **Non-root** : runAsNonRoot + capabilities drop ALL
- **Leader election** : safe en multi-replica
- **RBAC minimal** : uniquement les permissions nécessaires
- **No privilege escalation** : securityContext strict

## Prérequis cluster

1. **CSI driver** avec support VolumeSnapshot (EBS CSI, GCE PD CSI, etc.)
2. **VolumeSnapshot CRDs** installés :
   ```bash
   kubectl get crd volumesnapshots.snapshot.storage.k8s.io
   ```
3. **VolumeSnapshotClass** configurée :
   ```bash
   kubectl get volumesnapshotclass
   ```
