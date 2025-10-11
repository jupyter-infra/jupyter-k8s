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

    # Only add APPLICATION section if it doesn't exist
    if ! grep -q "^# \[APPLICATION\]" "${CHART_DIR}/values.yaml"; then
        # Append the entire application configuration from the patch file
        cat "${PATCHES_DIR}/values.yaml.patch" >> "${CHART_DIR}/values.yaml"
    fi
fi


# Handle manager.yaml patch to add registry arguments
if [ -f "${PATCHES_DIR}/manager.yaml.patch" ]; then
    MANAGER_YAML="${CHART_DIR}/templates/manager/manager.yaml"
    if [ -f "${MANAGER_YAML}" ]; then
        echo "Applying patch to manager.yaml..."
        # Check if the patch is already applied
        if ! grep -q "application-image-registry" "${MANAGER_YAML}"; then
            # Create a temporary file with the replacement
            TEMP_FILE=$(mktemp)
            # Replace the args block with the patched version
            awk '
                /^          args:/ { 
                    print "          args:"
                    print "            {{- range .Values.controllerManager.container.args }}"
                    print "            - {{ . }}"
                    print "            {{- end }}"
                    print "            - --application-images-pull-policy={{ .Values.application.imagesPullPolicy }}"
                    print "            - --application-images-registry={{ .Values.application.imagesRegistry }}"
                    # Skip the original args block
                    while (getline > 0 && !/^          command:/) continue
                    print
                    next
                }
                { print }
            ' "${MANAGER_YAML}" > "${TEMP_FILE}"
            mv "${TEMP_FILE}" "${MANAGER_YAML}"
        fi
    else
        echo "Warning: manager.yaml not found at ${MANAGER_YAML}"
    fi
fi

# Handle VAP files with full replacement
echo "Creating VAP templates..."
mkdir -p "${CHART_DIR}/templates/vap"
for vap_file in "immutable-creator-username-annotation.yaml" "private-workspace-vap.yaml" "private-workspace-vap-binding.yaml"; do
    if [ -f "${PATCHES_DIR}/${vap_file}.patch" ]; then
        VAP_TARGET="${CHART_DIR}/templates/vap/${vap_file}"
        echo "Creating ${vap_file} from patch..."
        cp "${PATCHES_DIR}/${vap_file}.patch" "${VAP_TARGET}"
    fi
done

# Process any additional patch files
for patch_file in "${PATCHES_DIR}"/*.patch; do
    if [ -f "$patch_file" ] && [ "$(basename "$patch_file")" != "values.yaml.patch" ] && [ "$(basename "$patch_file")" != "manager.yaml.patch" ] && [[ "$(basename "$patch_file")" != *"vap"* ]]; then
        file_name=$(basename "$patch_file" .patch)
        apply_patch "$file_name" "$(basename "$patch_file")"
    fi
done

echo "Helm chart patches applied successfully"