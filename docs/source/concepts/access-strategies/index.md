# Access Strategies

A **WorkspaceAccessStrategy** configures a workspace so that the routing layers - router, authentication, authorization - can connect to it. It determines the Kubernetes resources to create or update, how to construct connection URLs, and how to modify the workspace deployment.

Access strategies are resources deeply tied to the specific routing choices of the cluster. Administrators should setup a handful of access strategies in their cluster, and update them infrequently.

In an enterprise cluster, workspace users should not have permission to create, update or even describe access strategies directly.

## Usage

Workspace users can reference an access strategy when creating or updating a workspace, for example:
```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: alice-workspace
  namespace: alice-team
spec:
  displayName: Alice's Workspace
  accessStrategy:
    name: web-access
```

In this case, the access strategy modifies the workspace deployment, creates and monitors additional resources to ensure that the user can access their workspace. Access strategies act on workspaces and access resources during the reconciliation loop of a workspace.

The workspace user does not need to worry, or even know anything, about all these networking scaffolding details.

Access strategies are namespaced resources, and a `workspace.spec.accessStrategy` may only reference an access strategy in its own namespace.

There is an exception to this rule: **Jupyter K8s** allows workspaces of _any_ namespace to reference access strategies in the [shared namespace](../templates/shared-namespace) - a special namespace identified at the **Jupyter K8s** operator level.


```{toctree}
:hidden:

access-resources
deployment-modifications
access-url
```
