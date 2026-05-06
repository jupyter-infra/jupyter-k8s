# Web Access

Web access lets users open a workspace in their browser. 

**Jupyter K8s** supports two authentication flows:

## OIDC flow

The user navigates to the workspace URL. The auth middleware (if in use) detects no valid session cookie and initiates an OAuth2/OIDC login:

1. Browser is redirected to the identity provider login page.
2. User authenticates (e.g. via GitHub).
3. IdP redirects back with an authorization code.
4. IdP exchanges the code for an ID token, verifies it, and sends it to Auth middleware
5. Auth middleware checks the user's authorization via `Create:ConnectionAccessReview`.
6. A signed JWT cookie is set, scoped to the workspace path.
7. Subsequent requests present the cookie — no redirect needed.

## Bearer token flow

For programmatic access or environments without an IdP:

1. User creates a Connection resource via `kubectl` or the K8s API.
2. The Extension API signs a bearer token and returns a URL with the token embedded.
3. User opens the URL in their browser.
4. Auth middleware (if in use) validates the bearer token via `Create:BearerTokenReview` (calling back to the Extension API).
5. A signed JWT cookie is set — subsequent requests use the cookie.

## Creating a web connection

```bash
kubectl create -f - <<EOF
apiVersion: connection.workspace.jupyter.org/v1alpha1
kind: WorkspaceConnection
metadata:
  namespace: team-alice
spec:
  workspaceName: alice-notebook
  workspaceConnectionType: web-ui
EOF
```

The response includes the connection URL in `status.workspaceConnectionUrl`.
