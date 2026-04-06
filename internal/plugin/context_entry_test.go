/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveStr_Found(t *testing.T) {
	e := ContextEntry{Key: "region", Default: "us-west-2"}
	ctx := map[string]string{"region": "eu-west-1"}
	assert.Equal(t, "eu-west-1", e.ResolveStr(ctx))
}

func TestResolveStr_Missing_UsesDefault(t *testing.T) {
	e := ContextEntry{Key: "region", Default: "us-west-2"}
	ctx := map[string]string{}
	assert.Equal(t, "us-west-2", e.ResolveStr(ctx))
}

func TestResolveStr_NoDefault(t *testing.T) {
	e := ContextEntry{Key: "region"}
	ctx := map[string]string{}
	assert.Equal(t, "", e.ResolveStr(ctx))
}

func TestResolveInt32_Found(t *testing.T) {
	e := ContextEntry{Key: "port", Default: "8080"}
	ctx := map[string]string{"port": "9090"}
	assert.Equal(t, 9090, e.ResolveInt32(ctx))
}

func TestResolveInt32_Missing_UsesDefault(t *testing.T) {
	e := ContextEntry{Key: "port", Default: "8080"}
	ctx := map[string]string{}
	assert.Equal(t, 8080, e.ResolveInt32(ctx))
}

func TestResolveInt32_InvalidValue_FallsBackToDefault(t *testing.T) {
	e := ContextEntry{Key: "port", Default: "8080"}
	ctx := map[string]string{"port": "not-a-number"}
	assert.Equal(t, 8080, e.ResolveInt32(ctx))
}

func TestResolveInt32_InvalidValueAndDefault(t *testing.T) {
	e := ContextEntry{Key: "port", Default: "bad"}
	ctx := map[string]string{"port": "also-bad"}
	assert.Equal(t, 0, e.ResolveInt32(ctx))
}

func TestParseHandlerRef_WithAction(t *testing.T) {
	plugin, action := ParseHandlerRef("aws:ssm-remote-access")
	assert.Equal(t, "aws", plugin)
	assert.Equal(t, "ssm-remote-access", action)
}

func TestParseHandlerRef_WithoutAction(t *testing.T) {
	plugin, action := ParseHandlerRef("aws")
	assert.Equal(t, "aws", plugin)
	assert.Equal(t, "", action)
}

func TestParseHandlerRef_K8sNative(t *testing.T) {
	plugin, action := ParseHandlerRef("k8s-native")
	assert.Equal(t, "k8s-native", plugin)
	assert.Equal(t, "", action)
}

func TestParseHandlerRef_MultipleColons(t *testing.T) {
	plugin, action := ParseHandlerRef("aws:create:session")
	assert.Equal(t, "aws", plugin)
	assert.Equal(t, "create:session", action)
}
