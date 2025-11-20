/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

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
		if group == SystemAuthenticatedGroup {
			result[i] = group
		} else {
			result[i] = oidcPrefix + group
		}
	}
	return result
}
