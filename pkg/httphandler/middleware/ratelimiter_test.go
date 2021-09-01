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

	authMap := map[string]string{
		"valid-token": "client-1",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	testCases := map[string]struct {
		rl         *rate.Limiter
		headers    map[string]string
		wantStatus int
	}{
		"valid auth token not rate limited": {
			headers:    map[string]string{"Authorization": "Token valid-token"},
			rl:         newLimiter(0, 0), // would reject request unless rate limiting explicitly skipped
			wantStatus: http.StatusCreated,
		},
		"nil rate limiter skips rate limiting": {
			rl:         nil,
			wantStatus: http.StatusCreated,
		},
		"anonymous request rate limit ok": {
			rl:         newLimiter(1, 1),
			wantStatus: http.StatusCreated,
		},
		"anonymous request rate limit exceeded": {
			rl:         newLimiter(0, 0),
			wantStatus: http.StatusTooManyRequests,
		},
	}

	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(
				authHandler(
					rateLimitHandler(handler, tc.rl),
					authMap,
				),
			)

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
