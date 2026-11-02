/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"fmt"
	"strings"
)

// GetOidcUsername returns the k8s username with OIDC prefix applied
func GetOidcUsername(serverConfig *Config, preferredUsername string) string {
	oidcPrefix := serverConfig.OidcUsernamePrefix

	if preferredUsername != "" {
		return fmt.Sprintf("%s%s", oidcPrefix, preferredUsername)
	}
	return ""
}

// GetOidcGroups return the k8s groups from the groups with OIDC prefix applied
func GetOidcGroups(serverConfig *Config, groups []string) []string {
	oidcPrefix := serverConfig.OidcGroupsPrefix

	if len(groups) == 0 {
		return []string{}
	}

	result := make([]string, len(groups))
	for i, group := range groups {
		if group == SystemAuthenticatedGroup {
			result[i] = group
		} else {
			result[i] = oidcPrefix + group
		}
	}
	return result
}

// ValidateOIDCIdentities rejects post-prefix usernames and groups that start with system:,
// preventing any prefix+token combination from constructing a reserved Kubernetes identity.
// system:authenticated is always permitted.
func ValidateOIDCIdentities(username string, groups []string) error {
	if strings.HasPrefix(username, "system:") {
		return fmt.Errorf("username %q is not allowed: system: identities are reserved", username)
	}
	for _, group := range groups {
		if group != SystemAuthenticatedGroup && strings.HasPrefix(group, "system:") {
			return fmt.Errorf("group %q is not allowed: system: identities are reserved", group)
		}
	}
	return nil
}
