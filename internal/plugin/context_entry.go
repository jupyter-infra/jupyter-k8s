/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package plugin

import (
	"strconv"
	"strings"
)

// ContextEntry pairs a lookup key (for podEventsContext or createConnectionContext)
// with a default value used when the key is absent from the map.
type ContextEntry struct {
	Key     string // key to look up in the context map
	Default string // fallback value if not found
}

// ResolveStr returns the value from ctx, falling back to the default.
func (e ContextEntry) ResolveStr(ctx map[string]string) string {
	if v, ok := ctx[e.Key]; ok {
		return v
	}
	return e.Default
}

// ResolveInt32 returns the integer value from ctx, falling back to the default.
func (e ContextEntry) ResolveInt32(ctx map[string]string) int {
	s := e.ResolveStr(ctx)
	v, err := strconv.Atoi(s)
	if err != nil {
		v, _ = strconv.Atoi(e.Default)
	}
	return v
}

// ParseHandlerRef parses a handler reference in "plugin:action" format.
// Returns (pluginName, action). If no action is specified, action is empty.
// Examples:
//
//	"aws:ssm-remote-access" → ("aws", "ssm-remote-access")
//	"aws:createSession"     → ("aws", "createSession")
//	"aws"                   → ("aws", "")
//	"k8s-native"            → ("k8s-native", "")
func ParseHandlerRef(ref string) (pluginName, action string) {
	parts := strings.SplitN(ref, ":", 2)
	pluginName = parts[0]
	if len(parts) == 2 {
		action = parts[1]
	}
	return pluginName, action
}
