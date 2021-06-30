package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestRateLimiter(t *testing.T) {
	t.Parallel()

	authTokens := []string{"valid-token"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	testCases := map[string]struct {
		rl         *rate.Limiter
		headers    map[string]string
		wantStatus int
	}{
		"valid api key not rate limited": {
			headers:    map[string]string{"Authorization": "Token valid-token"},
			wantStatus: http.StatusCreated,
		},
		"auth token type is case insensitive": {
			headers:    map[string]string{"Authorization": "tOkEn valid-token"},
			wantStatus: http.StatusCreated,
		},
		"invalid api key rate limited": {
			headers:    map[string]string{"Authorization": "Token zzz-invalid-token"},
			rl:         newLimiter(0, 0),
			wantStatus: http.StatusTooManyRequests,
		},
		"unknown auth token type rate limited": {
			headers:    map[string]string{"Authorization": "Foo valid-token"},
			rl:         newLimiter(0, 0),
			wantStatus: http.StatusTooManyRequests,
		},
		"invalid auth token format rate limited": {
			headers:    map[string]string{"Authorization": "Token abc 123"},
			rl:         newLimiter(0, 0),
			wantStatus: http.StatusTooManyRequests,
		},
		"rate limit ok": {
			rl:         newLimiter(1, 1),
			wantStatus: http.StatusCreated,
		},
		"rate limit exceeded": {
			rl:         newLimiter(0, 0),
			wantStatus: http.StatusTooManyRequests,
		},
	}

	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(rateLimitHandler(authTokens, tc.rl, handler))

			req, err := http.NewRequest("GET", srv.URL, nil)
			assert.Nil(t, err)
			for key, val := range tc.headers {
				req.Header.Set(key, val)
			}

			resp, err := http.DefaultClient.Do(req)
			assert.Nil(t, err)
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}

func newLimiter(limit float64, burst int) *rate.Limiter {
	return rate.NewLimiter(rate.Limit(limit), burst)
}
