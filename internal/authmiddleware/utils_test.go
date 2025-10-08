package authmiddleware

import (
	"reflect"
	"testing"
)

func TestSplitGroup(t *testing.T) {
	testCases := []struct {
		groups   string
		expected []string
	}{
		{
			groups:   "github:org:team",
			expected: []string{"github:org:team"},
		},
		{
			groups:   "github:org1:team1, github:org1:team2",
			expected: []string{"github:org1:team1", "github:org1:team2"},
		},
		{
			groups:   "github:org1:team1,github:org1:team2",
			expected: []string{"github:org1:team1", "github:org1:team2"},
		},
		{
			groups:   "github:org1:team1 github:org1:team2",
			expected: []string{"github:org1:team1", "github:org1:team2"},
		},
		{
			groups:   "",
			expected: []string{},
		},
		{
			groups:   "with-inside\"quote,no-inside-quote",
			expected: []string{"with-inside\"quote", "no-inside-quote"},
		},
		{
			groups:   "\"with,inside,comma\",no-inside-comma",
			expected: []string{"with,inside,comma", "no-inside-comma"},
		},
	}

	for _, tc := range testCases {
		result := splitGroups(tc.groups)
		if !reflect.DeepEqual(result, tc.expected) {
			t.Errorf("Expected %q to equal %q", result, tc.expected)
		}
	}
}

func TestJoinGroups(t *testing.T) {
	testCases := []struct {
		groups   []string
		expected string
	}{
		{
			groups:   []string{"github:org:team"},
			expected: "github:org:team",
		},
		{
			groups:   []string{"github:org1:team1", "github:org1:team2"},
			expected: "github:org1:team1,github:org1:team2",
		},
		{
			groups:   []string{},
			expected: "",
		},
		{
			groups:   []string{"with-inside\"quote", "no-inside-quote"},
			expected: "with-inside\"quote,no-inside-quote",
		},
		{
			groups:   []string{"with,inside,comma", "no-inside-comma"},
			expected: "\"with,inside,comma\",no-inside-comma",
		},
	}

	for _, tc := range testCases {
		result := JoinGroups(tc.groups)
		if result != tc.expected {
			t.Errorf("Expected %q to equal %q", result, tc.expected)
		}
	}
}

func TestSplitString(t *testing.T) {
	testCases := []struct {
		input    string
		sep      string
		expected []string
	}{
		{
			input:    "a,b,c",
			sep:      ",",
			expected: []string{"a", "b", "c"},
		},
		{
			input:    "a:b:c d",
			sep:      ":",
			expected: []string{"a", "b", "c d"},
		},
		{
			input:    "a b:c",
			sep:      " ",
			expected: []string{"a", "b:c"},
		},
		{
			input:    "abc",
			sep:      ",",
			expected: []string{"abc"},
		},
		{
			input:    "",
			sep:      ",",
			expected: []string{""},
		},
		{
			input:    "a,,c",
			sep:      ",",
			expected: []string{"a", "", "c"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input+"-"+tc.sep, func(t *testing.T) {
			result := splitString(tc.input, tc.sep)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("splitString(%q, %q) = %v, want %v", tc.input, tc.sep, result, tc.expected)
			}
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	testCases := []struct {
		input    string
		sep      string
		expected []string
	}{
		{
			input:    "a,b,c",
			sep:      ",",
			expected: []string{"a", "b", "c"},
		},
		{
			input:    "a:b:c",
			sep:      ":",
			expected: []string{"a", "b", "c"},
		},
		{
			input:    "abc",
			sep:      ",",
			expected: []string{"abc"},
		},
		{
			input:    "",
			sep:      ",",
			expected: []string{},
		},
		{
			input:    "a,,c",
			sep:      ",",
			expected: []string{"a", "c"},
		},
		{
			input:    " a , b , c ",
			sep:      ",",
			expected: []string{" a ", " b ", " c "},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input+"-"+tc.sep, func(t *testing.T) {
			result := splitAndTrim(tc.input, tc.sep)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("splitAndTrim(%q, %q) = %v, want %v", tc.input, tc.sep, result, tc.expected)
			}
		})
	}
}

func TestHasProtocol(t *testing.T) {
	testCases := []struct {
		url      string
		expected bool
	}{
		{
			url:      "http://example.com",
			expected: true,
		},
		{
			url:      "https://example.com",
			expected: true,
		},
		{
			url:      "http:/example.com", // Missing a slash
			expected: false,
		},
		{
			url:      "https:/example.com", // Missing a slash
			expected: false,
		},
		{
			url:      "example.com",
			expected: false,
		},
		{
			url:      "/path/to/resource",
			expected: false,
		},
		{
			url:      "",
			expected: false,
		},
		{
			url:      "ftp://example.com", // Different protocol
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			result := hasProtocol(tc.url)
			if result != tc.expected {
				t.Errorf("hasProtocol(%q) = %v, want %v", tc.url, result, tc.expected)
			}
		})
	}
}

func TestIsValidRedirectURL(t *testing.T) {
	testCases := []struct {
		url      string
		host     string
		expected bool
	}{
		{
			url:      "/path/to/resource",
			host:     "example.com",
			expected: true,
		},
		{
			url:      "https://example.com/path",
			host:     "example.com",
			expected: true,
		},
		{
			url:      "http://example.com/path",
			host:     "example.com",
			expected: true,
		},
		{
			url:      "https://example.com:8443/path",
			host:     "example.com",
			expected: true,
		},
		{
			url:      "https://evil.com/path",
			host:     "example.com",
			expected: false,
		},
		{
			url:      "example.com/path",
			host:     "example.com",
			expected: false, // Not a valid URL without protocol or leading slash
		},
		{
			url:      "",
			host:     "example.com",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.url+"-"+tc.host, func(t *testing.T) {
			result := isValidRedirectURL(tc.url, tc.host)
			if result != tc.expected {
				t.Errorf("isValidRedirectURL(%q, %q) = %v, want %v", tc.url, tc.host, result, tc.expected)
			}
		})
	}
}

func TestHasHost(t *testing.T) {
	testCases := []struct {
		url      string
		host     string
		expected bool
	}{
		{
			url:      "example.com",
			host:     "example.com",
			expected: true,
		},
		{
			url:      "https://example.com",
			host:     "example.com",
			expected: true,
		},
		{
			url:      "http://example.com",
			host:     "example.com",
			expected: true,
		},
		{
			url:      "https://example.com/path",
			host:     "example.com",
			expected: true,
		},
		{
			url:      "https://subdomain.example.com",
			host:     "example.com",
			expected: false,
		},
		{
			url:      "evil.com",
			host:     "example.com",
			expected: false,
		},
		{
			url:      "https://evil.com",
			host:     "example.com",
			expected: false,
		},
		{
			url:      "",
			host:     "example.com",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.url+"-"+tc.host, func(t *testing.T) {
			result := hasHost(tc.url, tc.host)
			if result != tc.expected {
				t.Errorf("hasHost(%q, %q) = %v, want %v", tc.url, tc.host, result, tc.expected)
			}
		})
	}
}
