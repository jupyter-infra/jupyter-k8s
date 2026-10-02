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

## Shared memory

`/dev/shm` is shared memory, the slice of RAM that processes on one machine use to hand large data to each other without copying it. PyTorch's DataLoader workers and NCCL depend on it, and a container gets only 64MiB unless something mounts a larger one.

**Jupyter K8s** mounts a memory-backed `emptyDir` volume named `workspace-shm` at `/dev/shm` in the primary container of every workspace. Its `sizeLimit` is the container's memory limit, or the memory request when the container sets no limit; a container that declares neither gets no volume and keeps the 64MiB default. The volume adds no memory to the workspace: whatever a process writes into it counts against the container's memory. A workspace that declares its own volume at `/dev/shm` keeps it, and the volume name `workspace-shm` is reserved.

A workspace turns the volume off with `sharedMemory`:

```yaml
spec:
  sharedMemory:
    enabled: false
```

A template sets the default with `defaultSharedMemory`, which the admission webhook copies onto a workspace that sets no `sharedMemory`, and locks it with `sharedMemoryOverrides`:

```yaml
spec:
  defaultSharedMemory:
    enabled: false
  sharedMemoryOverrides:
    allow: false
```

With `allow: false` the webhook rejects a workspace `sharedMemory` that differs from the template default and any volume the workspace mounts at `/dev/shm` itself; a template that locks overrides must set a `defaultSharedMemory`. A workspace that needs a `/dev/shm` of another size declares its own volume there, which replaces the operator's.
