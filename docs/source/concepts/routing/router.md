# Router

The router is a reverse proxy that terminates TLS and routes HTTPS requests to workspace pods. The operator is router-agnostic — the access strategy's resource templates determine what routing resources the operator should create.

## How it works

1. The operator creates **routing resources** (e.g. Traefik IngressRoutes) in the workspace's namespace as defined by the workspace's access strategy.
2. The router picks up these resources and configures routes automatically.
3. Every request passes through the authentication and authorization components before reaching the workspace pod.

## Path-based routing

Each workspace gets a unique path as determined by its access strategy.

For example, using URL:
```
https://{domain}/workspaces/{namespace}/{workspace-name}/
```

Or with sub-domains:
```
https://{workspace-name}.{namespace}.{domain}/
```

The router matches this path and forwards the request — after authentication and authorization — to the workspace's Kubernetes Service which exposes the application's port.

## Router choice

The guided charts use [Traefik](https://traefik.io/) as the default router, but any ingress controller that supports:

- Forward-auth middleware (delegating authorization to an external service)
- Path-based routing
- WebSocket upgrades

can serve as the router.

The `spec.accessResourceTemplates` attribute of an access strategy lets you configure any Kubernetes resource (IngressRoute, Ingress, HTTPRoute, etc.)
the router needs to connect to your workspaces.
