# jupyter-k8s
Jupyter k8s is an open-source project that provides a secure-by-default but
flexible way to run JupyterLab applications natively on Kubernetes.

## Description
Jupyter k8s provides a Kubernetes custom operator to manage and run JupyterLab application
on your Kubernetes cluster.

It provides a set of custom resource definitions (CRDs) distributed as an helm chart, a controller image
distributed on a docker repository, and a set of default application images distributed on docker repositories.

## Getting Started

### Prerequisites
- go version v1.24.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To setup a local Kind cluster
```sh
make setup-kind
```

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/jupyter-k8s:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/jupyter-k8s:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

**Teardown the kind cluster**
```sh
make teardown-kind
```

### Remote Cluster Testing on AWS

**Setup**
```sh
make setup-aws
```

**NOTE:** the setup assumes that there exists an EKS cluster in your AWS account in region `us-west-2`
whose name is `jupyter-k8s-cluster`. You can pass AWS_REGION or EKS_CLUSTER_NAME to all methods
below to use a different config, e.g. `make setup AWS_REGION=us-east-1 EKS_CLUSTER_NAME=my-cluster`

**Install on remote cluster**
```sh
make deploy-aws
```

**Testing**
```sh
kubectl apply -k config/samples/
make port-forward
kubectl delete -k config/samples/
```


## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/jupyter-k8s:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/jupyter-ai-contrib/jupyter-k8s/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Generate the helm chart

```sh
make helm-generate
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes.

To review the effect of helm chart values substitution
```sh
make helm-test
```
Which writes the results at: `./dist/test-output`


## Contributing

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

**Compile the controller code**
```sh
make build
```

**Run the linter**
```sh
make lint
```

**Run the unit tests**
```sh
make test
```

**Generate the helm chart**
```sh
test helm-generate
```

**Run the end-to-end tests**
```sh
make test-e2e
```


## License

MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

