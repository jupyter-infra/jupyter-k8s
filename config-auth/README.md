# Auth Components for Local Kind Testing

This directory contains Kustomize manifests for deploying the authmiddleware and JWT rotator to a local Kind cluster for development and testing.

## Components

- **Authmiddleware**: JWT-based authentication service that watches for secret updates
- **Rotator**: CronJob that rotates JWT signing keys every 5 minutes
- **Secret**: Initial JWT signing key (hardcoded for local testing only)

## Quick Start

```bash
# Deploy to Kind cluster
make deploy-auth-kind

# Update after code changes
make update-auth-kind

# Remove deployment
make undeploy-auth-kind
```

To serve HTTPS, pass `AUTH_KIND_OVERLAY=tls` to the deploy and undeploy targets (see [TLS](#tls)).

## Verification

### 1. Check Authmiddleware Health

```bash
# Check pod is running
kubectl get pods -n jupyter-k8s-router

# Check startup logs
kubectl logs -n jupyter-k8s-router deployment/jupyter-k8s-authmiddleware

# Test health endpoint
kubectl port-forward -n jupyter-k8s-router svc/jupyter-k8s-authmiddleware 8080:8080
curl http://localhost:8080/health
# Expected: {"status":"ok","time":"..."}
```

### 2. Verify Secret Watch

Watch authmiddleware logs in real-time:
```bash
kubectl logs -n jupyter-k8s-router deployment/jupyter-k8s-authmiddleware -f
```

Expected log messages:
- On startup: `"Secret added event received"`
- On rotation: `"Secret updated event received"`
- After update: `"Successfully updated signing keys from secret"`

### 3. Test Secret Rotation

#### Manual Rotation
```bash
# Trigger manual rotation
kubectl create job -n jupyter-k8s-router --from=cronjob/jupyter-k8s-rotator manual-test-1

# Check job completed
kubectl get jobs -n jupyter-k8s-router manual-test-1

# View rotation logs
kubectl logs -n jupyter-k8s-router job/manual-test-1
```

#### Automatic Rotation
The CronJob runs every 5 minutes automatically. Check scheduled jobs:
```bash
kubectl get cronjobs -n jupyter-k8s-router
kubectl get jobs -n jupyter-k8s-router -l app=jwt-rotator
```

#### Verify Key Count
```bash
# List current JWT signing keys
kubectl get secret jupyter-k8s-authmiddleware-secrets -n jupyter-k8s-router -o json | jq -r '.data | keys[]'
```

Expected behavior:
- Initial deployment: 1 key (hardcoded)
- After 1st rotation: 2 keys
- After 2nd rotation: 3 keys (max configured)
- After 3rd rotation: 3 keys (oldest key pruned)

### 4. End-to-End Test

Complete rotation cycle verification:
```bash
# 1. Check initial state
kubectl get secret jupyter-k8s-authmiddleware-secrets -n jupyter-k8s-router -o json | jq -r '.data | keys[]'

# 2. Trigger rotation
kubectl create job -n jupyter-k8s-router --from=cronjob/jupyter-k8s-rotator test-rotation

# 3. Wait for completion
kubectl wait --for=condition=complete --timeout=30s job/test-rotation -n jupyter-k8s-router

# 4. Verify new key added
kubectl get secret jupyter-k8s-authmiddleware-secrets -n jupyter-k8s-router -o json | jq -r '.data | keys[]'

# 5. Check authmiddleware detected the change
kubectl logs -n jupyter-k8s-router deployment/jupyter-k8s-authmiddleware | grep "Secret updated event received"
```

## TLS

The `tls` overlay serves authmiddleware over HTTPS with a self-signed certificate issued by cert-manager
(installed by `make setup-kind`). It sets `TLS_CERT_FILE` and `TLS_KEY_FILE`, mounts the certificate
Secret, and switches the probes to HTTPS.

```bash
make deploy-auth-kind AUTH_KIND_OVERLAY=tls
kubectl wait certificate/jupyter-k8s-authmiddleware-tls -n jupyter-k8s-router --for=condition=Ready
```

Check the health endpoint (the server requires TLS 1.3, so use a curl built against OpenSSL 1.1.1 or later):
```bash
kubectl port-forward -n jupyter-k8s-router svc/jupyter-k8s-authmiddleware 8080:8080
curl -k https://localhost:8080/health
```

### Verify certificate rotation

The certificate is valid for 1h and renewed 55m before expiry, so cert-manager reissues it about every
5 minutes. The pod should serve each new certificate without restarting:
```bash
# Serial in the Secret
kubectl get secret jupyter-k8s-authmiddleware-tls -n jupyter-k8s-router -o jsonpath='{.data.tls\.crt}' \
  | base64 -d | openssl x509 -noout -serial
# Serial served (with the port-forward above running)
echo | openssl s_client -connect localhost:8080 2>/dev/null | openssl x509 -noout -serial
# Reload events, and a restart count that stays at 0
kubectl logs -n jupyter-k8s-router deployment/jupyter-k8s-authmiddleware | grep "Updated current TLS certificate"
kubectl get pods -n jupyter-k8s-router -l app=authmiddleware
```

The kubelet can take up to a minute to project a renewed Secret into the pod, after which the served
serial matches the Secret within about 10 seconds.

## Configuration

All configuration values are centralized in `default/kustomization.yaml` for easy customization. To modify any setting, edit the `configMapGenerator` section:

```yaml
configMapGenerator:
- name: auth-config
  literals:
  - ROTATION_SCHEDULE=*/5 * * * *      # How often to rotate keys (cron format)
  - NUMBER_OF_KEYS=3                   # Max keys to retain (must satisfy: numberOfKeys * rotationInterval >= jwtExpiration + 30m)
  - JWT_EXPIRATION=1h                  # JWT token lifetime
  - JWT_NEW_KEY_USE_DELAY=30s          # Delay before using newly rotated keys
  - JWT_REFRESH_WINDOW=10m             # Start refreshing this long before expiration
  - JWT_REFRESH_HORIZON=2h             # Must be >= JWT_EXPIRATION
  - AUTHMIDDLEWARE_IMAGE=docker.io/library/authmiddleware:local
  - ROTATOR_IMAGE=docker.io/library/rotator:local
```

After modifying values:
1. Redeploy: `kubectl apply -k config-auth/default`
2. The changes will automatically propagate to authmiddleware deployment and rotator CronJob

**Key Relationships:**
- `JWT_REFRESH_HORIZON` must be >= `JWT_EXPIRATION` (validation enforced at startup)
- `NUMBER_OF_KEYS * rotationInterval` should be >= `JWT_EXPIRATION + 30min` for safe overlap
- Example: 3 keys × 5min rotation = 15min retention (covers 60min JWT + buffer)

## Notes

- The hardcoded initial secret is **only for local Kind testing** and is not sensitive
- Production deployments use the Helm chart which generates random keys
- The rotator automatically prunes old keys when the count exceeds `NUMBER_OF_KEYS`
- All resources are deployed to the `jupyter-k8s-router` namespace with `jupyter-k8s-` prefix
