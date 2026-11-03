# Private and shared templates

A workspace's `accessType` controls who can connect to it, and its `ownershipType` who can modify it (see [Access types](../workspaces/access-types.md)). A template sets their defaults with `defaultAccessType` and `defaultOwnershipType`, but a default does not stop a workspace user from turning a private workspace `Public`.

This matters when the workspace runs with its owner's identity, for example their file system user or their cloud credentials. In a `Public` workspace, anyone who can connect then acts as the owner: their writes are attributed to the owner, and they can read every file the owner can read on shared volumes.

## Pinning the access and ownership types

To keep private workspaces private, pin both types in the template with `allowCustomAccessType: false` and `allowCustomOwnershipType: false`. A deployer who offers both private and shared workspaces defines two templates:

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: jupyterlab
  namespace: team-alice
  labels:
    workspace.jupyter.org/default-template: "true"
spec:
  displayName: JupyterLab (private)
  defaultImage: my-repository/jupyterlab:latest
  defaultAccessType: OwnerOnly
  defaultOwnershipType: OwnerOnly
  allowCustomAccessType: false
  allowCustomOwnershipType: false
---
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: jupyterlab-shared
  namespace: team-alice
spec:
  displayName: JupyterLab (shared)
  defaultImage: my-repository/jupyterlab:latest
  defaultAccessType: Public
  defaultOwnershipType: Public
  allowCustomAccessType: false
  allowCustomOwnershipType: false
```

A workspace user moves a workspace between the two by changing its `spec.templateRef`, and setting `spec.accessType` and `spec.ownershipType` to the new template's pinned values in the same update (see [Security restrictions](bounds.md#security-restrictions)). Since the private template pins `ownershipType: OwnerOnly`, only the workspace's creator can make that change.

## Locking the security contexts

A template that pins the access type should usually also lock the security contexts. Otherwise a workspace user could request a privileged container or extra capabilities, which give them root on the node, and with it access to other pods' volumes and the node's credentials. See [Security restrictions](bounds.md#security-restrictions).

## Requiring a template

These bounds only apply to workspaces that reference the template. A workspace without a `templateRef` is unconstrained. To make sure every workspace has one, label a template as the namespace's {ref}`default template <default-template-resolution>`: the webhook assigns it to any workspace created without a `templateRef`, and to any workspace whose update removes it.

## Template consistency

Templates are the administrator's responsibility. **Jupyter K8s** does not cross-check what a template combines: for example, it admits a template that gives workspaces their owner's identity without pinning their access type. Review the combination of identity, access type and security contexts in each template you publish.
