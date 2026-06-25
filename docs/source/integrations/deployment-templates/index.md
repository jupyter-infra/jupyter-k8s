# Deployment Templates

[`jupyter-deploy`](https://jupyter-deploy.readthedocs.io/en/latest/) (`jd`) is a vendor-neutral
CLI that deploys interactive applications from scratch with the cloud provider and
infrastructure-as-code engine of your choice.

Each `jd` **template** bundles the infrastructure, configuration, and deployment logic into a
project you drive with a few commands — `jd init`, `jd config`, `jd up`. Some templates provision
a Kubernetes cluster and install **Jupyter K8s** along with a routing and identity layer, so `jd`
becomes a turnkey way to stand up the operator and everything around it.

Unlike a [guided chart](../guided-charts/index), which installs into a cluster you already run, a
deployment template also creates the cluster and the surrounding cloud resources.

## Templates that use Jupyter K8s

| Template | Provider | Engine | What it deploys |
|---|---|---|---|
| [AWS EKS OIDC](aws-eks-oidc) | AWS (EKS) | Terraform | An EKS cluster, **Jupyter K8s**, and a guided HTTPS + GitHub OAuth (Dex) stack for multi-user **JupyterLab** workspaces. |

See the [`jupyter-deploy` templates](https://jupyter-deploy.readthedocs.io/en/latest/templates/index.html)
for the full list of officially supported templates. `jd init` also discovers and lets you choose
templates interactively.

```{toctree}
:hidden:

aws-eks-oidc
```
