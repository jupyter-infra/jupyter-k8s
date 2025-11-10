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

# Copy apiservice resources (kubebuilder helm plugin doesn't handle these)
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
                    sed -i '/volumeMounts:/a\            {{- if and .Values.extensionApi.enable .Values.certmanager.enable }}\n            - name: extension-server-cert\n              mountPath: /tmp/extension-server/serving-certs\n              readOnly: true\n            {{- end }}' "${MANAGER_YAML}"

                    # Add volume for extension API server certificates
                    sed -i '/volumes:/a\        {{- if and .Values.extensionApi.enable .Values.certmanager.enable }}\n        - name: extension-server-cert\n          secret:\n            secretName: extension-server-cert\n        {{- end }}' "${MANAGER_YAML}"

                    # Update the conditional wrapping for volumeMounts and volumes
                    sed -i 's/{{- if and .Values.certmanager.enable (or .Values.webhook.enable .Values.metrics.enable) }}/{{- if and .Values.certmanager.enable (or .Values.webhook.enable .Values.metrics.enable .Values.extensionApi.enable) }}/g' "${MANAGER_YAML}"
                fi

                # Clean up temp file
                rm -f "${TMP_PATCH}"
            else
                echo "Args section already has application-images-registry, skipping patch"
            fi
        else
            echo "Warning: Could not find proper args section in manager.yaml"
        fi
    else
        echo "Warning: manager.yaml not found at ${MANAGER_YAML}"
    fi
fi

# Handle manager scheduling patch
if [ -f "${PATCHES_DIR}/manager-scheduling.yaml.patch" ]; then
    MANAGER_YAML="${CHART_DIR}/templates/manager/manager.yaml"
    if [ -f "${MANAGER_YAML}" ]; then
        echo "Applying scheduling patch to manager.yaml..."
        
        # Check if scheduling patch is already applied
        if ! grep -q "{{- with .Values.controller.nodeSelector }}" "${MANAGER_YAML}"; then
            echo "Adding scheduling configuration to manager.yaml"
            
            # Replace the scheduling section between serviceAccountName and securityContext
            if [[ "$OSTYPE" == "darwin"* ]]; then
                # macOS sed - replace the section after serviceAccountName and before securityContext
                sed -i '' '/serviceAccountName:/a\
      {{- with .Values.controller.nodeSelector }}\
      nodeSelector:\
        {{- toYaml . | nindent 8 }}\
      {{- end }}\
      {{- with .Values.controller.tolerations }}\
      tolerations:\
        {{- toYaml . | nindent 8 }}\
      {{- end }}\
      {{- with .Values.controller.affinity }}\
      affinity:\
        {{- toYaml . | nindent 8 }}\
      {{- else }}\
      affinity:\
        nodeAffinity:\
          requiredDuringSchedulingIgnoredDuringExecution:\
            nodeSelectorTerms:\
              - matchExpressions:\
                - key: kubernetes.io/arch\
                  operator: In\
                  values:\
                    - amd64\
                    - arm64\
                    - ppc64le\
                    - s390x\
                - key: kubernetes.io/os\
                  operator: In\
                  values:\
                    - linux\
      {{- end }}' "${MANAGER_YAML}"
            else
                # Linux sed - replace the section after serviceAccountName and before securityContext
                sed -i '/serviceAccountName:/a\      {{- with .Values.controller.nodeSelector }}\n      nodeSelector:\n        {{- toYaml . | nindent 8 }}\n      {{- end }}\n      {{- with .Values.controller.tolerations }}\n      tolerations:\n        {{- toYaml . | nindent 8 }}\n      {{- end }}\n      {{- with .Values.controller.affinity }}\n      affinity:\n        {{- toYaml . | nindent 8 }}\n      {{- else }}\n      affinity:\n        nodeAffinity:\n          requiredDuringSchedulingIgnoredDuringExecution:\n            nodeSelectorTerms:\n              - matchExpressions:\n                - key: kubernetes.io/arch\n                  operator: In\n                  values:\n                    - amd64\n                    - arm64\n                    - ppc64le\n                    - s390x\n                - key: kubernetes.io/os\n                  operator: In\n                  values:\n                    - linux\n      {{- end }}' "${MANAGER_YAML}"
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
    sed -i 's/name: selfsigned-issuer/name: jupyter-k8s-selfsigned-issuer/g' "${CERT_YAML}"
    echo "Updated issuer name to jupyter-k8s-selfsigned-issuer"
fi

# Process any additional patch files
for patch_file in "${PATCHES_DIR}"/*.patch; do
    if [ -f "$patch_file" ] && [ "$(basename "$patch_file")" != "values.yaml.patch" ] && [ "$(basename "$patch_file")" != "manager.yaml.patch" ]; then
        file_name=$(basename "$patch_file" .patch)
        apply_patch "$file_name" "$(basename "$patch_file")"
    fi
done

echo "Helm chart patches applied successfully"