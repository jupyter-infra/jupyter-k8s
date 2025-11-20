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
				OidcUsernamePrefix: "github:",
			},
			preferredUsername: "johndoe",
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
				OidcUsernamePrefix: "github:",
			},
			preferredUsername: "",
			expected:          "",
		},
		{
			name: "empty prefix",
			config: &Config{
				OidcUsernamePrefix: "",
			},
			preferredUsername: "johndoe",
			expected:          "johndoe",
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
				OidcGroupsPrefix: "github:",
			},
			groups:   []string{"dev", "admin"},
			expected: []string{"github:dev", "github:admin"},
		},
		{
			name: "custom prefix",
			config: &Config{
				OidcGroupsPrefix: "gitlab-",
			},
			groups:   []string{"users", "team1", "team2"},
			expected: []string{"gitlab-users", "gitlab-team1", "gitlab-team2"},
		},
		{
			name: "empty groups list",
			config: &Config{
				OidcGroupsPrefix: "github:",
			},
			groups:   []string{},
			expected: []string{},
		},
		{
			name: "empty prefix",
			config: &Config{
				OidcGroupsPrefix: "",
			},
			groups:   []string{"dev", "admin"},
			expected: []string{"dev", "admin"},
		},
		{
			name: "already prefixed groups",
			config: &Config{
				OidcGroupsPrefix: "github:",
			},
			groups:   []string{"github:admin", "team1"},
			expected: []string{"github:github:admin", "github:team1"},
		},
		{
			name: "system:authenticated group is preserved",
			config: &Config{
				OidcGroupsPrefix: "github:",
			},
			groups:   []string{"system:authenticated", "dev-team"},
			expected: []string{"system:authenticated", "github:dev-team"},
		},
		{
			name: "mixed system and regular groups",
			config: &Config{
				OidcGroupsPrefix: "oidc:",
			},
			groups:   []string{"admin", "system:authenticated", "users"},
			expected: []string{"oidc:admin", "system:authenticated", "oidc:users"},
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
