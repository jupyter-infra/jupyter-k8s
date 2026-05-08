# Routing

Routing connects a user's browser to the correct workspace pod, with authentication and authorization enforced on every request.

The routing layer lives in a dedicated namespace, `jupyter-k8s-router` by default.

The routing namespace typically comprises three main elements:
1. a router
2. authentication components
3. authorization components

**Jupyter K8s** does not make any assumption as to which specific router, authentication or authorization components you choose to use in your cluster.

For authorization, **Jupyter K8s** provides a native implementation which calls the **Extension API** to authorize workspace access, but you are free to use your own authorization middleware.

The following sections provide examples of router, authentication and authorization implementations.

```{toctree}
:hidden:

router
authmiddleware
identity-provider
```
