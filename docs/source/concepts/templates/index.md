# Templates

A **WorkspaceTemplate** provides an easy way to configure a workspace and to enforce constraints on its attributes.

Workspace templates have a 1:many relationship with workspaces; multiple workspaces may reference the same template, but a workspace can reference at most one template.

## Usage
A workspace user can create a workspace referencing a template as follow:
```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: alice-workspace
  namespace: alice-team
spec:
  displayName: Alice's Workspace
  templateRef:
    name: template-for-alice-team
```

In this case, the template configures default settings for the workspace, for example its `spec.image` or `spec.accessType`.
The workspace user does not need to worry about all these details.

Templates are namespaced resources, and a `workspace.spec.templateRef` may only reference a template in its own namespace.

There is an exception to this rule: **Jupyter K8s** allows workspaces of _any_ namespace to reference templates in the [shared namespace](shared-namespace) - a special namespace identified at the **Jupyter K8s** operator level.

## Setup

There are two primary use cases for templates:
1. cluster or team administrators to provide trusted configurations to workspace users
2. advanced workspace users to save and reuse their own workspace configurations

A user with the right RBAC permission can create a template as follows:
```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: template-for-team-alice
  namespace: team-alice
spec:
  displayName: Team Alice Template
  defaultImage: my-repository/my-image:my-tag
```

A template may reference an access strategy as well:
```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: template-for-team-alice
  namespace: team-alice
spec:
  displayName: Team Alice Template
  defaultImage: my-repository/my-image:my-tag
  defaultAccessStrategy:
    name: web-access
```

In this case, any workspace that references this template will inherit the access strategy.

```{toctree}
:hidden:

defaults
bounds
shared-namespace
```
