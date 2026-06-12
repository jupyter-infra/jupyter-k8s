#!/usr/bin/env python3
"""Inject helm-docs `# --` annotations into the generated chart values.yaml.

`make helm-generate` rebuilds dist/chart/values.yaml from scratch (kubebuilder
helm plugin + hack/helm-patches/values.yaml.patch), which wipes any hand-added
helm-docs annotations. helm-docs (the `docs-helm-ref` target) reads those
`# --` comments to produce docs/source/reference/helm-charts/operator.md, so
they must be reproduced deterministically after every regen.

This script is the single source of truth for those annotations. It walks
values.yaml tracking the dotted path of each key by indentation and inserts the
mapped `# -- <comment>` line above the key (idempotently). Add an entry to
ANNOTATIONS when a new value is introduced.
"""
import re
import sys

# Dotted value path -> helm-docs annotation text.
ANNOTATIONS = {
    "fullnameOverride": "String to fully override chart.fullname template",
    "manager.replicas": "Number of controller manager replicas",
    "manager.image.repository": "Controller image repository",
    "manager.image.tag": "Controller image tag (defaults to chart appVersion)",
    "manager.image.pullPolicy": "Image pull policy",
    "manager.args": "Controller manager arguments",
    "manager.env": "Environment variables for the controller container",
    "manager.envOverrides": "Environment variable overrides (same name in env above: this value takes precedence)",
    "manager.imagePullSecrets": "Image pull secrets for private registries",
    "manager.podSecurityContext": "Pod-level security settings",
    "manager.securityContext": "Container-level security settings",
    "manager.resources": "Resource limits and requests for the controller container",
    "manager.affinity": "Node affinity rules for the manager pod",
    "manager.nodeSelector": "Node selector for the manager pod",
    "manager.tolerations": "Tolerations for the manager pod",
    "manager.topologySpreadConstraints": "Topology spread constraints for the manager pod",
    "manager.podDisruptionBudget": "PodDisruptionBudget configuration",
    "manager.podDisruptionBudget.enabled": "Enable PodDisruptionBudget",
    "manager.podDisruptionBudget.minAvailable": "Minimum available pods",
    "manager.podDisruptionBudget.maxUnavailable": "Maximum unavailable pods",
    "manager.pod": "Pod metadata",
    "manager.pod.labels": "Additional labels for the manager pod",
    "manager.pod.annotations": "Additional annotations for the manager pod",
    "rbacHelpers.enable": "Install convenience admin/editor/viewer roles for CRDs",
    "crd.enable": "Install CRDs with the chart",
    "crd.keep": "Keep CRDs when uninstalling",
    "metrics.enable": "Enable metrics endpoint",
    "metrics.port": "Metrics server port",
    "certManager.enable": "Enable cert-manager integration (required for webhooks and metrics TLS)",
    "webhook.enable": "Enable admission webhooks",
    "webhook.port": "Webhook server port",
    "prometheus.enable": "Enable Prometheus ServiceMonitor",
    "application.imagesPullPolicy": "Image pull policy for workspace pod containers",
    "application.imagesRegistry": "Image registry prefix for workspace pod containers",
    "workspaceTemplates.defaultNamespace": "Namespace where shared workspace templates are stored",
    "workspacePodWatching.enable": "Enable workspace pod event watching for lifecycle management (required for remote access plugins)",
    "accessResources.traefik.enable": "Enable watching Traefik IngressRoute resources",
    "accessResources.additionalGvk": "Additional Group-Version-Kind resources to watch for access strategy",
    "extensionApi.enable": "Enable the Extension API server (serves Connection APIs)",
    "extensionApi.jwtIssuer": "JWT issuer claim",
    "extensionApi.jwtAudience": "JWT audience claim",
    "extensionApi.jwtSecret.enable": "Enable K8s-native JWT signing (HMAC keys from a K8s Secret, rotated by CronJob)",
    "extensionApi.jwtSecret.secretName": "Name of the Kubernetes Secret holding signing keys",
    "extensionApi.jwtSecret.tokenTTL": "Token time-to-live",
    "extensionApi.jwtSecret.newKeyUseDelay": "Delay before using a newly rotated key (allows propagation)",
    "extensionApi.jwtSecret.rotationInterval": "How often the CronJob rotates the key",
    "extensionApi.jwtSecret.rotator.repository": "Rotator image repository",
    "extensionApi.jwtSecret.rotator.imageName": "Rotator image name",
    "extensionApi.jwtSecret.rotator.imageTag": "Rotator image tag (defaults to chart appVersion)",
    "extensionApi.jwtSecret.rotator.imagePullPolicy": "Rotator image pull policy",
    "extensionApi.jwtSecret.rotator.resources": "Rotator resource limits and requests",
    "controller.plugins": "Plugin sidecars to deploy alongside the controller. Each plugin runs as a sidecar container in the controller pod.",
}

KEY_RE = re.compile(r"^(\s*)([A-Za-z0-9_]+):")


def is_comment(line):
    return line.lstrip().startswith("#")


def annotate(text):
    lines = text.splitlines()
    out = []
    # Stack of (indent, key) describing the current nesting path.
    stack = []
    for line in lines:
        m = KEY_RE.match(line)
        if m:
            indent = len(m.group(1))
            key = m.group(2)
            while stack and stack[-1][0] >= indent:
                stack.pop()
            stack.append((indent, key))
            path = ".".join(k for _, k in stack)
            comment = ANNOTATIONS.get(path)
            if comment:
                # Replace any contiguous comment block directly above the key
                # (kubebuilder `##` descriptions or patch `#` notes) with the
                # canonical helm-docs annotation. Stops at the first blank or
                # non-comment line, so section banners separated by a blank line
                # and parent-key comments survive.
                while out and is_comment(out[-1]):
                    out.pop()
                out.append(f"{m.group(1)}# -- {comment}")
        out.append(line)
    return "\n".join(out) + "\n"


def main():
    if len(sys.argv) != 2:
        sys.stderr.write("usage: annotate-values.py <values.yaml>\n")
        sys.exit(2)
    path = sys.argv[1]
    with open(path) as f:
        text = f.read()
    with open(path, "w") as f:
        f.write(annotate(text))


if __name__ == "__main__":
    main()
