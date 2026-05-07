# Bounds

Templates can enforce constraints that prevent workspace users from creating or updating a workspace that exceeds resource limits or uses unauthorized configurations.

Note that these bounds only apply to a workspace that references the template.

To prevent workspace users from creating arbitrary workspaces, cluster administrators can combine two mechanisms:

1. **Shared namespace with a default template** — configure a [default template](shared-namespace) in the shared namespace. The mutating webhook automatically assigns it to any workspace created without an explicit `templateRef`. This is the recommended approach for most clusters.

2. **ValidationAdmissionPolicy** — for stricter enforcement, write a [ValidatingAdmissionPolicy](https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/) that rejects any workspace without a `templateRef`. This is a more DIY approach that gives administrators full control over the validation logic using CEL expressions.

The two approaches can be combined: the default template covers the common case, while the admission policy acts as a safety net rejecting workspaces that somehow bypass the mutating webhook.

## Resource bounds

The `resourceBounds` field defines min/max ranges for any Kubernetes resource type:

```yaml
spec:
  resourceBounds:
    resources:
      cpu:
        min: "500m"
        max: "8"
      memory:
        min: "1Gi"
        max: "32Gi"
      nvidia.com/gpu:
        min: "0"
        max: "4"
```

If a workspace requests resources outside these ranges, the **validating admission webhook** for workspaces rejects the request.

## Image restrictions

| Field | Effect |
|-------|--------|
| `allowedImages` | Only images in this list are accepted |
| `allowCustomImages: true` | Any image is accepted (overrides the list) |
| Neither set | Only `defaultImage` is allowed |

## Storage bounds

```yaml
spec:
  primaryStorage:
    minSize: 5Gi
    maxSize: 100Gi
```

## Idle shutdown bounds

```yaml
spec:
  idleShutdownOverrides:
    allow: true
    minIdleTimeoutInMinutes: 15
    maxIdleTimeoutInMinutes: 480
```

## Environment and label requirements

Templates can require specific environment variables or labels with regex validation:

```yaml
spec:
  envRequirements:
    - name: TEAM_ID
      required: true
      regex: "^team-[a-z0-9]+$"
  labelRequirements:
    - key: cost-center
      required: true
```

## Enforcement model

Bounds are enforced **lazily** — only during workspace CREATE and UPDATE operations via the **validating admission webhook** for workspaces.
Said another way, template changes do not trigger proactive re-validation of running workspaces.

**Note:** it is **always** possible to stop a workspace, even if the `workspace.spec` no longer respects the latest bounds of the template it references.
