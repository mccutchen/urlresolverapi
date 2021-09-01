package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthHandler(t *testing.T) {
	t.Parallel()

	authMap := map[string]string{
		"valid-token": "client-1",
	}

	testCases := map[string]struct {
		headers      map[string]string
		wantClientID string
		wantStatus   int
	}{
		"valid auth token accepted": {
			headers:      map[string]string{"Authorization": "Token valid-token"},
			wantStatus:   http.StatusOK,
			wantClientID: "client-1",
		},
		"auth token type is case insensitive": {
			headers:      map[string]string{"Authorization": "tOkEn valid-token"},
			wantStatus:   http.StatusOK,
			wantClientID: "client-1",
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
			srv := httptest.NewServer(authHandler(h, authMap))

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

func TestParseAuthMap(t *testing.T) {
	testCases := map[string]struct {
		input   string
		want    AuthMap
		wantErr error
	}{
		"ok": {
			input: "client-1:token-1,  client-1:token-2 ,   client-2 : token-3  , client-3:token-4",
			want: AuthMap{
				"token-1": "client-1",
				"token-2": "client-1",
				"token-3": "client-2",
				"token-4": "client-3",
			},
		},
		"empty ok": {
			input: "",
			want:  nil,
		},
		"duplicate tokens not allowed": {
			input:   "client-1:token-1,client-2:token-1",
			wantErr: errors.New("duplicate auth token value \"token-1\""),
		},
		"empty client ID not allowed": {
			input:   ":token-1",
			wantErr: errors.New("auth token \":token-1\" has empty client ID"),
		},
		"empty tokens not allowed": {
			input:   "client-1:",
			wantErr: errors.New("auth token value in \"client-1:\" cannot be empty or contain spaces"),
		},
		"tokens with spaces not allowed": {
			input:   "client-1:foo bar",
			wantErr: errors.New("auth token value in \"client-1:foo bar\" cannot be empty or contain spaces"),
		},
		"invalid token format": {
			input:   "client-1/token-1",
			wantErr: errors.New("invalid token format \"client-1/token-1\", token must be in \"client-id:token-value\" format"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseAuthMap(tc.input)
			assert.Equal(t, tc.wantErr, err)
			assert.Equal(t, tc.want, got)
		})
	}

}
