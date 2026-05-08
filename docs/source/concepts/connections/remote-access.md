# Remote Access

Remote access lets desktop IDEs (e.g. VS Code) connect directly to a workspace without going through the browser.

## How it works

1. User creates a [Connection](index) with a non-web type (e.g. `vscode-remote`).
2. **Extension API** looks up the access strategy's `createConnectionHandlerMap` for that type.
3. **Extension API** invokes the [plugin](../../integrations/plugins/index.md) handler (e.g. `aws:createSession`).
4. The plugin returns connection details — typically a session ID or tunnel endpoint.
5. The user's local IDE connects using those details.

## Example: VS Code Remote via AWS SSM

```bash
kubectl create -f - <<EOF
apiVersion: connection.workspace.jupyter.org/v1alpha1
kind: WorkspaceConnection
metadata:
  namespace: team-alice
spec:
  workspaceName: alice-workspace
  workspaceConnectionType: vscode-remote
EOF
```

The response contains a URL that the workspace user can paste in their web browser, which redirects them to their desktop IDE.

For example, VS Code desktop remote-SSH extension will then connect to the workspace without needing direct network access to the cluster.

## Plugin dispatch

The access strategy maps connection types to plugin handlers:

```yaml
spec:
  createConnectionHandlerMap:
    vscode-remote: "aws:createSession"
```

The format is `plugin:action` — in this example, the controller resolves `aws` to the plugin's HTTP endpoint and calls the `createSession` action. See [Plugin Architecture](../../integrations/index) for details.

## Pod lifecycle events

Some remote access methods require pod-level setup (e.g. registering with a session manager). The access strategy's `podEventsHandler` field triggers plugin actions on pod start/stop:

```yaml
spec:
  podEventsHandler: "aws:ssm-remote-access"
```
