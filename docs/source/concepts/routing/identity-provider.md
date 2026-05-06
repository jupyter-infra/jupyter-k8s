# Identity Provider

When using OIDC-based web access, the routing layer needs an identity provider (IdP) to authenticate users. The operator itself is IdP-agnostic — you bring your own.

## Role of the IdP

The identity provider:

1. Authenticates the user (login page, SSO, etc.)
2. Issues an OIDC token containing the user's Kubernetes identity - username and group memberships
3. The auth middleware uses the Kubernetes identity embedded in this token to authorize access using a [`Create:ConnectionAccessReview`](../connections/access-review)

## Common configurations

| Setup | Components | Use case |
|-------|-----------|----------|
| **Dex + GitHub** | Dex as OIDC bridge, GitHub as upstream IdP | Teams using GitHub organizations for access control |
| **Dex + LDAP** | Dex with LDAP connector | Enterprise environments with existing directory services |
| **Direct OIDC** | Any OIDC-compliant IdP (Okta, Azure AD, Cognito) | When the IdP natively speaks OIDC |

## Guided chart integration

The AWS guided charts bundle [Dex](https://dexidp.io/) as the identity provider and include [OAuth2 Proxy](https://oauth2-proxy.github.io/oauth2-proxy/) to issue cookies valid across all workspaces. This is just an example — you can replace either component with your own setup.

## Bearer token alternative

If you don't need browser-based OIDC login (e.g. programmatic access from CLI tools), you can set access strategies that leverage **bearer token** access. Users obtain a time-limited token via the `Create:Connection` API and pass it in the URL.

The bearer token is opaque to auth middleware, which validates it by calling [`Create:BearerTokenReview`](../connections/token-review).
