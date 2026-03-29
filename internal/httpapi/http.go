package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"git.server.lan/maksim/metric-sherlock-diploma/pkg/jwtclaims"
	targetgroupsv1 "git.server.lan/maksim/metric-sherlock-diploma/proto/metricsherlock/targetgroups/v1"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

const ssoSecuritySchemeName = "sso"

func NewHandler(
	storage targetGroupStorage,
	settings runtimeSettings,
	verifier jwtclaims.Verifier,
	swaggerSSOIDP string,
	swaggerOAuthClientID string,
) (http.Handler, error) {
	if verifier == nil {
		return nil, errors.New("jwt verifier is nil")
	}

	swaggerSpec, err := decorateSwaggerSpecWithSSO(targetGroupsSwaggerJSON, swaggerSSOIDP)
	if err != nil {
		return nil, err
	}
	swaggerPageHTML, err := renderSwaggerPageHTML(swaggerOAuthClientID)
	if err != nil {
		return nil, err
	}

	service := newTargetGroupsService(storage, settings)
	gwMux := runtime.NewServeMux()

	if err := targetgroupsv1.RegisterTargetGroupsServiceHandlerServer(context.Background(), gwMux, service); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", authMiddleware(verifier)(gwMux))
	mux.HandleFunc("/swagger", swaggerRedirectHandler)
	mux.HandleFunc("/swagger/oauth2-redirect.html", swaggerOAuth2RedirectHandler)
	mux.HandleFunc("/swagger/", swaggerUIHandler(swaggerPageHTML))
	mux.HandleFunc("/swagger/target-groups.json", swaggerJSONHandler(swaggerSpec))

	return mux, nil
}

func swaggerRedirectHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger/", http.StatusTemporaryRedirect)
}

func decorateSwaggerSpecWithSSO(swaggerSpec []byte, idp string) ([]byte, error) {
	idp = strings.TrimSpace(strings.TrimRight(idp, "/"))
	if idp == "" {
		return swaggerSpec, nil
	}

	spec := make(map[string]any)
	if err := json.Unmarshal(swaggerSpec, &spec); err != nil {
		return nil, err
	}

	securityDefinitions, _ := spec["securityDefinitions"].(map[string]any)
	if securityDefinitions == nil {
		securityDefinitions = make(map[string]any)
	}
	securityDefinitions[ssoSecuritySchemeName] = map[string]any{
		"type":             "oauth2",
		"flow":             "accessCode",
		"authorizationUrl": idp + "/protocol/openid-connect/auth",
		"tokenUrl":         idp + "/protocol/openid-connect/token",
		"scopes":           map[string]string{},
	}
	spec["securityDefinitions"] = securityDefinitions

	security, _ := spec["security"].([]any)
	hasSSORequirement := false
	for _, securityRequirement := range security {
		requirementMap, ok := securityRequirement.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := requirementMap[ssoSecuritySchemeName]; exists {
			hasSSORequirement = true
			break
		}
	}
	if !hasSSORequirement {
		security = append(security, map[string]any{ssoSecuritySchemeName: []string{}})
	}
	spec["security"] = security

	return json.Marshal(spec)
}

func swaggerJSONHandler(swaggerSpec []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(swaggerSpec)
	}
}

func swaggerUIHandler(swaggerPageHTML string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/swagger/" && r.URL.Path != "/swagger/index.html" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerPageHTML))
	}
}

func swaggerOAuth2RedirectHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/swagger/oauth2-redirect.html" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerOAuth2RedirectHTML))
}

func renderSwaggerPageHTML(swaggerOAuthClientID string) (string, error) {
	clientID, err := json.Marshal(strings.TrimSpace(swaggerOAuthClientID))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(swaggerPageHTMLTemplate, string(clientID)), nil
}

const swaggerPageHTMLTemplate = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Metric Sherlock API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    const swaggerOAuthClientId = %s;
    window.ui = SwaggerUIBundle({
      url: "/swagger/target-groups.json",
      dom_id: "#swagger-ui",
      oauth2RedirectUrl: window.location.origin + "/swagger/oauth2-redirect.html"
    });
    const oauthConfig = {
      usePkceWithAuthorizationCodeGrant: true
    };
    if (swaggerOAuthClientId) {
      oauthConfig.clientId = swaggerOAuthClientId;
    }
    window.ui.initOAuth(oauthConfig);
  </script>
</body>
</html>
`

const swaggerOAuth2RedirectHTML = `<!doctype html>
<html lang="en-US">
<head>
  <title>Swagger UI: OAuth2 Redirect</title>
</head>
<body>
  <script>
    'use strict';
    function run() {
      var oauth2 = window.opener.swaggerUIRedirectOauth2;
      var sentState = oauth2.state;
      var redirectUrl = oauth2.redirectUrl;
      var qp;

      if (/code|token|error/.test(window.location.hash)) {
        qp = window.location.hash.substring(1).replace('?', '&');
      } else {
        qp = location.search.substring(1);
      }

      var arr = qp.split('&').filter(function(v) { return v; });
      arr.forEach(function(v, i, _arr) {
        _arr[i] = '"' + v.replace('=', '":"') + '"';
      });
      qp = qp ? JSON.parse('{' + arr.join() + '}', function(key, value) {
        return key === '' ? value : decodeURIComponent(value);
      }) : {};

      var isValid = qp.state === sentState;

      if (
        (oauth2.auth.schema.get('flow') === 'accessCode' ||
          oauth2.auth.schema.get('flow') === 'authorizationCode' ||
          oauth2.auth.schema.get('flow') === 'authorization_code') &&
        !oauth2.auth.code
      ) {
        if (!isValid) {
          oauth2.errCb({
            authId: oauth2.auth.name,
            source: 'auth',
            level: 'warning',
            message: 'Authorization may be unsafe, passed state was changed in server',
          });
        }

        if (qp.code) {
          delete oauth2.state;
          oauth2.auth.code = qp.code;
          oauth2.callback({ auth: oauth2.auth, redirectUrl: redirectUrl });
        } else {
          var oauthErrorMsg;
          if (qp.error) {
            oauthErrorMsg =
              '[' + qp.error + ']: ' +
              (qp.error_description || 'no accessCode received from the server');
          }
          oauth2.errCb({
            authId: oauth2.auth.name,
            source: 'auth',
            level: 'error',
            message: oauthErrorMsg || '[Authorization failed]: no accessCode received from the server',
          });
        }
      } else {
        oauth2.callback({
          auth: oauth2.auth,
          token: qp.access_token,
          isValid: isValid,
          redirectUrl: redirectUrl,
        });
      }

      window.close();
    }

    if (document.readyState !== 'loading') {
      run();
    } else {
      document.addEventListener('DOMContentLoaded', run);
    }
  </script>
</body>
</html>
`
