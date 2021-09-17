package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCorsHandler(t *testing.T) {
	testCases := map[string]struct {
		reqHeaders  map[string]string
		respHeaders map[string]string
	}{
		"no origin, no problem": {
			respHeaders: map[string]string{
				"Access-Control-Allow-Origin": "",
				"Vary":                        "",
			},
		},
		"origin respected": {
			reqHeaders: map[string]string{
				"Origin": "https://foo.com",
			},
			respHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://foo.com",
				"Vary":                        "Origin",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			handler := corsHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			for k, v := range tc.reqHeaders {
				r.Header.Set(k, v)
			}
			handler.ServeHTTP(w, r)
			for key, wantValue := range tc.respHeaders {
				gotValue := w.Header().Get(key)
				assert.Equal(t, wantValue, gotValue, "wrong header value for key %s", key)
			}
		})
	}
}
