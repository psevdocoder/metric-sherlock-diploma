package jwtclaims

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestJWKSVerifierParseAndValidateSuccess(t *testing.T) {
	privateKey := generatePrivateKey(t)
	kid := "kid-1"
	issuer := "http://localhost:8080/realms/local"
	jwksBody := buildJWKSJSON(t, &privateKey.PublicKey, kid)

	verifier, err := NewJWKSVerifier(Config{
		Issuer:       issuer,
		JWKSEndpoint: "http://jwks.local/certs",
		ExpectedAZP:  "testtesttest",
		HTTPClient: &http.Client{
			Transport: staticRoundTripper{
				statusCode: http.StatusOK,
				body:       jwksBody,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	now := time.Now().Unix()
	token := buildRS256Token(t, privateKey, kid, map[string]any{
		"exp":                now + 300,
		"iat":                now,
		"auth_time":          now - 100,
		"jti":                "jti-1",
		"iss":                issuer,
		"sub":                "sub-1",
		"typ":                "Bearer",
		"azp":                "testtesttest",
		"sid":                "sid-1",
		"acr":                "0",
		"allowed-origins":    []string{"http://localhost:3001"},
		"scope":              "openid profile",
		"roles":              []string{"admin", "user"},
		"name":               "name surname",
		"preferred_username": "iamsherlockadmin",
		"given_name":         "name",
		"family_name":        "surname",
	})

	claims, err := verifier.ParseAndValidate(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if claims.PreferredUsername != "iamsherlockadmin" {
		t.Fatalf("unexpected preferred username: %q", claims.PreferredUsername)
	}
	if !claims.HasAnyRole("admin") {
		t.Fatalf("expected role admin to be present")
	}
}

func TestJWKSVerifierParseAndValidateExpired(t *testing.T) {
	privateKey := generatePrivateKey(t)
	kid := "kid-1"
	issuer := "http://localhost:8080/realms/local"
	jwksBody := buildJWKSJSON(t, &privateKey.PublicKey, kid)

	verifier, err := NewJWKSVerifier(Config{
		Issuer:       issuer,
		JWKSEndpoint: "http://jwks.local/certs",
		HTTPClient: &http.Client{
			Transport: staticRoundTripper{
				statusCode: http.StatusOK,
				body:       jwksBody,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	now := time.Now().Unix()
	token := buildRS256Token(t, privateKey, kid, map[string]any{
		"exp": now - 3600,
		"iat": now - 4000,
		"iss": issuer,
	})

	_, err = verifier.ParseAndValidate(context.Background(), token)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestJWKSVerifierParseAndValidateInvalidSignature(t *testing.T) {
	publicKeySource := generatePrivateKey(t)
	signingKey := generatePrivateKey(t)
	kid := "kid-1"
	issuer := "http://localhost:8080/realms/local"
	jwksBody := buildJWKSJSON(t, &publicKeySource.PublicKey, kid)

	verifier, err := NewJWKSVerifier(Config{
		Issuer:       issuer,
		JWKSEndpoint: "http://jwks.local/certs",
		HTTPClient: &http.Client{
			Transport: staticRoundTripper{
				statusCode: http.StatusOK,
				body:       jwksBody,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	now := time.Now().Unix()
	token := buildRS256Token(t, signingKey, kid, map[string]any{
		"exp": now + 3600,
		"iat": now,
		"iss": issuer,
	})

	_, err = verifier.ParseAndValidate(context.Background(), token)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !errors.Is(err, ErrTokenInvalidSignature) {
		t.Fatalf("expected ErrTokenInvalidSignature, got %v", err)
	}
}

func TestNewJWKSVerifierUsesIssuerJWKSWhenEndpointPlaceholder(t *testing.T) {
	issuer := "http://localhost:8080/realms/local"

	verifier, err := NewJWKSVerifier(Config{
		Issuer:       issuer,
		JWKSEndpoint: "string",
	})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	jwksVerifier, ok := verifier.(*jwksVerifier)
	if !ok {
		t.Fatalf("unexpected verifier type: %T", verifier)
	}

	if got, want := jwksVerifier.jwksEndpoint, issuer+"/protocol/openid-connect/certs"; got != want {
		t.Fatalf("unexpected jwks endpoint: got %q, want %q", got, want)
	}
}

func TestNewJWKSVerifierInvalidJWKSEndpoint(t *testing.T) {
	_, err := NewJWKSVerifier(Config{
		Issuer:       "http://localhost:8080/realms/local",
		JWKSEndpoint: "invalid-endpoint",
	})
	if err == nil {
		t.Fatal("expected constructor error")
	}
	if !errors.Is(err, ErrTokenInvalidClaims) {
		t.Fatalf("expected ErrTokenInvalidClaims, got %v", err)
	}
}

func generatePrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa private key: %v", err)
	}

	return privateKey
}

func buildJWKSJSON(t *testing.T, publicKey *rsa.PublicKey, kid string) string {
	t.Helper()

	n := base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes())

	body, err := json.Marshal(map[string]any{
		"keys": []map[string]any{
			{
				"kid": kid,
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"n":   n,
				"e":   e,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal jwks response: %v", err)
	}

	return string(body)
}

func buildRS256Token(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()

	headerJSON, err := json.Marshal(map[string]any{
		"alg": "RS256",
		"kid": kid,
		"typ": "JWT",
	})
	if err != nil {
		t.Fatalf("failed to marshal header: %v", err)
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("failed to marshal claims: %v", err)
	}

	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := header + "." + payload
	hash := sha256.Sum256([]byte(signingInput))

	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

type staticRoundTripper struct {
	statusCode int
	body       string
}

func (s staticRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}
