/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusError_WithMessage(t *testing.T) {
	err := &StatusError{Code: 400, Message: "bad request"}
	assert.Equal(t, "plugin error (HTTP 400): bad request", err.Error())
}

func TestStatusError_WithoutMessage(t *testing.T) {
	err := &StatusError{Code: 500}
	assert.Equal(t, "plugin error (HTTP 500)", err.Error())
}

func TestStatusError_ImplementsError(t *testing.T) {
	var err error = &StatusError{Code: 404, Message: "not found"}
	assert.Error(t, err)
}
