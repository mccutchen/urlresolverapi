package middleware

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/honeycombio/beeline-go"
	"github.com/honeycombio/beeline-go/wrappers/hnynethttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func Wrap(h http.Handler, l zerolog.Logger) http.Handler {
	h = hlog.AccessHandler(accessLogger)(h)
	h = hlog.NewHandler(l)(h)
	h = panicHandler(h)
	h = hnynethttp.WrapHandler(h)
	return h
}

func panicHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				buf := make([]byte, 2048)
				n := runtime.Stack(buf, false)
				stack := string(buf[:n])
				msg := fmt.Sprintf("panic: %s", p)
				ctx := r.Context()

				// l.Error().Str("stack", stack).Msg(msg)
				beeline.AddField(ctx, "error", msg)
				beeline.AddField(ctx, "stack", stack)

				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func accessLogger(r *http.Request, status int, size int, duration time.Duration) {
	remoteAddr := r.Header.Get("Fly-Client-IP")
	if remoteAddr == "" {
		remoteAddr = r.RemoteAddr
	}

	hlog.FromRequest(r).Info().
		Str("method", r.Method).
		Str("remote_addr", remoteAddr).
		Stringer("url", r.URL).
		Int("status", status).
		Int("size", size).
		Dur("duration", duration).
		Send()
}
