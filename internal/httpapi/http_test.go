package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecorateSwaggerSpecWithSSO(t *testing.T) {
	in := []byte(`{"swagger":"2.0","paths":{}}`)

	out, err := decorateSwaggerSpecWithSSO(in, "http://localhost:8080/realms/local/")
	if err != nil {
		t.Fatalf("decorateSwaggerSpecWithSSO() error = %v", err)
	}

	var spec map[string]any
	if err := json.Unmarshal(out, &spec); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	securityDefinitions, ok := spec["securityDefinitions"].(map[string]any)
	if !ok {
		t.Fatalf("securityDefinitions is missing or invalid: %T", spec["securityDefinitions"])
	}

	sso, ok := securityDefinitions[ssoSecuritySchemeName].(map[string]any)
	if !ok {
		t.Fatalf("security definition %q is missing", ssoSecuritySchemeName)
	}

	if got, want := sso["flow"], "accessCode"; got != want {
		t.Fatalf("flow mismatch: got %v, want %v", got, want)
	}
	if got, want := sso["authorizationUrl"], "http://localhost:8080/realms/local/protocol/openid-connect/auth"; got != want {
		t.Fatalf("authorizationUrl mismatch: got %v, want %v", got, want)
	}
	if got, want := sso["tokenUrl"], "http://localhost:8080/realms/local/protocol/openid-connect/token"; got != want {
		t.Fatalf("tokenUrl mismatch: got %v, want %v", got, want)
	}

	security, ok := spec["security"].([]any)
	if !ok || len(security) != 1 {
		t.Fatalf("security is missing or invalid: %T", spec["security"])
	}

	securityRule, ok := security[0].(map[string]any)
	if !ok {
		t.Fatalf("security rule has invalid type: %T", security[0])
	}
	scopes, ok := securityRule[ssoSecuritySchemeName].([]any)
	if !ok {
		t.Fatalf("security rule for %q has invalid type: %T", ssoSecuritySchemeName, securityRule[ssoSecuritySchemeName])
	}
	if len(scopes) != 0 {
		t.Fatalf("unexpected scopes length: got %d, want 0", len(scopes))
	}
}

func TestDecorateSwaggerSpecWithSSO_EmptyIDP(t *testing.T) {
	in := []byte(`{"swagger":"2.0","paths":{}}`)

	out, err := decorateSwaggerSpecWithSSO(in, "  ")
	if err != nil {
		t.Fatalf("decorateSwaggerSpecWithSSO() error = %v", err)
	}

	if !bytes.Equal(out, in) {
		t.Fatalf("expected swagger spec unchanged when idp is empty")
	}
}

func TestRenderSwaggerPageHTML_WithClientID(t *testing.T) {
	html, err := renderSwaggerPageHTML("swagger-ui-client")
	if err != nil {
		t.Fatalf("renderSwaggerPageHTML() error = %v", err)
	}

	if !strings.Contains(html, `const swaggerOAuthClientId = "swagger-ui-client";`) {
		t.Fatalf("rendered HTML does not contain client id")
	}
}

func TestRenderSwaggerPageHTML_WithoutClientID(t *testing.T) {
	html, err := renderSwaggerPageHTML(" ")
	if err != nil {
		t.Fatalf("renderSwaggerPageHTML() error = %v", err)
	}

	if !strings.Contains(html, `const swaggerOAuthClientId = "";`) {
		t.Fatalf("expected empty client id in rendered HTML")
	}
}

func TestSwaggerOAuth2RedirectPage(t *testing.T) {
	handler, err := NewHandler(nil, nil, verifierStub{}, "http://localhost:8080/realms/local", "metric-sherlock")
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/swagger/oauth2-redirect.html?code=test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "swaggerUIRedirectOauth2") {
		t.Fatalf("oauth2 redirect helper script not found in response")
	}
}
