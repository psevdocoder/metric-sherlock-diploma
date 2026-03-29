package jwtclaims

import (
	"context"
	"testing"
)

func TestClaimsContextHelpers(t *testing.T) {
	original := &Claims{
		PreferredUsername: "tester",
		Roles:             []string{"admin", "user"},
	}

	ctx := WithClaims(context.Background(), original)

	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("expected claims in context")
	}
	if claims.PreferredUsername != "tester" {
		t.Fatalf("unexpected username: %q", claims.PreferredUsername)
	}

	roles := RolesFromContext(ctx)
	if len(roles) != 2 {
		t.Fatalf("unexpected roles length: %d", len(roles))
	}
	if !HasAnyRole(ctx, "admin") {
		t.Fatal("expected admin role")
	}
	if HasAnyRole(ctx, "guest") {
		t.Fatal("guest role should be absent")
	}

	roles[0] = "changed"
	if original.Roles[0] != "admin" {
		t.Fatal("returned roles must be copied")
	}
}
