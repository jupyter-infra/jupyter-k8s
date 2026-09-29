# Example setup

The `ray-integration` template in the [`aws-hyperpod` chart](https://github.com/jupyter-infra/jupyter-k8s-aws/blob/main/charts/aws-hyperpod/templates/ray-integration-template.yaml) is a complete, maintained WorkspaceIntegrationTemplate, and the reference to read for a worked example. It attaches a workspace to an existing `RayCluster`: it resolves the cluster named by its `rayClusterName` parameter, injects a sidecar that joins the cluster as a zero-resource client node, mounts the shared session directories, sets the Ray environment variables in the workspace container, and reports reachability through a `statusProbe`.

It also carries the parts a production template needs that a minimal example would omit — a retry loop, resource sizing, security context, and GPU handling.

For the field list and validation rules, see the [WorkspaceIntegrationTemplate reference](../../reference/custom-resources/workspaceintegrationtemplate).
