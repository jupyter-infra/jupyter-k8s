package authmiddleware

import "fmt"

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
		result[i] = oidcPrefix + group
	}
	return result
}
