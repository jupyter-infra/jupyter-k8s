/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"reflect"
	"testing"
)

func TestGetOidcUsername(t *testing.T) {
	testCases := []struct {
		name              string
		config            *Config
		preferredUsername string
		expected          string
	}{
		{
			name: "add prefix to username",
			config: &Config{
				OidcUsernamePrefix: DefaultOidcUsernamePrefix,
			},
			preferredUsername: testJohndoe,
			expected:          "github:johndoe",
		},
		{
			name: "custom prefix",
			config: &Config{
				OidcUsernamePrefix: "gitlab-",
			},
			preferredUsername: "janedoe",
			expected:          "gitlab-janedoe",
		},
		{
			name: "empty username",
			config: &Config{
				OidcUsernamePrefix: DefaultOidcUsernamePrefix,
			},
			preferredUsername: "",
			expected:          "",
		},
		{
			name: "empty prefix",
			config: &Config{
				OidcUsernamePrefix: "",
			},
			preferredUsername: testJohndoe,
			expected:          testJohndoe,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetOidcUsername(tc.config, tc.preferredUsername)
			if result != tc.expected {
				t.Errorf("Expected %q to equal %q", result, tc.expected)
			}
		})
	}
}

func TestGetOidcGroups(t *testing.T) {
	testCases := []struct {
		name     string
		config   *Config
		groups   []string
		expected []string
	}{
		{
			name: "add prefix to groups",
			config: &Config{
				OidcGroupsPrefix: DefaultOidcUsernamePrefix,
			},
			groups:   []string{testDevValue, testAdminValue},
			expected: []string{"github:dev", "github:admin"},
		},
		{
			name: "custom prefix",
			config: &Config{
				OidcGroupsPrefix: "gitlab-",
			},
			groups:   []string{testUsersValue, "team1", "team2"},
			expected: []string{"gitlab-users", "gitlab-team1", "gitlab-team2"},
		},
		{
			name: "empty groups list",
			config: &Config{
				OidcGroupsPrefix: DefaultOidcUsernamePrefix,
			},
			groups:   []string{},
			expected: []string{},
		},
		{
			name: "empty prefix",
			config: &Config{
				OidcGroupsPrefix: "",
			},
			groups:   []string{testDevValue, testAdminValue},
			expected: []string{testDevValue, testAdminValue},
		},
		{
			name: "already prefixed groups",
			config: &Config{
				OidcGroupsPrefix: DefaultOidcUsernamePrefix,
			},
			groups:   []string{"github:admin", "team1"},
			expected: []string{"github:github:admin", "github:team1"},
		},
		{
			name: "system:authenticated group is preserved",
			config: &Config{
				OidcGroupsPrefix: DefaultOidcUsernamePrefix,
			},
			groups:   []string{SystemAuthenticatedGroup, "dev-team"},
			expected: []string{SystemAuthenticatedGroup, "github:dev-team"},
		},
		{
			name: "mixed system and regular groups",
			config: &Config{
				OidcGroupsPrefix: testOidcPrefix,
			},
			groups:   []string{testAdminValue, SystemAuthenticatedGroup, testUsersValue},
			expected: []string{"oidc:admin", SystemAuthenticatedGroup, "oidc:users"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetOidcGroups(tc.config, tc.groups)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %q to equal %q", result, tc.expected)
			}
		})
	}
}
