# Web Access

Web access lets users open a workspace in their browser. 

**Jupyter K8s** supports two authentication flows:

(web-access-oidc-flow)=
## OIDC flow

The user navigates to the workspace URL. The **Auth middleware** (if in use) detects no valid session cookie and initiates an OAuth2/OIDC login:

1. Browser is redirected to the identity provider login page.
2. User authenticates (e.g. via GitHub).
3. IdP redirects back with an authorization code.
4. IdP exchanges the code for an ID token, verifies it, and sends it to **Auth middleware**
5. **Auth middleware** checks the user's authorization via [Create:ConnectionAccessReview](access-review).
6. **Auth middleware** sets a signed JWT cookie, scoped to the workspace path.
7. Subsequent requests present the cookie — no redirect needed.

Refer to the {ref}`AWS-OIDC guided chart <chart-aws-oidc>` for an example implementation.

(web-access-bearer-token-flow)=
## Bearer token flow

For programmatic access or environments without an IdP:

1. User creates a Connection resource via `kubectl` or the K8s API.
2. The **Extension API** signs a bearer token and returns a URL with the token embedded.
3. User opens the URL in their browser.
4. **Auth middleware** validates the bearer token via [Create:BearerTokenReview](token-review) (calling back to the Extension API).
5. **Auth middleware** sets a signed JWT cookie — subsequent requests use the cookie.

Refer to the {ref}`AWS-Hyperpod guided chart <chart-aws-hyperpod>` for an example implementation.

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
