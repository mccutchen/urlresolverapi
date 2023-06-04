package middleware

import (
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"

	"github.com/felixge/httpsnoop"
	"github.com/honeycombio/beeline-go"
	"github.com/honeycombio/beeline-go/wrappers/hnynethttp"
	ctxdata "github.com/peterbourgon/ctxdata/v4"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// Wrap wraps an http handler with middleware to add instrumentation, error
// handling, authentication, and rate limiting.
func Wrap(h http.Handler, authMap AuthMap, rl *rate.Limiter, l zerolog.Logger) http.Handler {
	h = corsHandler(h)
	h = rateLimitHandler(h, rl)
	h = authHandler(h, authMap)
	h = panicHandler(h)
	h = observeHandler(h, l)
	h = hnynethttp.WrapHandler(h)
	return h
}

func observeHandler(next http.Handler, l zerolog.Logger) http.Handler {
	concurrentRequests := &atomic.Int64{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := beeline.StartSpan(r.Context(), "httphandler.middleware.observeHandler")
		defer span.Send()

		inflight := concurrentRequests.Add(1)
		defer concurrentRequests.Add(-1)
		beeline.AddField(ctx, "concurrent_requests", inflight)

		ctx, d := ctxdata.New(ctx)
		m := httpsnoop.CaptureMetrics(next, w, r.WithContext(ctx))

		rec := logRecord{
			ClientID:   d.GetString("client_id"),
			DurationMS: m.Duration.Milliseconds(),
			Method:     r.Method,
			RemoteAddr: getRemoteAddr(r),
			Size:       m.Written,
			Status:     m.Code,
			URL:        r.URL.String(),
			UserAgent:  r.Header.Get("User-Agent"),
		}
		if err := d.GetError("error"); err != nil {
			rec.Error = err.Error()
			beeline.AddField(ctx, "error", err)
			// stack might be added by panicHandler
			if stack := d.GetString("stack"); stack != "" {
				rec.Stack = stack
				beeline.AddField(ctx, "stack", stack)
			}
		}
		evt := l.Info()
		if rec.Status != http.StatusOK || rec.Error != "" {
			evt = l.Error()
		}
		evt.EmbedObject(rec).Send()
	})
}

func panicHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := beeline.StartSpan(r.Context(), "httphandler.middleware.panicHandler")
		defer span.Send()

		defer func() {
			if p := recover(); p != nil {

				buf := make([]byte, 2048)
				n := runtime.Stack(buf, false)
				stack := string(buf[:n])

				d := ctxdata.From(r.Context())
				_ = d.Set("error", fmt.Errorf("panic: %s", p))
				_ = d.Set("stack", stack)

				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getRemoteAddr(r *http.Request) string {
	remoteAddr := r.Header.Get("Fly-Client-IP")
	if remoteAddr == "" {
		remoteAddr = r.RemoteAddr
	}
	return remoteAddr
}

type logRecord struct {
	ClientID   string `json:"client_id"`
	DurationMS int64  `json:"duration_ms"`
	Method     string `json:"method"`
	RemoteAddr string `json:"remote_addr"`
	Size       int64  `json:"size"`
	Status     int    `json:"status"`
	URL        string `json:"url"`
	UserAgent  string `json:"user_agent"`

	// Only added when an error and/or a panic occurs
	Error string `json:"error,omitempty"`
	Stack string `json:"stack,omitempty"`
}

func (rec logRecord) MarshalZerologObject(e *zerolog.Event) {
	e.Int("status", rec.Status)
	e.Int64("duration_ms", rec.DurationMS)
	e.Int64("size", rec.Size)
	e.Str("method", rec.Method)
	e.Str("remote_addr", rec.RemoteAddr)
	e.Str("url", rec.URL)
	e.Str("user_agent", rec.UserAgent)
	e.Str("client_id", rec.ClientID)
	if rec.Error != "" {
		e.Str("error", rec.Error)
		if rec.Stack != "" {
			e.Str("stack", rec.Stack)
		}
	}
}

var _ zerolog.LogObjectMarshaler = logRecord{}
