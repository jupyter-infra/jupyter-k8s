(template-aws-eks-oidc)=
# AWS EKS OIDC

The **AWS EKS OIDC** template provisions a complete multi-user **JupyterLab** platform on AWS:
an EKS cluster, **Jupyter K8s**, and a browser access stack with HTTPS and GitHub OAuth. It is
the `jupyter-deploy` equivalent of bringing your own cluster and installing the
{ref}`AWS OIDC guided chart <chart-aws-oidc>` yourself.

- **Provider:** AWS (EKS)
- **Engine:** Terraform
- **Access:** OIDC web access — GitHub identities federated through Dex

It provisions an EKS cluster configured with OIDC access, installs the
[**Jupyter K8s** operator chart](../../reference/helm-charts/operator), and layers on the
{ref}`AWS OIDC guided chart <chart-aws-oidc>` for HTTPS ingress, GitHub OAuth, and browser access.
The [AWS EKS OIDC Template](https://jupyter-deploy.readthedocs.io/en/latest/templates/aws-eks-oidc-template/index.html)
documentation is the source of truth for its architecture, prerequisites, and configuration.

## Getting started

The template requires an AWS account, a domain registered with Amazon Route 53, and a GitHub OAuth app.
See the [prerequisites](https://jupyter-deploy.readthedocs.io/en/latest/templates/aws-eks-oidc-template/prerequisites.html)
for the details.

Create a Python environment for the `jd` CLI and the template. We recommend
[uv](https://github.com/astral-sh/uv):

```bash
# prepare a virtual environment
uv init . --bare
uv venv
source .venv/bin/activate

# install jupyter-deploy and the AWS EKS OIDC template
uv add "jupyter-deploy[aws,k8s]" jupyter-deploy-tf-aws-eks-oidc
```

Or with `pip`:

```bash
pip install "jupyter-deploy[aws,k8s]" jupyter-deploy-tf-aws-eks-oidc
```

Then initialize, configure, and deploy a project:

```bash
mkdir my-eks-deployment && cd my-eks-deployment
jd init . -E terraform -P aws -I eks -T oidc
jd config
jd up
```

`jd config` walks you through the template's variables and how to install any missing tools. Once the
cluster is up, you create and manage workspaces as in any **Jupyter K8s** deployment. See
[Run Workspaces](../../getting-started/run-workspaces).

## Learn more

- [AWS EKS OIDC Template](https://jupyter-deploy.readthedocs.io/en/latest/templates/aws-eks-oidc-template/index.html)
  — prerequisites, configuration, architecture, and the full user guide.
