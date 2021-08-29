package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"golang.org/x/time/rate"
)

// RateLimitConfig contains the information necessary to configure rate
// limiting.
type RateLimitConfig struct {
	// AuthTokens defines the set of valid auth tokens for which no rate
	// limiting will be applied.
	AuthTokens []string

	// Limiter is a rate limiter configure with an appropriate limit and burst.
	Limiter *rate.Limiter
}

func rateLimitHandler(authTokens []string, rateLimiter *rate.Limiter, next http.Handler) http.Handler {
	authTokenMap := stringSliceToMap(authTokens)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If a known API key is provided, no rate limiting is necessary
		authToken := authTokenFromRequest(r)
		if _, ok := authTokenMap[authToken]; ok {
			next.ServeHTTP(w, r)
			return
		}

		if !rateLimiter.Allow() {
			log.Printf("ratelimit: dry run: would deny request from %s %#v", getRemoteAddr(r), r.Header)
			// sendRateLimitError(w, rateLimiter)
			// return
		}

		next.ServeHTTP(w, r)
	})
}

func authTokenFromRequest(r *http.Request) string {
	val := r.Header.Get("Authorization")
	if val == "" {
		return ""
	}

	if !strings.HasPrefix(strings.ToLower(val), "token ") {
		return ""
	}

	parts := strings.Fields(val)
	if len(parts) != 2 {
		return ""
	}

	return parts[1]
}

func sendRateLimitError(w http.ResponseWriter, rl *rate.Limiter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": fmt.Sprintf("Anonymous request rate limit of %0f req/sec exceeded. Try again later.", rl.Limit()),
	})
}

func stringSliceToMap(xs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, k := range xs {
		k = strings.TrimSpace(k)
		if k != "" {
			m[k] = struct{}{}
		}
	}
	return m
}
