---
layout: landing
description: A Kubernetes operator for Jupyter notebooks and interactive IDEs.
---

# Jupyter K8s

```{rst-class} lead
A Kubernetes operator for Jupyter notebooks and interactive IDEs — managing compute, storage, networking, and access control for multiple users.
```

```{container} buttons
[Get Started](getting-started/index)
[{octicon}`mark-github;1.2em` GitHub](https://github.com/jupyter-infra/jupyter-k8s)
```

---

::::{grid} 1 1 2 3
:gutter: 2
:padding: 0
:class-row: surface

:::{grid-item-card} ☸️ Kubernetes Native
Workspaces are native Kubernetes resources. Your users access them with their Kubernetes identities and RBAC policies.
:::

:::{grid-item-card} 📓 Multi-Application Support
Run JupyterLab, VS Code, or bring your own apps. Each workspace gets its own persistent storage and unique URL.
:::

:::{grid-item-card} 🛡️ Secure by Default
Scope a workspace access to a single user or a team. Namespace-scoped RBAC, JWT-based authentication with automatic seed rotation.
:::

:::{grid-item-card} 🔌 Flexible Access
Connect to your workspaces from your web browser with OAuth 2 or bearer token URL, or directly from your desktop IDE.
:::

:::{grid-item-card} ⚙️ Fine-Grained Control
Provide default configurations to your users and enforce bounds with templates. Automatically shutdown idle workspaces.
:::

:::{grid-item-card} 🏗️ Vendor Neutral
Compatible with any cloud provider via an HTTP sidecar plugin pattern. Bring your own integration.
:::

::::


```{toctree}
:hidden:

getting-started/index
core-concepts/index
applications/index
dive-deeper/index
integrations/index
contributor-guide/index
```

```{toctree}
:hidden:
:caption: Reference

reference/index
```
