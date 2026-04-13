# Testing: aws-traefik-dex Guided Chart

This chart deploys the OSS routing stack: Traefik reverse proxy, Dex OIDC provider with GitHub OAuth,
OAuth2-proxy, authmiddleware, and JWT key rotation. It does NOT use `--plugin-endpoints` — there are
no plugins. It exercises the k8s-native bearer token path (HMAC JWT via Kubernetes Secret).

## Prerequisites

- An EKS cluster (the OSS chart does not require Pod Identity / IRSA)
- `.env` file with `AWS_REGION`, `EKS_CLUSTER_NAME`, domain, GitHub OAuth, and EFS values (see `.env.example`)
- kubectl context pointing at the target cluster
- A domain with DNS pointing to the cluster (for Traefik ingress + Let's Encrypt)

```bash
make setup-aws                        # configure kubectl context from .env
```

**Default cluster:** `jupyter-k8s-cluster` in `us-west-2` (`.env` defaults).
If switching from another cluster (e.g. parker in us-east-2), update `.env` and run:

```bash
make kubectl-aws                      # switch kubectl context
```

## Clean Slate

If there's an existing deployment, tear it down in dependency order:

```bash
kubectl delete workspaces --all --all-namespaces --wait
helm uninstall aws-traefik-dex -n jupyter-k8s-system 2>/dev/null; true
helm uninstall jk8s -n jupyter-k8s-system
kubectl delete crd workspaces.workspace.jupyter.org \
  workspaceaccessstrategies.workspace.jupyter.org \
  workspacetemplates.workspace.jupyter.org 2>/dev/null; true
```

## Build and Generate

```bash
make build                            # compile Go binaries
make helm-generate                    # generate dist/chart from config/
make helm-lint                        # lint all charts (operator + guided)
```

## Deploy

The OSS chart is deployed without plugins:

```bash
make deploy-aws                       # operator chart WITHOUT PLUGINS=aws
make deploy-aws-traefik-dex           # OSS guided chart (reads .env for domain, GitHub, EFS)
```

**Checks:**

1. **Controller pod has 1 container** (no sidecar):
   ```bash
   kubectl get pods -n jupyter-k8s-system -l control-plane=controller-manager \
     -o jsonpath='{.items[0].spec.containers[*].name}'
   ```
   Expected: `manager` (no `plugin-aws`)

2. **No `--plugin-endpoints` in manager args**:
   ```bash
   kubectl describe pod -n jupyter-k8s-system -l control-plane=controller-manager | grep plugin-endpoints
   ```
   Expected: empty

3. **Manager logs** show no plugin-related errors:
   ```bash
   kubectl logs -n jupyter-k8s-system -l control-plane=controller-manager -c manager --tail=20
   ```

4. **AccessStrategy uses k8s-native only** (no `createConnectionHandlerMap`, no `podEventsContext`):
   ```bash
   kubectl get workspaceaccessstrategy -n jupyter-k8s-system -o yaml
   ```
   - `createConnectionHandler: "k8s-native"`
   - No `podEventsHandler`, `createConnectionHandlerMap`, or `podEventsContext` fields

5. **Full routing stack is running**:
   ```bash
   kubectl get pods -n jupyter-k8s-system
   ```
   Expect: controller, authmiddleware replicas, traefik replicas, and completed rotator jobs.

## Create and Test Workspaces

```bash
make apply-sample-routing WS_USER=<your-user>
kubectl get workspaces -n default
kubectl wait --for=condition=Available workspaces --all -n default --timeout=300s
```

### Verify Bearer Token Flow (Web UI)

```bash
make bearer-token WS_NAME=<workspace-name>
```

Should return a URL with a JWT token. Open in browser — the bearer auth middleware creates
a session JWT and sets a cookie, then routes through Traefik to the workspace.

Alternatively, test via port-forward:
```bash
make port-forward
```

### Verify GitHub OAuth Flow

Access a workspace URL directly in browser (without bearer token):
- Should redirect to Dex -> GitHub login -> workspace
- Only users in the configured GitHub org/team should be granted access

### Verify JWT Key Rotation

Check that the rotator CronJob is running and the secret has multiple keys:
```bash
kubectl get cronjob -n jupyter-k8s-system
kubectl get secret authmiddleware-secrets -n jupyter-k8s-system -o jsonpath='{.data}' | jq 'keys'
```

## Cleanup

```bash
make delete-sample-routing
```

No errors should appear in controller logs.

## Failure Modes

| Symptom | Likely cause |
|---------|-------------|
| Authmiddleware CrashLoopBackOff | JWT secret not found — check `secretName` matches between auth deployment and rotator |
| Certificate not issued | cert-manager not installed, or DNS not pointing to cluster |
| OAuth redirect fails | GitHub OAuth app callback URL mismatch with domain, or Dex configmap has wrong issuer URL |
| Bearer token returns 403 | `enableBearerAuth` not set to true, or RBAC missing for `bearertokenreviews` |
| Rotator job fails | ServiceAccount missing RBAC to update the JWT secret |
