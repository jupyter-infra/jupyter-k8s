# Storage

Each workspace gets a dedicated PersistentVolumeClaim (PVC) for durable storage that survives pod restarts and re-scheduling.

The workspace storage persists when a workspace is stopped.

Workspaces can also mount shared volumes — pre-existing PVCs that multiple workspaces reference. This enables collaboration patterns like shared datasets, team code repositories, or common model artifacts across workspaces.

## Primary storage

Configured via `spec.storage`:

```yaml
spec:
  storage:
    size: 20Gi
    mountPath: /home/jovyan
    storageClassName: gp3
```

| Field | Default | Description |
|-------|---------|-------------|
| `size` | Template default (typically `10Gi`) | Volume capacity |
| `mountPath` | `/home/jovyan` | Where the volume is mounted in the container |
| `storageClassName` | Cluster default | Kubernetes StorageClass to provision the volume |

The storage class name is **immutable** after creation — changing it requires recreating the workspace.

## Template bounds

A template can constrain storage size:

```yaml
spec:
  primaryStorage:
    defaultSize: 10Gi
    minSize: 5Gi
    maxSize: 100Gi
    defaultStorageClassName: gp3
    defaultMountPath: /home/jovyan
```

The admission webhook rejects workspaces whose storage size falls outside the bounds defined by the template.

## Secondary volumes

Workspaces can mount additional pre-existing PVCs:

```yaml
spec:
  volumes:
    - name: shared-data
      persistentVolumeClaimName: team-shared-pvc
      mountPath: /data
```

The volume name `workspace-storage` is reserved for the primary volume.

Templates can disallow secondary volumes with `allowSecondaryStorages: false`, or provide default volumes via `defaultVolumes`.
