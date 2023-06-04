package middleware

import (
	"net/http"

	"github.com/honeycombio/beeline-go"
)

func corsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := beeline.StartSpan(r.Context(), "httphandler.middleware.corsHandler")
		defer span.Send()

		origin := r.Header.Get("Origin")
		if origin != "" {
			beeline.AddField(r.Context(), "cors_origin", origin)
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
