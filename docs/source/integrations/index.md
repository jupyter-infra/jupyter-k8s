# Integrations

**Jupyter K8s** is vendor-neutral — cloud-specific functionality is decoupled from the core controller via an HTTP sidecar plugin pattern.

## Plugin architecture

The controller is the HTTP client; each plugin runs as a sidecar container on `localhost` in the same pod as the controller.

```text
┌──────────────────────────────────────────┐
│  Operator Pod                            │
│                                          │
│  ┌────────────┐   HTTP     ┌──────────┐  │
│  │ Controller ├───────────►│  Plugin  │  │
│  └────────────┘ localhost  └─────┬────┘  │
│                                  │       │
└──────────────────────────────────┘───────┘
                                   │
                              Cloud API
                           (e.g. AWS SSM)
```

The controller never imports cloud SDKs directly. All cloud operations flow through the plugin's HTTP interface.

## Guided charts

Guided charts are opinionated Helm charts that provide working deployments for **Jupyter K8s**. They are frequently tied to a specific cloud provider, and rely on cloud-specific operators and resource annotations.

Some charts only create resources on the routing layer — selecting a router, authentication, and authorization components.

Other charts bundle **Jupyter K8s** with a plugin, the routing layer, and preconfigured access strategies — everything needed for a specific deployment scenario in a single `helm install`.

See [AWS](aws) for the currently available guided charts.

```{toctree}
:hidden:

plugins
aws
```
