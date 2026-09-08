# Example setup

This is a simplified version of the production `ray-integration` template shipped in the [`aws-hyperpod` chart](https://github.com/jupyter-infra/jupyter-k8s-aws/blob/main/charts/aws-hyperpod/templates/ray-integration-template.yaml). An admin installs it to attach a workspace to an existing `RayCluster`: it fetches the cluster named by the `rayClusterName` parameter, injects a Ray sidecar that joins the cluster as a zero-resource client node, mounts the shared session directories, sets the Ray environment variables in the workspace container, and reports Ray reachability through a `statusProbe`.

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceIntegrationTemplate
metadata:
  name: ray-integration
  namespace: alice-team
spec:
  displayName: Ray Cluster Integration
  parameters:
    - name: rayClusterName          # which RayCluster to join
  # Share the sidecar's PID namespace so the workspace can attach to its local Ray session.
  shareProcessNamespace: true
  # Fetch the customer-named RayCluster live to read its head image, head service, and GCS port.
  resourceRefs:
    - name: rayCluster
      apiVersion: ray.io/v1
      kind: RayCluster
      metadata:
        name: "{{ .Parameters.rayClusterName }}"
  # Report-only: the operator execs `ray status` in the workspace container; it never gates the pod.
  statusProbe:
    exec:
      command: ["ray", "status"]
  deploymentModifications:
    podModifications:
      additionalContainers:
        - name: ray-sidecar
          # Match the sidecar's Ray binary to the cluster by reusing the head image.
          image: '{{ resource "rayCluster" "{.spec.headGroupSpec.template.spec.containers[0].image}" }}'
          command: ["/bin/sh", "-c"]
          # Join the cluster as a zero-resource client node so no compute lands on the sidecar.
          args:
            - >-
              ray start
              --address={{ resource "rayCluster" "{.status.head.serviceName}" }}.{{ .Workspace.Namespace }}.svc.cluster.local:{{ resource "rayCluster" "{.status.endpoints.gcs-server}" }}
              --num-cpus=0 --num-gpus=0 --temp-dir=/tmp/ray --block
          volumeMounts:
            - { name: ray-tmp, mountPath: /tmp/ray }
            - { name: ray-dshm, mountPath: /dev/shm }
      volumes:
        - name: ray-tmp
          emptyDir: { medium: Memory, sizeLimit: 256Mi }
        - name: ray-dshm
          emptyDir: { medium: Memory, sizeLimit: 2Gi }
      primaryContainerModifications:
        # Mount the same session dir + /dev/shm into the workspace container.
        volumeMounts:
          - { name: ray-tmp, mountPath: /tmp/ray }
          - { name: ray-dshm, mountPath: /dev/shm }
        mergeEnv:
          - name: RAY_ADDRESS          # ray.init() attaches to the sidecar's local session
            valueTemplate: "auto"
          - name: RAY_CLUSTER_NAME
            valueTemplate: "{{ .Parameters.rayClusterName }}"
```

A workspace user in `alice-team` attaches it and supplies the cluster name:

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: alice-workspace
  namespace: alice-team
spec:
  displayName: Alice's Workspace
  image: my-repository/my-image:my-tag
  integrationTemplateRefs:
    - name: ray-integration
      parameters:
        - name: rayClusterName
          value: team-ray
```

On reconcile, the controller fetches the `team-ray` `RayCluster` in `alice-team`, renders the sidecar image and join address from its live status, injects the sidecar, volumes, and `RAY_*` environment variables into the workspace pod, execs `ray status` for the probe verdict, and records the resolved values in `status.resolvedIntegrations`. The workspace user runs `ray.init()` and connects; they never need to know the cluster's address.

For the full field list and validation rules, see the [WorkspaceIntegrationTemplate reference](../../reference/custom-resources/workspaceintegrationtemplate). For the complete production template (retry loop, resource sizing, security context, GPU handling), see the [`aws-hyperpod` chart](https://github.com/jupyter-infra/jupyter-k8s-aws/blob/main/charts/aws-hyperpod/templates/ray-integration-template.yaml).
