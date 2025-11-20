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

package jwt

import (
	"errors"

	jwt5 "github.com/golang-jwt/jwt/v5"
)

// TokenType constants define the different types of authentication tokens used in the system.
const (
	// TokenTypeBootstrap represents a bootstrap token used for initial session setup. These are
	// short-lived and exchanged by the service for a session token.
	TokenTypeBootstrap = "bootstrap"
	// TokenTypeSession represents a session token used for ongoing authenticated requests
	TokenTypeSession = "session"
)

// Common errors
var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenExpired     = errors.New("token expired")
	ErrInvalidSignature = errors.New("invalid token signature")
	ErrInvalidClaims    = errors.New("invalid token claims")
	ErrDomainMismatch   = errors.New("token domain mismatch")
)

// Claims represents the JWT claims for our auth token
type Claims struct {
	jwt5.RegisteredClaims
	User        string              `json:"User,omitempty"`
	Groups      []string            `json:"Groups,omitempty"`
	UID         string              `json:"Uid,omitempty"`
	Extra       map[string][]string `json:"Extra,omitempty"`
	Path        string              `json:"Path,omitempty"`
	Domain      string              `json:"Domain,omitempty"`
	TokenType   string              `json:"TokenType,omitempty"`
	SkipRefresh bool                `json:"SkipRefresh,omitempty"`
}
