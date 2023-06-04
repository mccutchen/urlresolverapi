package middleware

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/honeycombio/beeline-go"
	"golang.org/x/time/rate"
)

var (
	errInvalidAuthHeaderFormat = errors.New("INVALID_AUTH_HEADER_FORMAT")
	errInvalidAuthTokenFormat  = errors.New("INVALID_AUTH_TOKEN_FORMAT")
	errInvalidAuthToken        = errors.New("INVALID_AUTH_TOKEN")
)

func rateLimitHandler(next http.Handler, rateLimiter *rate.Limiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := beeline.StartSpan(r.Context(), "httphandler.middleware.rateLimitHandler")
		defer span.Send()

		clientID := clientIDFromContext(ctx)
		r = r.WithContext(ctx)

		if rateLimiter == nil {
			beeline.AddField(r.Context(), "rate_limit_result", "skipped_disabled")
			next.ServeHTTP(w, r)
			return
		}

		// If a known API key is provided, no rate limiting is necessary
		if clientID != "" {
			beeline.AddField(r.Context(), "rate_limit_result", "skipped_authenticated")
			next.ServeHTTP(w, r)
			return
		}

		if !rateLimiter.Allow() {
			beeline.AddField(r.Context(), "rate_limit_result", "denied_anonymous")
			sendRateLimitError(w, rateLimiter)
			return
		}

		beeline.AddField(r.Context(), "rate_limit_result", "allowed_anonymous")
		next.ServeHTTP(w, r)
	})
}

func sendRateLimitError(w http.ResponseWriter, rl *rate.Limiter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = fmt.Fprintf(w, `{"error": "Anonymous request rate limit of %0f req/sec exceeded. Try again later."}`, rl.Limit())
}
