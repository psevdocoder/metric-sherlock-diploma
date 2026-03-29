package httpapi

import (
	"context"
	"errors"
	"net/http"

	"git.server.lan/maksim/metric-sherlock-diploma/pkg/jwtclaims"
	targetgroupsv1 "git.server.lan/maksim/metric-sherlock-diploma/proto/metricsherlock/targetgroups/v1"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

func NewHandler(storage targetGroupStorage, verifier jwtclaims.Verifier) (http.Handler, error) {
	if verifier == nil {
		return nil, errors.New("jwt verifier is nil")
	}

	service := newTargetGroupsService(storage)
	gwMux := runtime.NewServeMux()

	if err := targetgroupsv1.RegisterTargetGroupsServiceHandlerServer(context.Background(), gwMux, service); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", authMiddleware(verifier)(gwMux))
	mux.HandleFunc("/swagger", swaggerRedirectHandler)
	mux.HandleFunc("/swagger/", swaggerUIHandler)
	mux.HandleFunc("/swagger/target-groups.json", swaggerJSONHandler)

	return mux, nil
}

func swaggerRedirectHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger/", http.StatusTemporaryRedirect)
}

func swaggerJSONHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(targetGroupsSwaggerJSON)
}

func swaggerUIHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/swagger/" && r.URL.Path != "/swagger/index.html" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerPageHTML))
}

const swaggerPageHTML = `<!doctype html>
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
    window.ui = SwaggerUIBundle({
      url: "/swagger/target-groups.json",
      dom_id: "#swagger-ui"
    });
  </script>
</body>
</html>
`
