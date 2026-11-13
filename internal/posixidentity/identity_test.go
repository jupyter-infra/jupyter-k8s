/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"testing"
)

func TestUnixName(t *testing.T) {
	cases := map[string]string{
		"github:JaneDoe":       "github-janedoe",
		"jane.doe@example.com": "jane-doe-example-com",
		"":                     "user",
		"a-very-long-username-that-exceeds-32-chars": "a-very-long-username-that-exceed",
		"élan": "-lan",
	}
	for in, want := range cases {
		if got := UnixName(in); got != want {
			t.Errorf("UnixName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUsernameHashIsLabelSafe(t *testing.T) {
	h := UsernameHash(alice)
	if len(h) != 32 || h != UsernameHash(alice) || h == UsernameHash(bob) {
		t.Fatalf("unexpected hash %q", h)
	}
}

func TestV1CandidatesAreDeterministicAndInRange(t *testing.T) {
	a, b := v1Candidates(alice), v1Candidates(alice)
	if len(a) != maxProbes {
		t.Fatalf("got %d candidates", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("candidate %d differs", i)
		}
		if !InUserRange(a[i]) {
			t.Fatalf("candidate %d out of range: %d", i, a[i])
		}
	}
}

func TestIDRanges(t *testing.T) {
	for id, want := range map[int64]bool{0: false, 999: false, 1000: true, 65534: false, UserRangeMin: false, UserRangeMax: false, GroupRangeMin: false, GroupRangeMax: false, GroupRangeMax + 1: true, MaxID: true, MaxID + 1: false} {
		if got := ValidGroupID(id); got != want {
			t.Errorf("ValidGroupID(%d) = %v, want %v", id, got, want)
		}
	}
}
