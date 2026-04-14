# Testing: aws-hyperpod Guided Chart

This chart deploys the HyperPod access strategy with optional cluster web UI (Traefik + authmiddleware)
and SSM remote access. It requires the operator chart (`jk8s`) to be deployed first.

## Prerequisites

- An EKS cluster with Pod Identity / IRSA configured
- `.env` file with `AWS_REGION`, `EKS_CLUSTER_NAME`, and HyperPod-specific values (see `.env.example`)
- kubectl context pointing at the target cluster

```bash
make setup-aws                        # configure kubectl context from .env
```

## Clean Slate

If there's an existing deployment, tear it down in dependency order (workspaces first,
so the controller can process finalizers):

```bash
kubectl delete workspaces --all --all-namespaces --wait
helm uninstall aws-hyperpod -n jupyter-k8s-system
helm uninstall jk8s -n jupyter-k8s-system
kubectl delete crd workspaces.workspace.jupyter.org \
  workspaceaccessstrategies.workspace.jupyter.org \
  workspacetemplates.workspace.jupyter.org 2>/dev/null; true
```

Verify nothing remains:

```bash
kubectl get all -n jupyter-k8s-system
kubectl get crd | grep jupyter
```

## Build and Generate

```bash
make build                            # compile Go binaries
make helm-generate                    # generate dist/chart from config/
make helm-lint                        # lint all charts (operator + guided)
```

## Deploy

### Stack 1: Operator Only (no plugin, backward compatibility)

Tests that the controller works without `--plugin-endpoints`. No AWS plugin sidecar,
no remote access features — only k8s-native paths.

```bash
make deploy-aws                       # deploys WITHOUT PLUGINS=aws
```

**Checks:**

1. Controller pod has 1 container (`manager`, no `plugin-aws`):
   ```bash
   kubectl get pods -n jupyter-k8s-system -l control-plane=controller-manager \
     -o jsonpath='{.items[0].spec.containers[*].name}'
   ```
2. No `--plugin-endpoints` in manager args:
   ```bash
   kubectl describe pod -n jupyter-k8s-system -l control-plane=controller-manager | grep plugin-endpoints
   ```
3. Manager logs show no plugin-related errors.

### Stack 2: Operator with Plugin + HyperPod Chart

Tests full plugin sidecar deployment with AWS remote access.

```bash
make deploy-aws-with-plugin           # deploys WITH PLUGINS=aws (builds & pushes aws-plugin image)
make deploy-aws-hyperpod              # deploys the hyperpod guided chart
```

**Checks:**

1. **Controller pod has 2 containers** (manager + plugin-aws):
   ```bash
   kubectl get pods -n jupyter-k8s-system -l control-plane=controller-manager \
     -o jsonpath='{.items[0].spec.containers[*].name}'
   ```
   Expected: `manager plugin-aws`

2. **Plugin sidecar is healthy**:
   ```bash
   kubectl logs -n jupyter-k8s-system -l control-plane=controller-manager -c plugin-aws --tail=5
   ```
   Look for: healthz 200s, no errors.

3. **Manager has correct flag**:
   ```bash
   kubectl get pods -n jupyter-k8s-system -l control-plane=controller-manager \
     -o jsonpath='{.items[0].spec.containers[0].args}' | tr ',' '\n' | grep plugin
   ```
   Expected: `--plugin-endpoints=aws=http://localhost:8080`

4. **AccessStrategy rendered correctly**:
   ```bash
   kubectl get workspaceaccessstrategy hyperpod-access-strategy -n jupyter-k8s-system -o yaml
   ```
   - `podEventsHandler: "aws:ssm-remote-access"`
   - `createConnectionHandlerMap.vscode-remote: "aws:createSession"`
   - `podEventsContext` has all keys (ssmManagedNodeRole, sidecarContainerName, etc.)
   - `createConnectionContext` has podUid, ssmDocumentName, port

5. **Authmiddleware and Traefik running** (when `clusterWebUI.enabled=true`):
   ```bash
   kubectl get pods -n jupyter-k8s-system
   ```
   Expect: `workspace-auth-middleware` replicas + `workspace-traefik-router` replicas.

## Create and Test Workspaces

```bash
make apply-sample-hyperpod WS_USER=<your-user>
kubectl wait --for=condition=Available workspaces --all -n default --timeout=300s
```

### Verify SSM Registration Flow

Controller logs should show pod events dispatched to the AWS adapter:
```bash
kubectl logs -n jupyter-k8s-system -l control-plane=controller-manager -c manager \
  | grep -i "ssm\|remote\|adapter\|plugin"
```

Plugin sidecar logs should show incoming HTTP calls (register-node-agent):
```bash
kubectl logs -n jupyter-k8s-system -l control-plane=controller-manager -c plugin-aws --tail=20
```

### Verify Web-UI Connection (bearer token flow)

```bash
make bearer-token WS_NAME=<workspace-name>
```

Should return a URL with a JWT token. Open in browser (or via `make port-forward`) to
verify the authmiddleware cookie flow works.

### Verify VSCode Remote Connection

```bash
make vscode-token WS_NAME=<workspace-name>
```

Should route through `aws:createSession` -> plugin sidecar -> SSM `StartSession` and
return a `vscode://` connection URL.

## Cleanup

```bash
make delete-sample-hyperpod WS_USER=<your-user>
```

Controller logs should show `HandlePodDeleted` dispatched to AWS adapter.
Plugin sidecar logs should show SSM deregistration calls.

## Failure Modes

| Symptom | Likely cause |
|---------|-------------|
| Plugin sidecar CrashLoopBackOff | Missing env vars (PLUGIN_PORT, AWS_REGION, CLUSTER_ID) or IAM role not reaching sidecar |
| "Pod event adapter not available" in manager logs | `--plugin-endpoints` flag not set or parsed incorrectly |
| "no plugin endpoint configured for handler" on connection create | `pluginClients` map empty in extensionapi |
| SSM registration fails | IRSA/PodIdentity not shared to sidecar (both containers share the pod's SA) |
| `CLUSTER_ID` empty in sidecar | Check Makefile passes `$(EKS_CONTEXT)` correctly |
