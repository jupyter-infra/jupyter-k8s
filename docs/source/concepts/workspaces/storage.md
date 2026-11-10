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

The volume name `workspace-storage` is reserved for the primary volume. No two volumes may share a mount path: the admission webhook rejects a volume at the path of the primary storage or of another volume, and names both.

### Subdirectories and read-only mounts

A volume can mount a subdirectory of its claim with `subPath`, and mount the claim read-only with `readOnly`:

```yaml
spec:
  volumes:
    - name: scratch
      persistentVolumeClaimName: scratch
      mountPath: /home/jovyan/scratch
      subPath: namespaces/ml
    - name: datasets
      persistentVolumeClaimName: datasets
      mountPath: /home/jovyan/datasets
      readOnly: true
```

| Field | Default | Description |
|-------|---------|-------------|
| `subPath` | Claim root | Directory inside the claim to mount; only that directory is visible in the workspace |
| `readOnly` | `false` | Mounts the volume read-only; writes fail with `EROFS` |

`subPath` must be relative, with no empty or `..` segments, and applies to claims only, not to `emptyDir` volumes. The directory must already exist on the volume, owned by the user the workspace runs as. If it is missing, the kubelet creates it as root, which fails or yields a directory the workspace cannot write to on file systems that squash root, such as NFS or Lustre. Create the directories beforehand, for example with a Job that mounts the claim's root when you onboard a team or namespace. `readOnly` also sets the pod volume's `persistentVolumeClaim.readOnly`, so no other container of the pod can mount the claim read-write.

### Template default volumes

Templates can disallow secondary volumes with `allowSecondaryStorages: false`, or provide default volumes via `defaultVolumes`. The admission webhook copies `defaultVolumes` onto a workspace that declares no volumes of its own, `subPath` and `readOnly` included; `allowSecondaryStorages: false` only counts the volumes a workspace adds beyond them.

A template locks its default volumes with `volumeOverrides`, which deployers use to offer shared storage that users cannot remove, alter, or bypass:

```yaml
spec:
  defaultVolumes:
    - name: team
      persistentVolumeClaimName: team
      mountPath: /home/jovyan/shared
      subPath: namespaces/ml
  volumeOverrides:
    allow: false
```

With `allow: false`:

- every workspace of the template must keep each default volume unchanged (name, mount path, claim, `subPath`, `readOnly`). A workspace that declares no volumes gets them from the webhook; one that declares its own volumes must list the default volumes alongside them, and the webhook rejects it otherwise, naming the missing or changed volume.
- `allowSecondaryStorages` still governs the volumes a workspace adds. With the lock alone, a workspace can add its own volume on a default volume's claim, for example read-write at another path without the `subPath`. Set `allowSecondaryStorages: false` as well to restrict workspaces to the default volumes exactly as defined.
- init containers mount the pod's volumes with their own `volumeMounts`, `subPath` included, and their own `securityContext`. Keep `allowCustomInitContainers: false` as well, so that only the template's `defaultInitContainers` touch the volumes.

The webhook enforces these rules lazily, on workspace create and update. A template edit to its default volumes does not change existing workspaces: their next update other than a stop, starting the workspace included, is rejected until it lists the template's current volumes. Changing `templateRef` evaluates these rules against the new template; the workspace keeps the old template's volumes unless the update removes them.

## Shared memory

`/dev/shm` is shared memory, the slice of RAM that processes on one machine use to hand large data to each other without copying it. PyTorch's DataLoader workers and NCCL depend on it, and a container gets only 64MiB unless something mounts a larger one.

**Jupyter K8s** mounts a memory-backed `emptyDir` volume named `workspace-shm` at `/dev/shm` in the primary container of every workspace that asks for it. Its `sizeLimit` is the container's memory limit, or the memory request when the container sets no limit; a container that declares neither gets no volume and keeps the 64MiB default, and the admission webhook warns about it when the workspace is created or updated. The volume adds no memory to the workspace: whatever a process writes into it counts against the container's memory. A workspace that declares its own volume at `/dev/shm` keeps it, and the volume name `workspace-shm` is reserved, on workspaces and on template default volumes alike.

A workspace asks for the volume by setting `sharedMemory`; an empty value is enough, and `enabled: false` turns off a template's default:

```yaml
spec:
  sharedMemory: {}
```

A template gives the volume to all its workspaces with `defaultSharedMemory`, which the admission webhook copies onto a workspace that sets no `sharedMemory`, and locks it with `sharedMemoryOverrides`:

```yaml
spec:
  defaultSharedMemory: {}
  sharedMemoryOverrides:
    allow: false
```

With `allow: false` a workspace must keep the template's setting: the webhook rejects a `sharedMemory` that differs from the template default and any volume the workspace mounts at `/dev/shm` itself, and a template that sets `allow: false` must also set a `defaultSharedMemory`. Volumes that an access strategy or integration template mounts at `/dev/shm` are not subject to this rule and take precedence over the operator's volume. A deployment that wants every workspace covered sets the default on the templates it deploys, or sets `sharedMemory` on every workspace with a mutating admission policy or webhook. A workspace that needs a `/dev/shm` of another size declares its own volume there, which replaces the operator's.

Files in `/dev/shm` outlive the process that created them. A worker that crashes can leave files behind that a kernel restart does not clear, and they keep counting against the container's memory. If a workspace runs out of memory after a failed run, check `df -h /dev/shm` and delete stale files with `rm /dev/shm/<file>`, or stop and start the workspace, which recreates the pod and with it the volume.
