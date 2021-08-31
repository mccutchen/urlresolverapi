package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthHandler(t *testing.T) {
	t.Parallel()

	authTokens := []string{"valid-token"}

	testCases := map[string]struct {
		headers      map[string]string
		wantClientID string
		wantStatus   int
	}{
		"valid auth token accepted": {
			headers:      map[string]string{"Authorization": "Token valid-token"},
			wantStatus:   http.StatusOK,
			wantClientID: "ab73a3eaca01a7059dcdff6f95556ec7fd83de96", // hash of "valid-token"
		},
		"auth token type is case insensitive": {
			headers:      map[string]string{"Authorization": "tOkEn valid-token"},
			wantStatus:   http.StatusOK,
			wantClientID: "ab73a3eaca01a7059dcdff6f95556ec7fd83de96", // hash of "valid-token"
		},
		"invalid auth token rejected": {
			headers:    map[string]string{"Authorization": "Token zzz-invalid-token"},
			wantStatus: http.StatusForbidden,
		},
		"unknown auth token type rejected": {
			headers:    map[string]string{"Authorization": "Foo valid-token"},
			wantStatus: http.StatusForbidden,
		},
		"invalid auth token format rejected": {
			headers:    map[string]string{"Authorization": "Token abc 123"},
			wantStatus: http.StatusForbidden,
		},
		"anonymous request allowed": {
			wantStatus:   http.StatusOK,
			wantClientID: "",
		},
	}

	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotClientID := clientIDFromContext(r.Context())
				assert.Equal(t, tc.wantClientID, gotClientID)
			})
			srv := httptest.NewServer(authHandler(h, authTokens))

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
