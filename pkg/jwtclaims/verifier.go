package jwtclaims

import (
	"context"
	"errors"
)

var (
	ErrTokenMalformed            = errors.New("token is malformed")
	ErrTokenUnsupportedAlgorithm = errors.New("token uses unsupported algorithm")
	ErrTokenInvalidSignature     = errors.New("token signature is invalid")
	ErrTokenExpired              = errors.New("token is expired")
	ErrTokenInvalidClaims        = errors.New("token claims are invalid")
	ErrTokenIssuerMismatch       = errors.New("token issuer mismatch")
	ErrTokenAZPMismatch          = errors.New("token azp mismatch")
	ErrTokenKeyNotFound          = errors.New("token signing key not found")
)

type Verifier interface {
	ParseAndValidate(ctx context.Context, rawToken string) (*Claims, error)
}
