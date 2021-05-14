package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestMiddleware(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		handler   http.HandlerFunc
		validator func(*testing.T, logRecord)
		wantCode  int
	}{
		"ok": {
			handler: func(w http.ResponseWriter, r *http.Request) {
				<-time.After(25 * time.Millisecond)
				w.WriteHeader(http.StatusCreated)
			},
			validator: func(t *testing.T, rec logRecord) {
				assert.Equal(t, http.StatusCreated, rec.Status)
				assert.GreaterOrEqual(t, rec.DurationMS, (25 * time.Millisecond).Milliseconds())
			},
			wantCode: http.StatusCreated,
		},
		"panics are caught and return HTTP 500": {
			handler: func(w http.ResponseWriter, r *http.Request) {
				panic("oops")
			},
			validator: func(t *testing.T, rec logRecord) {
				assert.Equal(t, http.StatusInternalServerError, rec.Status)
				assert.Equal(t, rec.Error, "panic: oops")
				assert.Contains(t, rec.Stack, "goroutine")
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			captured := &capturingWriter{}
			wrapped := Wrap(tc.handler, zerolog.New(captured))
			srv := httptest.NewServer(wrapped)
			defer srv.Close()

			resp, err := http.Get(srv.URL)
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, tc.wantCode, resp.StatusCode)
			if !assert.Len(t, captured.events, 1) {
				return
			}
			tc.validator(t, captured.events[0])
		})
	}
}

type capturingWriter struct {
	mu     sync.Mutex
	events []logRecord
}

func (w *capturingWriter) Write(data []byte) (int, error) {
	var evt logRecord
	if err := json.Unmarshal(data, &evt); err != nil {
		return 0, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, evt)

	return len(data), nil
}

var _ io.Writer = &capturingWriter{}
