package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"git.server.lan/maksim/metric-sherlock-diploma/pkg/jwtclaims"
	"git.server.lan/pkg/zaplogger/logger"
	"go.uber.org/zap"
)

var errInvalidAuthorizationHeader = errors.New("invalid authorization header")

func authMiddleware(verifier jwtclaims.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := extractBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			claims, err := verifier.ParseAndValidate(r.Context(), token)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized")
				logger.Error("Failed to verify token", zap.String("token", token), zap.Error(err))
				return
			}

			next.ServeHTTP(w, r.WithContext(jwtclaims.WithClaims(r.Context(), claims)))
		})
	}
}

func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	if len(roles) == 0 {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !jwtclaims.HasAnyRole(r.Context(), roles...) {
				writeAuthError(w, http.StatusForbidden, "forbidden")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractBearerToken(authHeader string) (string, error) {
	fields := strings.Fields(authHeader)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return "", errInvalidAuthorizationHeader
	}

	if fields[1] == "" {
		return "", errInvalidAuthorizationHeader
	}

	return fields[1], nil
}

func writeAuthError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}
