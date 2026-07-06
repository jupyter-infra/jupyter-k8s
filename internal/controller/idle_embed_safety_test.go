/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// TestEmbedSafety_NoCollisionWithUpstreamHTTPGetAction is a CI guardrail that
// protects against silent field shadowing when we upgrade k8s.io/api.
//
// IdleHTTPGetAction embeds corev1.HTTPGetAction via `json:",inline"` and adds
// sibling fields ("transport", "lastActivityTimestamp"). If a future k8s release
// adds a field to HTTPGetAction with the same JSON tag as one of ours, Go's
// depth rule silently picks the outer (our) field — the upstream field becomes
// invisible without any compile error. This test fails loudly in that scenario
// so we can rename our field before shipping a broken CRD.
func TestEmbedSafety_NoCollisionWithUpstreamHTTPGetAction(t *testing.T) {
	reservedTags := map[string]bool{
		"transport":             true,
		"lastActivityTimestamp": true,
	}

	rt := reflect.TypeOf(corev1.HTTPGetAction{})
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		// Extract the field name portion before any comma options
		name := jsonTag
		if idx := len(name); idx > 0 {
			for j, c := range jsonTag {
				if c == ',' {
					name = jsonTag[:j]
					break
				}
			}
		}
		if reservedTags[name] {
			t.Errorf("corev1.HTTPGetAction field %q has JSON tag %q which collides with IdleHTTPGetAction field — "+
				"this would silently shadow our field on k8s.io/api upgrade", field.Name, name)
		}
	}
}
