#!/bin/bash
set -e

# Script to apply custom patches to helm chart files after generation
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

# Copy apiservice resources (kubebuilder helm plugin doesn't handle these)
if [ -d "${SCRIPT_DIR}/../config/apiservice" ]; then
    echo "Copying apiservice resources..."
    mkdir -p "${CHART_DIR}/templates/apiservice"
    cp "${SCRIPT_DIR}/../config/apiservice"/*.yaml "${CHART_DIR}/templates/apiservice/"
    # Remove kustomization.yaml as it's not a Kubernetes resource
    rm -f "${CHART_DIR}/templates/apiservice/kustomization.yaml"
fi

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

    # Check if the application section already exists
    if grep -q "^# \[APPLICATION\]" "${CHART_DIR}/values.yaml"; then
        # Remove existing application section - from [APPLICATION] heading to the next section heading or end of file
        sed -i '/^# \[APPLICATION\]/,/^# \[/{ /^# \[APPLICATION\]/d; /^# \[/!d; }' "${CHART_DIR}/values.yaml"
    fi

    # Append the entire application configuration from the patch file
    cat "${PATCHES_DIR}/values.yaml.patch" >> "${CHART_DIR}/values.yaml"
fi


# Handle manager.yaml patch to add registry arguments
if [ -f "${PATCHES_DIR}/manager.yaml.patch" ]; then
    MANAGER_YAML="${CHART_DIR}/templates/manager/manager.yaml"
    if [ -f "${MANAGER_YAML}" ]; then
        echo "Applying patch to manager.yaml..."
        # Find the line with args: followed by the line with range
        if grep -q "args:" "${MANAGER_YAML}" && grep -q "{{- range .Values.controllerManager.container.args }}" "${MANAGER_YAML}"; then
            # Check if the patch is already applied
            if ! grep -q "application-image-registry" "${MANAGER_YAML}"; then
                # Replace the whole args block with our patched version
                sed -i '/args:/,/command:/ {
                    /command:/!d
                    i\          args:\n            {{- range .Values.controllerManager.container.args }}\n            - {{ . }}\n            {{- end }}\n            - "--application-images-pull-policy={{ .Values.application.imagesPullPolicy }}"\n            - "--application-images-registry={{ .Values.application.imagesRegistry }}"
                }' "${MANAGER_YAML}"
            fi
        else
            echo "Warning: Could not find proper args section in manager.yaml"
        fi
    else
        echo "Warning: manager.yaml not found at ${MANAGER_YAML}"
    fi
fi

# Process any additional patch files
for patch_file in "${PATCHES_DIR}"/*.patch; do
    if [ -f "$patch_file" ] && [ "$(basename "$patch_file")" != "values.yaml.patch" ] && [ "$(basename "$patch_file")" != "manager.yaml.patch" ]; then
        file_name=$(basename "$patch_file" .patch)
        apply_patch "$file_name" "$(basename "$patch_file")"
    fi
done

echo "Helm chart patches applied successfully"