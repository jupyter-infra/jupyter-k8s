# Run Workspaces

After installing **Jupyter K8s** in your Kubernetes cluster, you can create workspaces to run your applications.

See [Concepts: Workspaces](../concepts/workspaces/index) for more details.

You'll need to reference your application image, see [Applications](../applications/index) for more details.

## Create a workspace

Apply a minimal `Workspace` resource:

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: my-notebook
spec:
  displayName: My Notebook
  image: <your-repo>/<your-image>:<your-tag>
  desiredStatus: Running
```

```bash
kubectl apply -f workspace.yaml
```

The controller creates a deployment, a PVC, and a service for this workspace.

## Check workspace status

```bash
kubectl get workspace my-notebook
```

The workspace reports its condition columns:

```text
NAME          AVAILABLE   PROGRESSING   DEGRADED   AGE
my-notebook   True        False         False      30s
```

For more detail:

```bash
kubectl describe workspace my-notebook
```

## Connect to the workspace

How you connect depends on your access strategy. The simplest method is port forwarding:

```bash
kubectl port-forward svc/my-notebook 8888:8888
```

Then open `http://localhost:8888` in your browser.

For production deployments with HTTPS routing, see [Concepts: Routing](../concepts/routing/index) and [Concepts: Connections](../concepts/connections/index).

## Stop and restart

To stop a workspace (pod removed, storage preserved):

```bash
kubectl patch workspace my-notebook --type merge -p '{"spec":{"desiredStatus":"Stopped"}}'
```

To restart:

```bash
kubectl patch workspace my-notebook --type merge -p '{"spec":{"desiredStatus":"Running"}}'
```

## Using a template

Templates provide default configuration and enforce bounds. When a template is available in the [shared namespace](../concepts/templates/shared-namespace):

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: my-notebook
spec:
  displayName: My Notebook
  templateRef:
    name: production-notebook-template
    namespace: jupyter-k8s-shared
  desiredStatus: Running
```

The controller fills in defaults from the template (image, resources, storage) for any field not explicitly set on the workspace.

See [Concepts: Templates](../concepts/templates/index) for details on how defaults and bounds work.
