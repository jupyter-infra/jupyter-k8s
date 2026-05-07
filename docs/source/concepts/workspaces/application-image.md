# Application Image

The `workspace.spec.image` attribute determines what software runs in the workspace's container. Any container image that listens on an HTTP port works — common choices include:

- **JupyterLab**
- **VS Code** (code-server, OpenVSCode Server)
- **Custom images** built on top of the above

## Image selection with templates

When a workspace references a template, the template controls which images are allowed:

| Template setting | Effect |
|-----------------|--------|
| `defaultImage` | Used when the workspace omits `spec.image` |
| `allowedImages` | AllowList — workspace may pick from this list |
| `allowCustomImages: true` | Any image is accepted |

If the template defines `allowedImages` and the workspace specifies an image not in the list, the admission webhook rejects the request.

## Container configuration

You can override the image's default entrypoint with `spec.containerConfig`:

```yaml
spec:
  containerConfig:
    command: ["/bin/sh"]
    args: ["-c", "jupyter lab --ip=0.0.0.0 --port=8888"]
```

Templates can provide a `defaultContainerConfig` that applies when the workspace doesn't specify one.
