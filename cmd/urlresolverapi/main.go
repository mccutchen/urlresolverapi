package main

import (
	"context"
	_ "expvar" // register expvar w/ default handler, only exposed over private network
	"flag"
	"net"
	"net/http"
	_ "net/http/pprof" // register pprof endpoints w/ default handler, only exposed over private network
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-redis/cache/v8"
	"github.com/go-redis/redis/v8"
	beeline "github.com/honeycombio/beeline-go"
	"github.com/peterbourgon/ff/v3"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"

	"github.com/mccutchen/urlresolver"
	"github.com/mccutchen/urlresolver/fakebrowser"
	"github.com/mccutchen/urlresolver/safedialer"
	"github.com/mccutchen/urlresolverapi/pkg/httphandler"
	"github.com/mccutchen/urlresolverapi/pkg/httphandler/middleware"
	"github.com/mccutchen/urlresolverapi/pkg/resolvers/cached"
	"github.com/mccutchen/urlresolverapi/pkg/resolvers/coalesced"
	"github.com/mccutchen/urlresolverapi/pkg/tracetransport"
)

func main() {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	// Rewrite some platform-specific env vars before parsing configuration
	envRewrites := map[string]string{
		"FLY_REDIS_CACHE_URL": "REDIS_URL",
		"FLY_APP_NAME":        "HONEYCOMB_SERVICE_NAME",
	}
	for srcKey, dstKey := range envRewrites {
		if os.Getenv(srcKey) != "" && os.Getenv(dstKey) == "" {
			os.Setenv(dstKey, os.Getenv(srcKey))
		}
	}

	fs := flag.NewFlagSet("urlresolverapi", flag.ExitOnError)
	var (
		port      = fs.Int("port", 8080, "Port to listen on")
		debugPort = fs.Int("debug-port", 6060, "Port on which to expose pprof/expvar debugging endpoints (disabled if == 0)")

		authTokens = fs.String("auth-tokens", "", "Comma-separated list of valid auth tokens in \"client-id:token-value\" format for which rate limiting is disabled")
		rateLimit  = fs.Float64("rate-limit", 10, "Per-second, per-instance rate limit for anonymous clients (use 0 to disable anonymous requests)")
		burstLimit = fs.Int("burst-limit", 2, "Allowed bursts over rate limit (if rate limit >= 0)")

		requestTimeout = fs.Duration("request-timeout", 10*time.Second, "Overall timeout on a single resolve request, including any redirects")
		clientPatience = fs.Duration("client-patience", 1*time.Second, "How long to wait for slow clients to write requests or read responses")

		redisURL     = fs.String("redis-url", "", "Redis connection URL (enables caching)")
		redisTimeout = fs.Duration("redis-timeout", 150*time.Millisecond, "Timeout for redis operations (if caching enabled)")
		cacheTTL     = fs.Duration("cache-ttl", 120*time.Hour, "TTL for cached results (if caching enabled)")

		honeycombAPIKey      = fs.String("honeycomb-api-key", "", "Honeycomb API key (enables sending telemetry data to honeycomb)")
		honeycombDataset     = fs.String("honeycomb-dataset", "urlresolverapi", "Honeycomb dataset for telemetry data")
		honeycombServiceName = fs.String("honeycomb-service-name", "urlresolverapi", "Service name for telemetry data")
		honeycombSampleRate  = fs.Uint("honeycomb-sample-rate", 1, "Sample rate for telemetry data (1/N events will be submitted)")

		transportIdleConnTTL         = fs.Duration("idle-cx-ttl", 90*time.Second, "TTL for idle connections")
		transportMaxIdleConnsPerHost = fs.Int("max-idle-cx-per-host", 10, "Max idle connections per host")
	)
	if err := ff.Parse(fs, os.Args[1:], ff.WithEnvVarNoPrefix()); err != nil {
		logger.Fatal().Msgf("error parsing configuration: %s", err)
	}

	authMap, err := middleware.ParseAuthMap(*authTokens)
	if err != nil {
		logger.Fatal().Msgf("error parsing auth tokens: %s", err)
	}

	var (
		shutdownTimeout    = *requestTimeout + *clientPatience
		serverReadTimeout  = *clientPatience
		serverWriteTimeout = shutdownTimeout
	)

	if *debugPort <= 0 {
		// Use the default mux to expose expvar and pprof endpoints on an
		// internal-only port.
		//
		// If deployed on fly.io, use `flyctl wg` to create a wireguard tunnel that
		// will allow direct access to these pprof endpoints, via something like:
		//
		//     go tool pprof <app>.internal:6060/debug/pprof/allocs
		//
		// See fly.io's Private Networking docs for more context:
		// https://fly.io/docs/reference/privatenetwork/#private-network-vpn
		go func() {
			debugAddr := net.JoinHostPort("", strconv.Itoa(*debugPort))
			logger.Info().Msgf("debug endpoints available on %s", debugAddr)
			if err := http.ListenAndServe(debugAddr, nil); err != http.ErrServerClosed {
				logger.Error().Msgf("error serving debug endpoints: %s", err)
			}
		}()
	} else {
		logger.Info().Msg("set DEBUG_PORT to enable internal debug endpoints")
	}

	// set up optional telemetry
	if *honeycombAPIKey != "" {
		beeline.Init(beeline.Config{
			Dataset:     *honeycombDataset,
			ServiceName: *honeycombServiceName,
			WriteKey:    *honeycombAPIKey,
			SampleRate:  *honeycombSampleRate,
		})
		defer beeline.Close()
	} else {
		logger.Info().Msg("set HONEYCOMB_API_KEY to capture telemetry")
	}

	// set up transport used by resolver
	transport := fakebrowser.New(tracetransport.New(&http.Transport{
		DialContext: (&net.Dialer{
			Control: safedialer.Control,
		}).DialContext,
		IdleConnTimeout:     *transportIdleConnTTL,
		MaxIdleConnsPerHost: *transportMaxIdleConnsPerHost,
		MaxIdleConns:        *transportMaxIdleConnsPerHost * 2,
	}))

	// set up resolver w/ optional redis caching
	var resolver urlresolver.Interface = urlresolver.New(transport, *requestTimeout)
	if *redisURL != "" {
		// set up optional redis cache
		opt, err := redis.ParseURL(*redisURL)
		if err == nil {
			opt.DialTimeout = *redisTimeout * 2
			opt.ReadTimeout = *redisTimeout
			opt.WriteTimeout = *redisTimeout
			redisCache := cache.New(&cache.Options{Redis: redis.NewClient(opt)})
			resolver = cached.NewResolver(resolver, cached.NewRedisCache(redisCache, *cacheTTL))
		} else {
			logger.Error().Err(err).Msg("REDIS_URL invalid, cache disabled")
		}
	} else {
		logger.Info().Msg("set REDIS_URL to enable caching")
	}

	// ensure that concurrent requests are coalesced, regardless of whether
	// they're cached or not
	resolver = coalesced.New(resolver)

	// configure per-instance rate limiting
	rl := rate.NewLimiter(rate.Limit(*rateLimit), *burstLimit)

	mux := http.NewServeMux()
	mux.Handle("/resolve", httphandler.New(resolver))

	srv := &http.Server{
		Handler:      middleware.Wrap(mux, authMap, rl, logger),
		Addr:         net.JoinHostPort("", strconv.Itoa(*port)),
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
