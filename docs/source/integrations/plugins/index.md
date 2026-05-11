# Plugins

A plugin is an HTTP server that runs alongside the controller container in the manager pod.

![Plugin dispatch](/_static/img/diagrams/plugin-dispatch.svg)

## Handler dispatch

Access strategies map operations to plugin handlers using a `plugin:action` format:

```yaml
spec:
  podEventsHandler: "aws:ssm-remote-access"
  createConnectionHandlerMap:
    vscode-remote: "aws:createSession"
```

The controller parses the handler reference, resolves the plugin name to its HTTP endpoint, and dispatches the request.

## Multi-plugin support

**Jupyter K8s** supports multiple plugins via `PluginEndpoints` configuration — a map from plugin name to localhost endpoint. Different access strategies can route to different plugins.

## Writing a plugin

A plugin is a Go binary that:

1. Imports `github.com/jupyter-infra/jupyter-k8s-plugin/pluginserver` for the HTTP server framework.
2. Implements the relevant handler interfaces.
3. Runs as a sidecar container in the controller pod, listening on localhost.

The `jupyter-k8s-plugin` [repository](https://github.com/jupyter-infra/jupyter-k8s-plugin) provides the shared interfaces, HTTP client, and server framework.

```{toctree}
:hidden:

aws-plugin
```
