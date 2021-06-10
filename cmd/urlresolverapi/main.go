package main

import (
	"context"
	_ "expvar" // register expvar w/ default handler, only exposed over private network
	"net"
	"net/http"
	_ "net/http/pprof" // register pprof endpoints w/ default handler, only exposed over private network
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/cache/v8"
	"github.com/go-redis/redis/v8"
	beeline "github.com/honeycombio/beeline-go"
	"github.com/rs/zerolog"

	"github.com/mccutchen/urlresolver"
	"github.com/mccutchen/urlresolver/fakebrowser"
	"github.com/mccutchen/urlresolver/safedialer"
	"github.com/mccutchen/urlresolverapi/pkg/cachedresolver"
	"github.com/mccutchen/urlresolverapi/pkg/httphandler"
	"github.com/mccutchen/urlresolverapi/pkg/httphandler/middleware"
	"github.com/mccutchen/urlresolverapi/pkg/tracetransport"
)

const (
	cacheTTL    = 120 * time.Hour
	defaultPort = "8080"

	// How long we will wait for a client to write its request or read our
	// response.
	clientPatience = 2 * time.Second

	// requestTimeout sets an overall timeout on a single resolve request,
	// including any redirects that must be followed and any time spent in DNS
	// lookup, tcp connect, tls handshake, etc.
	requestTimeout = 10 * time.Second

	// shutdownTimeout is just a bit longer than we expect the longest
	// individual request we're handling to take.
	shutdownTimeout = requestTimeout + clientPatience

	// dialTimeout determines how long we'll wait to make a connection to a
	// remote host.
	dialTimeout = 2 * time.Second

	// server timeouts prevent slow/malicious clients from occupying resources
	// for too long.
	serverReadTimeout  = clientPatience
	serverWriteTimeout = requestTimeout + clientPatience

	// configure our http client to reuse connections somewhat aggressively.s
	transportIdleConnTimeout     = 90 * time.Second
	transportMaxIdleConnsPerHost = 100
	transportTLSHandshakeTimeout = 2 * time.Second

	// redis timeout
	redisTimeout = 150 * time.Millisecond

	// pprof listener will be exposed on this address, which should not be
	// available on the public internet
	pprofAddr = ":6060"
)

func main() {
	logger, cleanup := initTelemetry()
	defer cleanup()

	resolver := initResolver(logger)
	handler := httphandler.New(resolver)

	mux := http.NewServeMux()
	mux.Handle("/resolve", handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	// Spin up the default mux to expose expvar and pprof endpoints on an
	// internal-only port. Use `flyctl wg` to create a wireguard tunnel that
	// will allow direct access to these pprof endpoints.
	//
	// E.g. go tool pprof urlresolverapi-production.internal:6060/debug/pprof/allocs
	go func() {
		logger.Info().Msgf("debug endpoints available on %s", pprofAddr)
		if err := http.ListenAndServe(pprofAddr, nil); err != http.ErrServerClosed {
			logger.Error().Msgf("error serving debug endpoints: %s", err)
		}
	}()

	srv := &http.Server{
		Addr:         net.JoinHostPort("", port),
		Handler:      middleware.Wrap(mux, logger),
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
	}

	listenAndServeGracefully(srv, shutdownTimeout, logger)
}

func listenAndServeGracefully(srv *http.Server, shutdownTimeout time.Duration, logger zerolog.Logger) {
	// exitCh will be closed when it is safe to exit, after the server has had
	// a chance to shut down gracefully
	exitCh := make(chan struct{})

	go func() {
		// wait for SIGTERM or SIGINT
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigCh

		// start graceful shutdown
		logger.Info().Msgf("shutdown started by signal: %s", sig)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error().Err(err).Msg("shutdown error")
		}

		// indicate that it is now safe to exit
		close(exitCh)
	}()

	// start server
	logger.Info().Msgf("listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error().Err(err).Msg("listen error")
	}

	// wait until it is safe to exit
	<-exitCh
}

func initResolver(logger zerolog.Logger) urlresolver.Interface {
	transport := fakebrowser.New(tracetransport.New(&http.Transport{
		DialContext: (&net.Dialer{
			Control: safedialer.Control,
			Timeout: dialTimeout,
		}).DialContext,
		IdleConnTimeout:     transportIdleConnTimeout,
		MaxIdleConnsPerHost: transportMaxIdleConnsPerHost,
		MaxIdleConns:        transportMaxIdleConnsPerHost * 2,
		TLSHandshakeTimeout: transportTLSHandshakeTimeout,
	}))

	var r urlresolver.Interface = urlresolver.New(transport, requestTimeout)

	// Wrap resolver with a redis cache if we successfully connect to redis
	if redisCache := initRedisCache(logger); redisCache != nil {
		r = cachedresolver.NewCachedResolver(r, cachedresolver.NewRedisCache(redisCache, cacheTTL))
	}

	return r
}

func initRedisCache(logger zerolog.Logger) *cache.Cache {
	redisURL := os.Getenv("FLY_REDIS_CACHE_URL")
	if redisURL == "" {
		logger.Info().Msg("set FLY_REDIS_CACHE_URL to enable caching")
		return nil
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Error().Err(err).Msg("FLY_REDIS_CACHE_URL invalid, cache disabled")
		return nil
	}

	opt.DialTimeout = 2 * redisTimeout
	opt.ReadTimeout = redisTimeout
	opt.WriteTimeout = redisTimeout

	return cache.New(&cache.Options{Redis: redis.NewClient(opt)})
}

func initTelemetry() (zerolog.Logger, func()) {
	var (
		apiKey      = os.Getenv("HONEYCOMB_API_KEY")
		serviceName = os.Getenv("FLY_APP_NAME")
	)

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	if apiKey == "" {
		logger.Info().Msg("set HONEYCOMB_API_KEY to capture telemetry")
		return logger, func() {}
	}
	if serviceName == "" {
		serviceName = "urlresolver"
	}

	beeline.Init(beeline.Config{
		Dataset:     "urlresolver",
		ServiceName: serviceName,
		WriteKey:    apiKey,
		SampleRate:  4, // submit 25% or 1/4 events
	})
	return logger, beeline.Close
}
