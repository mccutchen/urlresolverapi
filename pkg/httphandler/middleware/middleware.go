package middleware

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/felixge/httpsnoop"
	"github.com/honeycombio/beeline-go"
	"github.com/honeycombio/beeline-go/wrappers/hnynethttp"
	ctxdata "github.com/peterbourgon/ctxdata/v4"
	"github.com/rs/zerolog"
)

func Wrap(h http.Handler, rlc RateLimitConfig, l zerolog.Logger) http.Handler {
	if rlc.Limiter != nil {
		h = rateLimitHandler(rlc.AuthTokens, rlc.Limiter, h)
	}
	h = panicHandler(h)
	h = observeHandler(h, l)
	h = hnynethttp.WrapHandler(h)
	return h
}

func observeHandler(next http.Handler, l zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, d := ctxdata.New(r.Context())
		m := httpsnoop.CaptureMetrics(next, w, r.WithContext(ctx))

		rec := logRecord{
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
			if stack := d.GetString("stack"); err != nil {
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
		next.ServeHTTP(w, r)
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
	if rec.Error != "" {
		e.Str("error", rec.Error)
		if rec.Stack != "" {
			e.Str("stack", rec.Stack)
		}
	}
}

var _ zerolog.LogObjectMarshaler = logRecord{}
