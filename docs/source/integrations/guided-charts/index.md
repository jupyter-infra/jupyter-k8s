# Guided Charts

Guided charts are Helm charts that integrate **Jupyter K8s** with a specific cloud and routing components.

## Choosing a chart

Ask yourself:

1. **What cloud am I on?** — Charts generally target a specific cloud.
2. **How will users authenticate?** — OIDC (e.g. GitHub), bearer token (e.g. via cloud provider SDK), something else?
3. **Which interfaces do you want?** — Browser, desktop IDE, or both?


## Available charts

| Chart | Cloud | Router & TLS | Identities | Admission | Access Types |
|---|---|---|---|---|---|
| {ref}`AWS OIDC <chart-aws-oidc>` | AWS (EKS) | Traefik with Let's Encrypt + NLB | OIDC (GitHub) |  Dex + Oauth2-proxy + **Auth middleware** | Web browser |
| {ref}`AWS HyperPod <chart-aws-hyperpod>` | AWS (HyperPod) | Traefik + ALB with ACM | bearer token (IAM) |  **Auth middleware** | Web browser, desktop IDE |

```{toctree}
:hidden:

aws-charts
```
