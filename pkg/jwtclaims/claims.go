package jwtclaims

import "context"

type Claims struct {
	Exp               int64    `json:"exp"`
	Iat               int64    `json:"iat"`
	AuthTime          int64    `json:"auth_time"`
	JTI               string   `json:"jti"`
	Iss               string   `json:"iss"`
	Sub               string   `json:"sub"`
	Typ               string   `json:"typ"`
	Azp               string   `json:"azp"`
	Sid               string   `json:"sid"`
	Acr               string   `json:"acr"`
	AllowedOrigins    []string `json:"allowed-origins"`
	Scope             string   `json:"scope"`
	Roles             []string `json:"roles"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	GivenName         string   `json:"given_name"`
	FamilyName        string   `json:"family_name"`
}

func (c *Claims) HasAnyRole(roles ...string) bool {
	if c == nil || len(roles) == 0 || len(c.Roles) == 0 {
		return false
	}

	roleSet := make(map[string]struct{}, len(c.Roles))
	for _, role := range c.Roles {
		roleSet[role] = struct{}{}
	}

	for _, role := range roles {
		if _, exists := roleSet[role]; exists {
			return true
		}
	}

	return false
}

type contextKey struct{}

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	if claims == nil {
		return ctx
	}

	return context.WithValue(ctx, contextKey{}, claims)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(contextKey{}).(*Claims)
	if !ok || claims == nil {
		return nil, false
	}

	return claims, true
}

func RolesFromContext(ctx context.Context) []string {
	claims, ok := ClaimsFromContext(ctx)
	if !ok || len(claims.Roles) == 0 {
		return nil
	}

	roles := make([]string, len(claims.Roles))
	copy(roles, claims.Roles)

	return roles
}

func HasAnyRole(ctx context.Context, roles ...string) bool {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return false
	}

	return claims.HasAnyRole(roles...)
}
