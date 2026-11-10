# POSIX identity

By default, every workspace runs as the uid of its image (often `1000`). On a file system shared across workspaces (for example an NFS share mounted through a `ReadWriteMany` volume), every user then owns every file. A template can instead run its workspaces as their owner, or as their team, with real POSIX ownership.

## How it works

### Users

**Jupyter K8s** allocates one uid per user the first time a workspace of that user references a template that uses `{{ .User.Uid }}`. It records the uid in a cluster-scoped `PosixIdentity` object named `uid-<uid>`. The user's primary gid equals the uid.

### Teams

A team is a namespace. The deployer gives each namespace a gid with the `workspace.jupyter.org/posix-gid` label, and keeps it stable. **Jupyter K8s** reads the label and never writes it.

### Templates

In a template's `defaultPodSecurityContext` and `defaultContainerSecurityContext`, the identity fields (`runAsUser`, `runAsGroup`, `fsGroup`, `supplementalGroups`) accept a Go template as well as an integer. The admission webhook resolves them on every workspace create and update, and writes the integers into `spec.podSecurityContext` and `spec.containerSecurityContext`.

| Template input | Value |
|---|---|
| `{{ .User.Uid }}`, `{{ .User.Gid }}` | the workspace owner's uid, allocated on first use |
| `{{ .Namespace.Gid }}` | the `workspace.jupyter.org/posix-gid` label of the workspace's namespace |

Any field may also be a literal integer, for example the gid of a volume shared by several teams.

### Rules

- A template that uses a Go template in an identity field must also set `runAsNonRoot: true` and `supplementalGroupsPolicy: Strict` in `defaultPodSecurityContext`, so no workspace runs as root and the image's `/etc/group` cannot add groups. **Jupyter K8s** rejects the template otherwise, and holds its workspaces to both values.
- A workspace's identity fields must equal the values its template resolves, whatever `podSecurityContextOverrides` allows: users never pick a uid.
- Templates are the administrator's responsibility: **Jupyter K8s** does not check that a template using `{{ .User.Uid }}` also pins `OwnerOnly`, locks the security contexts and keeps `allowCustomInitContainers: false`.

### Id space

**Kubernetes** accepts uids and gids in `[0, 2^31)`.

| Range | Size | Owner |
|---|---|---|
| `0` to `999`, and `65534` | | reserved, always rejected |
| `1000` to `1048575` | about 1 million | deployer: team and volume gids |
| `1048576` to `1074790399` | 2^30 | **Jupyter K8s**: user uids |
| `1074790400` to `1343225855` | 2^28 | **Jupyter K8s**: reserved for group identities |
| `1343225856` to `2147483647` | about 800 million | deployer: team and volume gids |

Pick team and volume gids outside the **Jupyter K8s** ranges. **Jupyter K8s** rejects a workspace whose namespace gid is reserved, falls in a **Jupyter K8s** range, or that another namespace also carries.

## Setting it up

### Requirements

- **Kubernetes** 1.33 or later.
- Nodes whose container runtime supports `supplementalGroupsPolicy`: containerd 2.0 or later, or CRI-O 1.31 or later. Check with `kubectl get node <node> -o jsonpath='{.status.features.supplementalGroupsPolicy}'`. On other nodes, the kubelet rejects the pod with a `SupplementalGroupsPolicyNotSupported` event.
- An image that runs under an arbitrary uid.

### Private and shared templates

A private template runs its workspaces as their owner. Pin it to `OwnerOnly` and lock the security contexts, so that nobody else can connect to a workspace that acts as its owner. Keep custom init containers off too: they carry their own `securityContext`, which the template does not resolve or pin.

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: jupyterlab
spec:
  displayName: JupyterLab
  defaultImage: my-repository/my-image:my-tag
  defaultAccessType: OwnerOnly
  defaultOwnershipType: OwnerOnly
  allowCustomAccessType: false
  allowCustomOwnershipType: false
  defaultPodSecurityContext:
    runAsNonRoot: true
    supplementalGroupsPolicy: Strict
    fsGroupChangePolicy: OnRootMismatch
    runAsUser: "{{ .User.Uid }}"
    runAsGroup: "{{ .User.Gid }}"
    fsGroup: "{{ .User.Gid }}"
    supplementalGroups: ["{{ .Namespace.Gid }}", 2147000123]
  podSecurityContextOverrides: {allow: false}
  containerSecurityContextOverrides: {allow: false}
  allowCustomInitContainers: false
```

A shared template runs its workspaces as the team, so their files belong to the team rather than to whoever created the workspace:

```yaml
spec:
  displayName: JupyterLab (shared)
  defaultAccessType: Public
  allowCustomAccessType: false
  defaultPodSecurityContext:
    runAsNonRoot: true
    supplementalGroupsPolicy: Strict
    runAsUser: "{{ .Namespace.Gid }}"
    runAsGroup: "{{ .Namespace.Gid }}"
```

For a user with uid `2047113` in a namespace labelled `workspace.jupyter.org/posix-gid: "2130706500"`:

| Field | Private template | Shared template |
|---|---|---|
| `runAsUser` | `2047113` | `2130706500` |
| `runAsGroup` | `2047113` | `2130706500` |
| `fsGroup` | `2047113` | unset |
| `supplementalGroups` | `2130706500`, `2147000123` | unset |

A workspace moves between the two by changing its `templateRef`. Clear its `spec.podSecurityContext` in the same update, so the new template's values apply.

### User names in the image

A uid without a `passwd` entry breaks `whoami`, `ssh` and some libraries. The pod never belongs to group `0`, so the image cannot append to `/etc/passwd`; use [nss_wrapper](https://cwrap.org/nss_wrapper.html) instead. The access strategy passes the user's unix name:

```yaml
spec:
  deploymentModifications:
    podModifications:
      primaryContainerModifications:
        mergeEnv:
          - name: WORKSPACE_USER
            valueTemplate: "{{ .User.UnixName }}"
```

The image entrypoint then copies `/etc/passwd` and `/etc/group` to a writable directory, appends an entry named `$WORKSPACE_USER` for the current uid, and points nss_wrapper at the copies before starting the application.

`{{ .User.UnixName }}` is the name of the owner's `passwd` entry, what `whoami` prints and `ls -l` shows for their uid. It comes from the owner's `PosixIdentity` (`spec.unixName`), derived from the username at allocation (`github:JaneDoe` becomes `github-janedoe`), since a Kubernetes username cannot be a `passwd` name. Access strategy env templates and integration templates also accept `{{ .User.UsernameHash }}`, a label-safe digest of the username, for example to select a per-user `Secret`.

## Trust model

### Usernames

The Kubernetes username is the identity: a reclaimed login inherits the uid of its previous owner, as it inherits `OwnerOnly` workspaces.

### File systems

Network file systems such as NFS (with `AUTH_SYS`) trust the uid the client asserts. The barrier is admission: locked security contexts, locked volumes, no custom init containers, and no privileged containers.

### Permissions on PosixIdentity

```{warning}
Let **Jupyter K8s** manage `PosixIdentity` objects. Do not grant end users any permission on `posixidentities`: a user who can create or delete one can take another user's uid, and their files. Create one yourself only to pre-seed a uid, and delete one only to resolve a conflict (its `spec` is immutable), as described below.
```

## Administration

### Retention

**Jupyter K8s** never garbage-collects `PosixIdentity` objects: files keep their uid after the user leaves. Uninstalling the Helm release leaves the CRD in place, and each object carries a finalizer.

### Recovery

Each workspace records its owner's uid in the `workspace.jupyter.org/posix-uid` label, which its pods inherit: `kubectl get workspaces -A -l workspace.jupyter.org/posix-uid=<uid>` lists the workspaces running as a uid. If `PosixIdentity` objects are lost, **Jupyter K8s** recreates them from these labels as it reconciles the workspaces, and never gives a new user a uid a workspace still claims.

### Conflicts

**Jupyter K8s** never resolves a conflict automatically (two objects for one user, or a uid claimed by two users). It emits a `PosixIdentityConflict` event, and rejects the owner's changes to identity-templated workspaces until an administrator deletes the object in error.

### Pre-seeding a uid

To keep a uid an existing file system already uses, create the `PosixIdentity` before the user's first workspace. Its name carries the uid, so two objects can never record the same uid:

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: PosixIdentity
metadata:
  name: uid-5001
  finalizers: [workspace.jupyter.org/posix-identity-protection]
spec:
  username: "github:JaneDoe"
  usernameHash: <first 32 hex characters of the SHA-256 of the username>
  unixName: janedoe
```

```{warning}
Never delete a `PosixIdentity` without checking which files still carry its uid.
```
