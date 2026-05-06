# Defaults

When a workspace references a template via `workspace.spec.templateRef`, the template's defaults fill in any attributes that `workspace.spec` did not specify.

A template optionally can add labels to the `workspace.metadata.labels`.

## What gets defaulted

The general rule is a template default applies **only** when the `workspace.spec` omits this particular attribute.

For example, if a `workspace.spec` specifies a `containerSecurityContext`, the template's `defaultContainerSecurityContext` won't be applied at all.

| Template field | Workspace field it fills |
|---------------|------------------------|
| `defaultImage` | `spec.image` |
| `defaultResources` | `spec.resources` |
| `primaryStorage.defaultSize` | `spec.storage.size` |
| `primaryStorage.defaultMountPath` | `spec.storage.mountPath` |
| `primaryStorage.defaultStorageClassName` | `spec.storage.storageClassName` |
| `defaultContainerConfig` | `spec.containerConfig` |
| `defaultNodeSelector` | `spec.nodeSelector` |
| `defaultAffinity` | `spec.affinity` |
| `defaultTolerations` | `spec.tolerations` |
| `defaultOwnershipType` | `spec.ownershipType` |
| `defaultAccessType` | `spec.accessType` |
| `defaultAccessStrategy` | `spec.accessStrategy` |
| `defaultIdleShutdown` | `spec.idleShutdown` |
| `defaultLifecycle` | `spec.lifecycle` |
| `defaultPodSecurityContext` | `spec.podSecurityContext` |
| `defaultContainerSecurityContext` | `spec.containerSecurityContext` |

## Merge rules

The following `template.spec` attributes modify an attribute even when the workspace user specified a value for that attribute in the `workspace.spec` or `workspace.metadata`.


| Template field | Workspace field it fills |
|---------------|------------------------|
| `baseEnv` | `spec.env` |
| `baseLabels` | `metadata.labels` |

For such attributes, the controller **adds** the template defaults to the user-specified workspace attributes.

In case of conflict between the template default and the value specified by the workspace, the workspace attribute takes precedence.

## When defaults apply

The **mutating admission webhook** for workspaces injects defaults at workspace creation and update time.

When a `template.spec` changes, the controller does not modify running workspaces that already reference it — this is the **lazy application** model.
