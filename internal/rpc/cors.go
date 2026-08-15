package rpc

import (
	"net/http"
	"slices"
	"strings"
)

var (
	allowedHeaders = []string{
		"Content-Type",
		"Connect-Protocol-Version",
		"Connect-Timeout-Ms",
		"Connect-Accept-Encoding",
		"Connect-Content-Encoding",
		"Grpc-Timeout",
		"X-Grpc-Web",
		"X-User-Agent",
	}
	exposedHeaders = []string{
		"Connect-Content-Encoding",
		"Connect-Accept-Encoding",
		"Grpc-Status",
		"Grpc-Message",
		"Grpc-Status-Details-Bin",
	}
)

func CORS(origins []string, next http.Handler) http.Handler {
	if len(origins) == 0 {
		return next
	}

	allowHeaders := strings.Join(allowedHeaders, ", ")
	exposeHeaders := strings.Join(exposedHeaders, ", ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(origins, origin) {
			header := w.Header()
			header.Set("Access-Control-Allow-Origin", origin)
			header.Set("Access-Control-Allow-Credentials", "true")
			header.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			header.Set("Access-Control-Allow-Headers", allowHeaders)
			header.Set("Access-Control-Expose-Headers", exposeHeaders)
			header.Set("Access-Control-Max-Age", "7200")
			header.Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
