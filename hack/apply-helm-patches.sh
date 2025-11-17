#!/bin/bash
set -e

# Script to apply custom patches to helm chart files after generation
# Contains patches for:
# 1. extension_issuer - Adds jupyter-k8s-selfsigned-issuer for extension API
# Run this after 'kubebuilder edit --plugins=helm/v1-alpha --force'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="$(cd "${SCRIPT_DIR}/../dist/chart" && pwd)"
PATCHES_DIR="${SCRIPT_DIR}/helm-patches"

# Check if chart directory exists
if [ ! -d "${CHART_DIR}" ]; then
    echo "Error: Chart directory not found at ${CHART_DIR}"
    echo "Run 'kubebuilder edit --plugins=helm/v1-alpha' first"
    exit 1
fi

echo "Applying custom patches to Helm chart files..."
echo "Using patches from ${PATCHES_DIR}"

# Copy manager resources (including PodDisruptionBudget)
if [ -d "${SCRIPT_DIR}/../config/manager" ]; then
    echo "Copying additional manager resources..."
    
    # Copy PodDisruptionBudget if it exists
    if [ -f "${SCRIPT_DIR}/../config/manager/poddisruptionbudget.yaml" ]; then
        echo "Copying PodDisruptionBudget template..."
        cp "${SCRIPT_DIR}/../config/manager/poddisruptionbudget.yaml" "${CHART_DIR}/templates/manager/"
    fi
fi
if [ -d "${SCRIPT_DIR}/../config/apiservice" ]; then
    echo "Copying apiservice resources..."
    mkdir -p "${CHART_DIR}/templates/apiservice"

    # Copy each YAML file and wrap with conditional
    # This adds {{- if .Values.extensionApi.enable }} around each file
    # so the API service resources are only created when extension API is enabled
    for file in "${SCRIPT_DIR}/../config/apiservice"/*.yaml; do
        if [ -f "$file" ]; then
            filename=$(basename "$file")
            target="${CHART_DIR}/templates/apiservice/${filename}"

            # Add conditional wrapper
            echo "{{- if .Values.extensionApi.enable }}" > "$target"

            # Apply specific modifications based on file name
            if [[ "$filename" == "apiservice.yaml" ]]; then
                # Replace hardcoded namespace with {{ .Release.Namespace }}
                # Also replace hardcoded cert reference with {{ .Release.Namespace }}
                cat "$file" | \
                    sed 's/cert-manager.io\/inject-ca-from: jupyter-k8s-system\/extension-server-cert/cert-manager.io\/inject-ca-from: {{ .Release.Namespace }}\/extension-server-cert/' | \
                    sed 's/namespace: jupyter-k8s-system/namespace: {{ .Release.Namespace }}/' >> "$target"
            elif [[ "$filename" == "certificate.yaml" ]]; then
                # Replace hardcoded namespace with {{ .Release.Namespace }}
                # Also replace hardcoded DNS names with templated values
                cat "$file" | \
                    sed 's/namespace: jupyter-k8s-system/namespace: {{ .Release.Namespace }}/' | \
                    sed 's/- extension-server.jupyter-k8s-system.svc/- extension-server.{{ .Release.Namespace }}.svc/' | \
                    sed 's/- extension-server.jupyter-k8s-system.svc.cluster.local/- extension-server.{{ .Release.Namespace }}.svc.cluster.local/' >> "$target"
            elif [[ "$filename" == "service.yaml" ]]; then
                # Replace hardcoded namespace with {{ .Release.Namespace }}
                cat "$file" | \
                    sed 's/namespace: jupyter-k8s-system/namespace: {{ .Release.Namespace }}/' >> "$target"
            else
                cat "$file" >> "$target"
            fi

            echo "{{- end }}" >> "$target"

            echo "  Added conditional to ${filename}"
        fi
    done

    # Remove kustomization.yaml as it's not a Kubernetes resource
    rm -f "${CHART_DIR}/templates/apiservice/kustomization.yaml"
fi

# Remove connection CRDs as they're meant to be subresources, not standalone CRDs
echo "Removing connection CRDs from Helm chart..."
find "${CHART_DIR}/templates/crd/" -name "connection.workspace.jupyter.org_*.yaml" -delete

# Function to apply patches
apply_patch() {
    local file=$1
    local patch=$2
    local target="${CHART_DIR}/${file}"

    if [ -f "${target}" ]; then
        echo "Patching ${file}..."
        # For simple patches, we'll use grep and sed
        # For more complex patches, consider using 'patch' command

        # Read the patch file line by line
        while IFS= read -r line; do
            # Extract the match pattern (everything before the first occurrence of ' = ')
            # and the replacement text
            if [[ "$line" == *"="* ]]; then
                pattern=$(echo "$line" | sed -E 's/^([^=]+)=.*/\1/')
                # Check if the pattern exists in the file
                if grep -q "$pattern" "$target"; then
                    # Replace existing line
                    sed -i "s#$pattern.*#$line#" "$target"
                else
                    # Pattern not found, add new line
                    echo "$line" >> "$target"
                fi
            else
                # For lines without '=', just look for the line and add if not present
                if ! grep -q "$line" "$target"; then
                    echo "$line" >> "$target"
                fi
            fi
        done < "${PATCHES_DIR}/${patch}"
    else
        echo "Warning: Target file ${file} not found, skipping patch"
    fi
}

# Process values.yaml patch
if [ -f "${PATCHES_DIR}/values.yaml.patch" ]; then
    # For values.yaml, we need more careful handling due to YAML structure
    echo "Applying patches to values.yaml..."
    echo "Found patch file: ${PATCHES_DIR}/values.yaml.patch"

    # Add env section to controllerManager.container if it doesn't exist
    if ! grep -q "env:" "${CHART_DIR}/values.yaml"; then
        # Add env section right after container: (macOS compatible)
        sed -i.bak '/container:/a\
    env:\
      CLUSTER_ADMIN_GROUP: "cluster-workspace-admin"\
      CLUSTER_ID: ""
' "${CHART_DIR}/values.yaml" && rm "${CHART_DIR}/values.yaml.bak"
    fi

    # Check if the application section already exists
    if grep -q "^# \[APPLICATION\]" "${CHART_DIR}/values.yaml"; then
        echo "Removing existing APPLICATION section from values.yaml"
        # Remove existing application section - from [APPLICATION] heading to the next section heading or end of file
        sed -i '/^# \[APPLICATION\]/,/^# \[/{ /^# \[APPLICATION\]/d; /^# \[/!d; }' "${CHART_DIR}/values.yaml"
    fi

    # Check if the access resources section already exists
    if grep -q "^# \[ACCESS RESOURCES\]" "${CHART_DIR}/values.yaml"; then
        echo "Removing existing ACCESS RESOURCES section from values.yaml"
        # Remove existing section - from [ACCESS RESOURCES] heading to the next section heading or end of file
        sed -i '/^# \[ACCESS RESOURCES\]/,/^# \[/{ /^# \[ACCESS RESOURCES\]/d; /^# \[/!d; }' "${CHART_DIR}/values.yaml"
    fi

    # Check if the workspace pod watching section already exists
    if grep -q "^# \[WORKSPACE POD WATCHING\]" "${CHART_DIR}/values.yaml"; then
        echo "Removing existing WORKSPACE POD WATCHING section from values.yaml"
        # Remove existing section - from [WORKSPACE POD WATCHING] heading to the next section heading or end of file
        sed -i '/^# \[WORKSPACE POD WATCHING\]/,/^# \[/{ /^# \[WORKSPACE POD WATCHING\]/d; /^# \[/!d; }' "${CHART_DIR}/values.yaml"
    fi

    # Add scheduling configuration to controllerManager section
    if ! grep -q "nodeSelector:" "${CHART_DIR}/values.yaml"; then
        echo "Adding scheduling configuration to controllerManager section"
        # Add scheduling fields after terminationGracePeriodSeconds
        if [[ "$OSTYPE" == "darwin"* ]]; then
            # macOS sed requires different syntax for append command
            sed -i '' '/terminationGracePeriodSeconds:/a\
  # Controller pod scheduling configuration\
  nodeSelector: {}\
  tolerations: []\
  affinity: {}\
  topologySpreadConstraints: []
' "${CHART_DIR}/values.yaml"
        else
            # Linux sed
            sed -i '/terminationGracePeriodSeconds:/a\  # Controller pod scheduling configuration\n  nodeSelector: {}\n  tolerations: []\n  affinity: {}\n  topologySpreadConstraints: []' "${CHART_DIR}/values.yaml"
        fi
    fi

    # Append the entire patch file content to values.yaml
    echo "Appending patch content to values.yaml"
    cat "${PATCHES_DIR}/values.yaml.patch" >> "${CHART_DIR}/values.yaml"
    echo "Successfully applied values.yaml.patch"
fi


# Handle manager.yaml patch to add registry arguments and watch-traefik flag
if [ -f "${PATCHES_DIR}/manager.yaml.patch" ]; then
    MANAGER_YAML="${CHART_DIR}/templates/manager/manager.yaml"
    if [ -f "${MANAGER_YAML}" ]; then
        echo "Applying patch to manager.yaml..."
        # Find the line with args: followed by the line with range
        if grep -q "args:" "${MANAGER_YAML}" && grep -q "{{- range .Values.controllerManager.container.args }}" "${MANAGER_YAML}"; then
            # Check if the patch is already applied
            if ! grep -q "application-images-registry" "${MANAGER_YAML}"; then
                echo "Replacing args section with content from manager.yaml.patch"

                # Create a temporary file with the patch content
                TMP_PATCH="/tmp/manager_patch_content"
                cat "${PATCHES_DIR}/manager.yaml.patch" > "${TMP_PATCH}"

                # Replace the whole args block with our patched version from the patch file
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    # macOS sed requires empty string after -i
                    sed -i '' '/args:/,/command:/ {
                        /command:/!d
                        i\
          args:\
            {{- range .Values.controllerManager.container.args }}\
            - {{ . }}\
            {{- end }}\
            - "--application-images-pull-policy={{ .Values.application.imagesPullPolicy }}"\
            - "--application-images-registry={{ .Values.application.imagesRegistry }}"\
            {{- if .Values.accessResources.traefik.enable }}\
            - "--watch-traefik"\
            {{- end}}\
            {{- if .Values.extensionApi.enable }}\
            - "--enable-extension-api"\
            {{- end}}\
            {{- if .Values.workspacePodWatching.enable }}\
            - "--enable-workspace-pod-watching"\
            {{- end}}
                    }' "${MANAGER_YAML}"
                else
                    # Linux sed
                    sed -i '/args:/,/command:/ {
                    /command:/!d
                    i\          args:\n            {{- range .Values.controllerManager.container.args }}\n            - {{ . }}\n            {{- end }}\n            - "--application-images-pull-policy={{ .Values.application.imagesPullPolicy }}"\n            - "--application-images-registry={{ .Values.application.imagesRegistry }}"\n            {{- if .Values.accessResources.traefik.enable }}\n            - "--watch-traefik"\n            {{- end}}\n            {{- if .Values.extensionApi.enable }}\n            - "--enable-extension-api"\n            {{- end}}\n            {{- if .Values.workspacePodWatching.enable }}\n            - "--enable-workspace-pod-watching"\n            {{- end}}
                }' "${MANAGER_YAML}"
                fi
                # Also add extension API volume mount if not already present
                if ! grep -q "extension-server-cert" "${MANAGER_YAML}"; then
                    # Add volume mount for extension API server certificates
                    if [[ "$OSTYPE" == "darwin"* ]]; then
                        sed -i '' '/volumeMounts:/a\
            {{- if and .Values.extensionApi.enable .Values.certmanager.enable }}\
            - name: extension-server-cert\
              mountPath: /tmp/extension-server/serving-certs\
              readOnly: true\
            {{- end }}
' "${MANAGER_YAML}"
                    else
                        sed -i '/volumeMounts:/a\            {{- if and .Values.extensionApi.enable .Values.certmanager.enable }}\n            - name: extension-server-cert\n              mountPath: /tmp/extension-server/serving-certs\n              readOnly: true\n            {{- end }}' "${MANAGER_YAML}"
                    fi

                    # Add volume for extension API server certificates
                    if [[ "$OSTYPE" == "darwin"* ]]; then
                        sed -i '' '/volumes:/a\
        {{- if and .Values.extensionApi.enable .Values.certmanager.enable }}\
        - name: extension-server-cert\
          secret:\
            secretName: extension-server-cert\
        {{- end }}
' "${MANAGER_YAML}"
                    else
                        sed -i '/volumes:/a\        {{- if and .Values.extensionApi.enable .Values.certmanager.enable }}\n        - name: extension-server-cert\n          secret:\n            secretName: extension-server-cert\n        {{- end }}' "${MANAGER_YAML}"
                    fi

                    # Update the conditional wrapping for volumeMounts and volumes
                    if [[ "$OSTYPE" == "darwin"* ]]; then
                        sed -i '' 's/{{- if and .Values.certmanager.enable (or .Values.webhook.enable .Values.metrics.enable) }}/{{- if and .Values.certmanager.enable (or .Values.webhook.enable .Values.metrics.enable .Values.extensionApi.enable) }}/g' "${MANAGER_YAML}"
                    else
                        sed -i 's/{{- if and .Values.certmanager.enable (or .Values.webhook.enable .Values.metrics.enable) }}/{{- if and .Values.certmanager.enable (or .Values.webhook.enable .Values.metrics.enable .Values.extensionApi.enable) }}/g' "${MANAGER_YAML}"
                    fi
                fi

                # Clean up temp file
                rm -f "${TMP_PATCH}"
            else
                echo "Args section already has application-images-registry, skipping patch"
            fi
        else
            echo "Warning: Could not find proper args section in manager.yaml"
        fi
        
        # Add dynamic environment variables for namespace and service account
        echo "Adding dynamic environment variables to manager.yaml..."
        if ! grep -q "CONTROLLER_POD_NAMESPACE" "${MANAGER_YAML}"; then
            # Find where to insert env vars - after imagePullPolicy or after the existing env section
            if grep -q "{{- if .Values.controllerManager.container.env }}" "${MANAGER_YAML}"; then
                # Env section exists, add our dynamic vars after the closing {{- end }}
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    # macOS sed
                    sed -i '' '/{{- if .Values.controllerManager.container.env }}/,/{{- end }}/ {
                        /{{- end }}/a\
            - name: CONTROLLER_POD_NAMESPACE\
              valueFrom:\
                fieldRef:\
                  fieldPath: metadata.namespace\
            - name: CONTROLLER_POD_SERVICE_ACCOUNT\
              valueFrom:\
                fieldRef:\
                  fieldPath: spec.serviceAccountName
                    }' "${MANAGER_YAML}"
                else
                    # Linux sed
                    sed -i '/{{- if .Values.controllerManager.container.env }}/,/{{- end }}/ {
                        /{{- end }}/a\            - name: CONTROLLER_POD_NAMESPACE\n              valueFrom:\n                fieldRef:\n                  fieldPath: metadata.namespace\n            - name: CONTROLLER_POD_SERVICE_ACCOUNT\n              valueFrom:\n                fieldRef:\n                  fieldPath: spec.serviceAccountName
                    }' "${MANAGER_YAML}"
                fi
            else
                # No env section exists, create one after imagePullPolicy
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    sed -i '' '/imagePullPolicy:/a\
          env:\
          - name: CONTROLLER_POD_NAMESPACE\
            valueFrom:\
              fieldRef:\
                fieldPath: metadata.namespace\
          - name: CONTROLLER_POD_SERVICE_ACCOUNT\
            valueFrom:\
              fieldRef:\
                fieldPath: spec.serviceAccountName
' "${MANAGER_YAML}"
                else
                    sed -i '/imagePullPolicy:/a\          env:\n          - name: CONTROLLER_POD_NAMESPACE\n            valueFrom:\n              fieldRef:\n                fieldPath: metadata.namespace\n          - name: CONTROLLER_POD_SERVICE_ACCOUNT\n            valueFrom:\n              fieldRef:\n                fieldPath: spec.serviceAccountName
' "${MANAGER_YAML}"
                fi
            fi
            echo "Added CONTROLLER_POD_NAMESPACE and CONTROLLER_POD_SERVICE_ACCOUNT environment variables"
        else
            echo "CONTROLLER_POD_NAMESPACE already exists, skipping env var injection"
        fi
    else
        echo "Warning: manager.yaml not found at ${MANAGER_YAML}"
    fi
fi

# Handle manager annotations patch - add podAnnotations support
if [ -f "${PATCHES_DIR}/manager-annotations.yaml.patch" ]; then
    MANAGER_YAML="${CHART_DIR}/templates/manager/manager.yaml"
    if [ -f "${MANAGER_YAML}" ]; then
        echo "Adding podAnnotations support to manager.yaml..."
        
        # Check if annotations patch is already applied
        if ! grep -q "controllerManager.pod.annotations" "${MANAGER_YAML}"; then
            echo "Adding pod annotations templating"
            
            # Insert patch content after the default-container annotation
            if [[ "$OSTYPE" == "darwin"* ]]; then
                sed -i '' '/kubectl.kubernetes.io\/default-container: manager$/r '"${PATCHES_DIR}/manager-annotations.yaml.patch" "${MANAGER_YAML}"
            else
                sed -i '/kubectl.kubernetes.io\/default-container: manager$/r '"${PATCHES_DIR}/manager-annotations.yaml.patch" "${MANAGER_YAML}"
            fi
            echo "Successfully added podAnnotations support"
        fi
    fi
fi

# Handle manager scheduling patch
if [ -f "${PATCHES_DIR}/manager-scheduling.yaml.patch" ]; then
    MANAGER_YAML="${CHART_DIR}/templates/manager/manager.yaml"
    if [ -f "${MANAGER_YAML}" ]; then
        echo "Applying scheduling patch to manager.yaml..."
        
        # Check if scheduling patch is already applied
        if ! grep -q "{{- with .Values.controllerManager.nodeSelector }}" "${MANAGER_YAML}"; then
            echo "Adding scheduling configuration to manager.yaml"
            
            # Insert the patch content after serviceAccountName line
            if [[ "$OSTYPE" == "darwin"* ]]; then
                # macOS sed - insert patch content after serviceAccountName
                sed -i '' '/serviceAccountName:/r '"${PATCHES_DIR}/manager-scheduling.yaml.patch" "${MANAGER_YAML}"
            else
                # Linux sed - insert patch content after serviceAccountName
                sed -i '/serviceAccountName:/r '"${PATCHES_DIR}/manager-scheduling.yaml.patch" "${MANAGER_YAML}"
            fi
            echo "Successfully applied scheduling patch"
        else
            echo "Scheduling patch already applied, skipping"
        fi
    else
        echo "Warning: manager.yaml not found at ${MANAGER_YAML}"
    fi
fi

# Patch the issuer name and add extension certificate in certmanager/certificate.yaml
echo "Patching certmanager/certificate.yaml..."
CERT_YAML="${CHART_DIR}/templates/certmanager/certificate.yaml"
if [ -f "${CERT_YAML}" ]; then
    # Update all issuer references to use jupyter-k8s-selfsigned-issuer
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' 's/name: selfsigned-issuer/name: jupyter-k8s-selfsigned-issuer/g' "${CERT_YAML}"
    else
        sed -i 's/name: selfsigned-issuer/name: jupyter-k8s-selfsigned-issuer/g' "${CERT_YAML}"
    fi
    echo "Updated issuer name to jupyter-k8s-selfsigned-issuer"
fi

# Handle manager labels patch
if [ -f "${PATCHES_DIR}/manager-labels.yaml.patch" ]; then
    MANAGER_YAML="${CHART_DIR}/templates/manager/manager.yaml"
    if [ -f "${MANAGER_YAML}" ]; then
        echo "Applying labels patch to manager.yaml..."
        
        # Check if labels patch is already applied
        if ! grep -q "workspace.jupyter.org/component: controller" "${MANAGER_YAML}"; then
            echo "Adding custom labels to manager.yaml"
            
            # Add custom label to deployment metadata labels (after the first control-plane: controller-manager)
            sed -i '0,/control-plane: controller-manager$/s//&\n    workspace.jupyter.org\/component: controller/' "${MANAGER_YAML}"
            
            # Add custom label to selector matchLabels (after the selector control-plane: controller-manager)
            sed -i '0,/control-plane: controller-manager$/b; s/control-plane: controller-manager$/&\n      workspace.jupyter.org\/component: controller/' "${MANAGER_YAML}"
            
            # Add custom label to pod template labels (after the pod template control-plane: controller-manager)
            sed -i '0,/control-plane: controller-manager$/b; 0,/control-plane: controller-manager$/b; s/control-plane: controller-manager$/&\n        workspace.jupyter.org\/component: controller/' "${MANAGER_YAML}"
            
            echo "Successfully applied labels patch"
        else
            echo "Labels patch already applied, skipping"
        fi
    else
        echo "Warning: manager.yaml not found at ${MANAGER_YAML}"
    fi
fi

# Process any additional patch files
for patch_file in "${PATCHES_DIR}"/*.patch; do
    if [ -f "$patch_file" ] && [ "$(basename "$patch_file")" != "values.yaml.patch" ] && [ "$(basename "$patch_file")" != "manager.yaml.patch" ] && [ "$(basename "$patch_file")" != "manager-labels.yaml.patch" ]; then
        file_name=$(basename "$patch_file" .patch)
        apply_patch "$file_name" "$(basename "$patch_file")"
    fi
done

echo "Helm chart patches applied successfully"