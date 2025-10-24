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
