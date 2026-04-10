/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"fmt"
	"strings"

	"github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// extensionapiFuncPrefix is the prefix for dynamic context values resolved by the extension API.
	extensionapiFuncPrefix = "extensionapi::"
	// funcPodUID is the function name for resolving the pod UID.
	funcPodUID = "PodUid()"
)

// ResolveConnectionContext resolves dynamic values in the connection context map.
// Values prefixed with "extensionapi::" are replaced with computed values.
// Currently supported: "extensionapi::PodUid()" → pod UID for the workspace.
func ResolveConnectionContext(contextMap map[string]string, k8sClient client.Client, workspaceName string) (map[string]string, error) {
	if len(contextMap) == 0 {
		return contextMap, nil
	}

	resolved := make(map[string]string, len(contextMap))
	for k, v := range contextMap {
		if strings.HasPrefix(v, extensionapiFuncPrefix) {
			funcName := strings.TrimPrefix(v, extensionapiFuncPrefix)
			val, err := resolveExtensionapiFunc(funcName, k8sClient, workspaceName)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve %q for key %q: %w", v, k, err)
			}
			resolved[k] = val
		} else {
			resolved[k] = v
		}
	}
	return resolved, nil
}

func resolveExtensionapiFunc(funcName string, k8sClient client.Client, workspaceName string) (string, error) {
	switch funcName {
	case funcPodUID:
		return workspace.GetPodUIDFromWorkspaceName(k8sClient, workspaceName)
	default:
		return "", fmt.Errorf("unknown extensionapi function: %q", funcName)
	}
}
