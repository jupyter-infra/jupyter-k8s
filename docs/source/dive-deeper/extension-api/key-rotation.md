# Key Rotation

**Extension API**'s HMAC signing keys rotate automatically via a CronJob deployed by the **Jupyter K8s** chart.

## Setup

When `extensionApi.jwtSecret.enable=true`, the Helm chart creates:
- A Kubernetes Secret (default name: `extensionapi-jwt-secrets`) in the `jupyter-k8s-system` namespace
- A CronJob running the rotator image on a configurable schedule

## Rotation behavior

1. The rotator generates a new HMAC key and adds it to the Secret.
2. Old keys remain up to a configurable retention count (default: 3).
3. Keys beyond the retention limit are pruned.

## Graceful overlap

Each controller manager pod watches the signing Secret for changes, and sends them to their **Extension API** process. When a new key appears:

- A `newKeyUseDelay` (default: 5 seconds) ensures all replicas observe the new key before it is used for signing.
- Each token carries a `kid` (key ID) in its header. Validation looks up the key by `kid`, so tokens signed with a previous key remain valid as long as that key is still in the Secret.

Because bearer tokens are short-lived (5 minutes TTL), the overlap window between rotation events is more than sufficient for uninterrupted operation.

## Secret watching

**Extension API**'s `StandardSigner` registers a controller-runtime event handler on the signing Secret. On any update:
1. It reloads all keys from the Secret.
2. It selects the newest key (by timestamp annotation) as the active signing key, respecting the `newKeyUseDelay`.
3. Older keys remain available for validation only.
