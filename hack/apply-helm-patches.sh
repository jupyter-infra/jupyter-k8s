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

    # First ensure we have the application section for pull policy configuration
    if ! grep -q "application:" "${CHART_DIR}/values.yaml"; then
        echo -e "\n# [APPLICATION]: Application (JupyterServer) configuration\napplication:\n  # Image pull policy for JupyterServer deployments (Always, IfNotPresent, or Never)\n  # This controls how the JupyterServer containers pull their images\n  imagePullPolicy: IfNotPresent" >> "${CHART_DIR}/values.yaml"
    elif ! grep -q "application.imagePullPolicy:" "${CHART_DIR}/values.yaml"; then
        # Add the pull policy to the application section
        sed -i '/application:/a\  # Image pull policy for JupyterServer deployments (Always, IfNotPresent, or Never)\n  # This controls how the JupyterServer containers pull their images\n  imagePullPolicy: IfNotPresent' "${CHART_DIR}/values.yaml"
    fi

    # Now handle the args section
    if grep -q "args:" "${CHART_DIR}/values.yaml"; then
        # First backup the current args section
        ARGS_SECTION=$(sed -n '/args:/,/resources:/p' "${CHART_DIR}/values.yaml")

        # Check if application-image-pull-policy arg exists
        if echo "$ARGS_SECTION" | grep -q -- "--application-image-pull-policy"; then
            # Update it to use the variable
            sed -i '/--application-image-pull-policy/c\      - "--application-image-pull-policy={{ .Values.application.imagePullPolicy }}"' "${CHART_DIR}/values.yaml"
        else
            # Add the argument before resources section
            sed -i '/resources:/i\      - "--application-image-pull-policy={{ .Values.application.imagePullPolicy }}"' "${CHART_DIR}/values.yaml"
        fi
    else
        echo "Warning: args section not found in values.yaml"
    fi
fi

# Process any additional patch files
for patch_file in "${PATCHES_DIR}"/*.patch; do
    if [ -f "$patch_file" ] && [ "$(basename "$patch_file")" != "values.yaml.patch" ]; then
        file_name=$(basename "$patch_file" .patch)
        apply_patch "$file_name" "$(basename "$patch_file")"
    fi
done

echo "Helm chart patches applied successfully"