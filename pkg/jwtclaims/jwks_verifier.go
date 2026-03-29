package jwtclaims

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultClockSkew = 30 * time.Second
	defaultCacheTTL  = 5 * time.Minute
	defaultHTTPWait  = 5 * time.Second
	maxJWKSBodyBytes = 1 << 20
)

type Config struct {
	Issuer       string
	JWKSEndpoint string
	ExpectedAZP  string
	ClockSkew    time.Duration
	CacheTTL     time.Duration
	HTTPClient   *http.Client
}

type jwksVerifier struct {
	issuer       string
	jwksEndpoint string
	expectedAZP  string
	clockSkew    time.Duration
	cacheTTL     time.Duration
	httpClient   *http.Client

	mu         sync.RWMutex
	keysByKid  map[string]*rsa.PublicKey
	cacheUntil time.Time
}

func NewJWKSVerifier(cfg Config) (Verifier, error) {
	issuer := strings.TrimSpace(strings.TrimRight(cfg.Issuer, "/"))
	if issuer == "" {
		return nil, errorsJoin(ErrTokenInvalidClaims, fmt.Errorf("issuer is required"))
	}

	jwksEndpoint, err := resolveJWKSEndpoint(issuer, cfg.JWKSEndpoint)
	if err != nil {
		return nil, err
	}

	clockSkew := cfg.ClockSkew
	if clockSkew <= 0 {
		clockSkew = defaultClockSkew
	}

	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPWait}
	}

	return &jwksVerifier{
		issuer:       issuer,
		jwksEndpoint: jwksEndpoint,
		expectedAZP:  strings.TrimSpace(cfg.ExpectedAZP),
		clockSkew:    clockSkew,
		cacheTTL:     cacheTTL,
		httpClient:   httpClient,
		keysByKid:    make(map[string]*rsa.PublicKey),
	}, nil
}

func resolveJWKSEndpoint(issuer, rawEndpoint string) (string, error) {
	endpoint := strings.TrimSpace(rawEndpoint)
	if endpoint == "" || strings.EqualFold(endpoint, "string") {
		return issuer + "/protocol/openid-connect/certs", nil
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errorsJoin(ErrTokenInvalidClaims, fmt.Errorf("invalid jwks endpoint %q", endpoint))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errorsJoin(ErrTokenInvalidClaims, fmt.Errorf("invalid jwks endpoint scheme %q", parsed.Scheme))
	}

	return endpoint, nil
}

func (v *jwksVerifier) ParseAndValidate(ctx context.Context, rawToken string) (*Claims, error) {
	parts := strings.Split(strings.TrimSpace(rawToken), ".")
	if len(parts) != 3 {
		return nil, ErrTokenMalformed
	}

	header, err := decodeHeader(parts[0])
	if err != nil {
		return nil, err
	}

	signingKey, err := v.getSigningKey(ctx, header.KID)
	if err != nil {
		return nil, err
	}

	if err = verifyRS256(parts[0]+"."+parts[1], parts[2], signingKey); err != nil {
		return nil, err
	}

	claims, err := decodeClaims(parts[1])
	if err != nil {
		return nil, err
	}

	if err = v.validateClaims(claims); err != nil {
		return nil, err
	}

	return claims, nil
}

func (v *jwksVerifier) validateClaims(claims *Claims) error {
	now := time.Now()

	if claims.Exp == 0 {
		return errorsJoin(ErrTokenInvalidClaims, fmt.Errorf("exp claim is required"))
	}

	if now.After(time.Unix(claims.Exp, 0).Add(v.clockSkew)) {
		return ErrTokenExpired
	}

	if claims.Iat > 0 && time.Unix(claims.Iat, 0).After(now.Add(v.clockSkew)) {
		return errorsJoin(ErrTokenInvalidClaims, fmt.Errorf("iat claim is in the future"))
	}

	if claims.AuthTime > 0 && time.Unix(claims.AuthTime, 0).After(now.Add(v.clockSkew)) {
		return errorsJoin(ErrTokenInvalidClaims, fmt.Errorf("auth_time claim is in the future"))
	}

	if claims.Iss != v.issuer {
		return errorsJoin(ErrTokenIssuerMismatch, fmt.Errorf("expected %q, got %q", v.issuer, claims.Iss))
	}

	if v.expectedAZP != "" && claims.Azp != v.expectedAZP {
		return errorsJoin(ErrTokenAZPMismatch, fmt.Errorf("expected %q, got %q", v.expectedAZP, claims.Azp))
	}

	return nil
}

func (v *jwksVerifier) getSigningKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if key, ok := v.getCachedKey(kid); ok {
		return key, nil
	}

	if err := v.refreshJWKS(ctx); err != nil {
		return nil, err
	}

	if key, ok := v.getCachedKey(kid); ok {
		return key, nil
	}

	return nil, errorsJoin(ErrTokenKeyNotFound, fmt.Errorf("kid %q", kid))
}

func (v *jwksVerifier) getCachedKey(kid string) (*rsa.PublicKey, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if len(v.keysByKid) == 0 || time.Now().After(v.cacheUntil) {
		return nil, false
	}

	if kid != "" {
		key, ok := v.keysByKid[kid]
		return key, ok
	}

	if len(v.keysByKid) == 1 {
		for _, key := range v.keysByKid {
			return key, true
		}
	}

	return nil, false
}

func (v *jwksVerifier) refreshJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksEndpoint, nil)
	if err != nil {
		return errorsJoin(ErrTokenKeyNotFound, err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return errorsJoin(ErrTokenKeyNotFound, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errorsJoin(ErrTokenKeyNotFound, fmt.Errorf("jwks endpoint returned status %d", resp.StatusCode))
	}

	keysByKid, err := parseJWKS(resp.Body)
	if err != nil {
		return errorsJoin(ErrTokenKeyNotFound, err)
	}

	if len(keysByKid) == 0 {
		return errorsJoin(ErrTokenKeyNotFound, fmt.Errorf("jwks has no usable rsa keys"))
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	v.keysByKid = keysByKid
	v.cacheUntil = time.Now().Add(v.cacheTTL)

	return nil
}

type tokenHeader struct {
	Alg string `json:"alg"`
	KID string `json:"kid"`
	Typ string `json:"typ"`
}

func decodeHeader(raw string) (*tokenHeader, error) {
	decoded, err := decodeBase64URL(raw)
	if err != nil {
		return nil, errorsJoin(ErrTokenMalformed, err)
	}

	var header tokenHeader
	if err = json.Unmarshal(decoded, &header); err != nil {
		return nil, errorsJoin(ErrTokenMalformed, err)
	}

	if header.Alg != "RS256" {
		return nil, errorsJoin(ErrTokenUnsupportedAlgorithm, fmt.Errorf("got %q", header.Alg))
	}

	return &header, nil
}

func decodeClaims(raw string) (*Claims, error) {
	decoded, err := decodeBase64URL(raw)
	if err != nil {
		return nil, errorsJoin(ErrTokenMalformed, err)
	}

	var claims Claims
	if err = json.Unmarshal(decoded, &claims); err != nil {
		return nil, errorsJoin(ErrTokenMalformed, err)
	}

	return &claims, nil
}

func verifyRS256(signingInput string, rawSignature string, key *rsa.PublicKey) error {
	signature, err := decodeBase64URL(rawSignature)
	if err != nil {
		return errorsJoin(ErrTokenInvalidSignature, err)
	}

	hash := sha256.Sum256([]byte(signingInput))
	if err = rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); err != nil {
		return errorsJoin(ErrTokenInvalidSignature, err)
	}

	return nil
}

func decodeBase64URL(raw string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}

	return decoded, nil
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func parseJWKS(r io.Reader) (map[string]*rsa.PublicKey, error) {
	var doc jwksDocument
	if err := json.NewDecoder(io.LimitReader(r, maxJWKSBodyBytes)).Decode(&doc); err != nil {
		return nil, err
	}

	keysByKid := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, key := range doc.Keys {
		if key.Kty != "RSA" {
			continue
		}
		if key.Use != "" && key.Use != "sig" {
			continue
		}
		if key.Alg != "" && key.Alg != "RS256" {
			continue
		}

		publicKey, err := toRSAPublicKey(key.N, key.E)
		if err != nil {
			continue
		}

		keysByKid[key.Kid] = publicKey
	}

	return keysByKid, nil
}

func toRSAPublicKey(rawN string, rawE string) (*rsa.PublicKey, error) {
	nBytes, err := decodeBase64URL(rawN)
	if err != nil {
		return nil, err
	}

	eBytes, err := decodeBase64URL(rawE)
	if err != nil {
		return nil, err
	}

	eBig := big.NewInt(0).SetBytes(eBytes)
	if !eBig.IsInt64() {
		return nil, fmt.Errorf("rsa exponent is too large")
	}

	e := int(eBig.Int64())
	if e <= 1 {
		return nil, fmt.Errorf("rsa exponent is invalid")
	}

	n := big.NewInt(0).SetBytes(nBytes)
	if n.Sign() <= 0 {
		return nil, fmt.Errorf("rsa modulus is invalid")
	}

	return &rsa.PublicKey{
		N: n,
		E: e,
	}, nil
}

func errorsJoin(base error, err error) error {
	return fmt.Errorf("%w: %v", base, err)
}
