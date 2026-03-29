package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.server.lan/maksim/metric-sherlock-diploma/pkg/jwtclaims"
)

func TestAuthMiddlewareSuccess(t *testing.T) {
	verifier := verifierStub{
		claims: &jwtclaims.Claims{
			PreferredUsername: "tester",
			Roles:             []string{"admin"},
		},
	}

	handler := authMiddleware(verifier)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := jwtclaims.ClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("expected claims in context")
		}
		if claims.PreferredUsername != "tester" {
			t.Fatalf("unexpected username: %q", claims.PreferredUsername)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/target-groups", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}
}

func TestAuthMiddlewareUnauthorized(t *testing.T) {
	handler := authMiddleware(verifierStub{})(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/target-groups", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}
}

func TestRequireAnyRole(t *testing.T) {
	protectedHandler := RequireAnyRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	reqWithoutRole := httptest.NewRequest(http.MethodGet, "/api/v1/target-groups", nil)
	reqWithoutRole = reqWithoutRole.WithContext(jwtclaims.WithClaims(reqWithoutRole.Context(), &jwtclaims.Claims{
		Roles: []string{"user"},
	}))
	recorder := httptest.NewRecorder()
	protectedHandler.ServeHTTP(recorder, reqWithoutRole)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected status code without role: %d", recorder.Code)
	}

	reqWithRole := httptest.NewRequest(http.MethodGet, "/api/v1/target-groups", nil)
	reqWithRole = reqWithRole.WithContext(jwtclaims.WithClaims(reqWithRole.Context(), &jwtclaims.Claims{
		Roles: []string{"admin"},
	}))
	recorder = httptest.NewRecorder()
	protectedHandler.ServeHTTP(recorder, reqWithRole)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code with role: %d", recorder.Code)
	}
}

type verifierStub struct {
	claims *jwtclaims.Claims
	err    error
}

func (v verifierStub) ParseAndValidate(_ context.Context, rawToken string) (*jwtclaims.Claims, error) {
	if rawToken == "" {
		return nil, errors.New("empty token")
	}

	if v.err != nil {
		return nil, v.err
	}

	return v.claims, nil
}
