# Integrations

**Jupyter K8s** is vendor-neutral, and agnostic to the choice of router, authentication and authorization components.

Cloud-specific functionalities integrate at the controller or **Extension API** level with HTTP sidecar plugins.

Additional helm charts and deployment templates provide examples of integration with a specific cloud and routing components.

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

The controller does not make external API calls directly. All cloud operations flow through the plugin's HTTP interface.

## Helm charts

The operator chart deploys the core controller, **Extension API** and CRDs, but a production deployment typically needs additional charts to:

- **Configure the routing layer** — select a reverse proxy, authentication mechanism, and authorization components.
- **Integrate with cloud providers** — deploy cloud-specific plugins and resources (e.g. ALB ingress, SSM activations).
- **Define access strategies** —integrate workspaces with the routing layer by creating access strategies.


```{toctree}
:hidden:

plugins/index
guided-charts/index
```
