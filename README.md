# urlresolverapi

[![GoDoc](https://pkg.go.dev/badge/github.com/mccutchen/urlresolverapi)](https://pkg.go.dev/github.com/mccutchen/urlresolverapi)
[![Build status](https://github.com/mccutchen/urlresolverapi/actions/workflows/test.yaml/badge.svg)](https://github.com/mccutchen/urlresolverapi/actions/workflows/test.yaml)
[![Coverage](https://codecov.io/gh/mccutchen/urlresolverapi/branch/main/graph/badge.svg)](https://codecov.io/gh/mccutchen/urlresolverapi)

A golang HTTP server that uses [github.com/mccutchen/urlresolver][pkg] to
"resolve" a URL into its canonical form by following any redirects, normalizing
query parameters, and attempting to fetch its title.

It is used by [Thresholderbot][] to resolve URLs found in tweets, which tend to
be wrapped in one or more URL shorteners (t.co, bit.ly, etc).

A publicly-available instance is available at https://api.urlresolver.com,
(deployed on [fly.io](https://fly.io)).

## API

There is a single API endpoint, `/resolve`, seen here resolving a t.co URL that redirects a
few times before ending up at the New York Times:

```
GET https://api.urlresolver.com/resolve?url=https://t.co/1AuEh8FMK0?amp=1

{
  "given_url": "https://t.co/1AuEh8FMK0?amp=1",
  "resolved_url": "https://www.nytimes.com/2021/08/25/style/lil-nas-x.html",
  "title": "Some Said Lil Nas X Was a One-Hit Wonder. They Were Wrong. - The New York Times",
  "intermediate_urls": [
    "https://t.co/1AuEh8FMK0?amp=1",
    "https://nyti.ms/3BlpRIR",
    "https://trib.al/4OR3gtI"
  ]
}
```

A `200 OK` status code means that the API successfully resolved the URL and
extracted a title.

A `203 Non-Authoritative Information` status code means that the API ran into
an error while resolving the URL and/or extracting a title (e.g. a request
timeout). In this case, the API returns as much information as it can; it will
at least return a normalized/canonicalized and potentially partially-resolved
`resolved_url` value.


## 🔒 Access control

Because this server can be used to generate load on arbitrary other web sites,
it should not be deployed without some form of authentication or rate limiting
in place.

**👉 TL;DR**:
- The server allows both authenticated and unauthenticated requests by default
- Rate limits are applied only to unauthenticated requests
- To require authentication for all requests, set the rate limit to `0`

### Authentication

We currently support an extremely simple token-based authentication mechanism.
Specify valid tokens as a list of comma-separated `client-id:token-value`
pairs:

```bash
AUTH_TOKENS="client-a:abcd1234,client-a:efgh56789,client-b:1234abcd"
```

(This format allows for token rotation while keeping the client ID consistent
for observability purposes.)

Authentication information is used to determine whether or not to rate limit
requests, and recorded in the server's instrumentation to identify known
clients.

### Rate limiting

Rate limits are applied per-instance, using an in-process token bucket rate
limiting implementation. So, if you set the rate limit to 10 req/sec and scale
the server up to 5 instances, the effective rate limit for will be 50 req/sec.

Rate limits are applied only to unauthenticated clients, and all
unauthenticated clients share one global rate limit.


## Configuration

This server can be configured via CLI arguments or environment variables. See
the `--help` output for all of the configurable parameters:

```
urlresolverapi --help
Usage of urlresolverapi:
  -auth-tokens string
      Comma-separated list of valid auth tokens in "client-id:token-value" format for which rate limiting is disabled
  -burst-limit int
      Allowed bursts over rate limit (if rate limit >= 0) (default 2)
  -cache-ttl duration
      TTL for cached results (if caching enabled) (default 120h0m0s)
  -client-patience duration
      How long to wait for slow clients to write requests or read responses (default 1s)
  -debug-port int
      Port on which to expose pprof/expvar debugging endpoints (disabled if == 0) (default 6060)
  -honeycomb-api-key string
      Honeycomb API key (enables sending telemetry data to honeycomb)
  -honeycomb-dataset string
      Honeycomb dataset for telemetry data (default "urlresolverapi")
  -honeycomb-sample-rate uint
      Sample rate for telemetry data (1/N events will be submitted) (default 1)
  -honeycomb-service-name string
      Service name for telemetry data (default "urlresolverapi")
  -idle-cx-ttl duration
      TTL for idle connections (default 1m30s)
  -max-idle-cx-per-host int
      Max idle connections per host (default 10)
  -port int
      Port to listen on (default 8080)
  -rate-limit float
      Per-second rate limit for anonymous clients (disabled if <= 0)
  -redis-timeout duration
      Timeout for redis operations (if caching enabled) (default 150ms)
  -redis-url string
      Redis connection URL (enables caching)
  -request-timeout duration
      Overall timeout on a single resolve request, including any redirects (default 10s)
```


## Profiling notes

The app exposes Go's [expvar][] and [net/http/pprof][pprof] endpoints on port
`6060`, which should not be exposed to the outside world.

When deployed on [fly.io], you can access those endpoints via a secure
WireGuard VPN connection.  First, follow their [Private Network VPN setup guide][vpn].

With that VPN enabled, you can now access the internal endpoints by hostname
(which will connect to 1/N instances of the app):

```
go tool pprof appname.internal:6060/debug/pprof/allocs
```

Or you can connect to a specific instance by first getting its private IPv6
address and then connecting directly to it:

```
$ dig +short aaaa appname.internal
fdaa:0:2530:a7b:ab2:5f9e:d6b1:2
fdaa:0:2530:a7b:ab3:97d6:9ae8:2

$ go tool pprof '[fdaa:0:2530:a7b:ab2:5f9e:d6b1:2]:6060/debug/pprof/allocs'
```


[pkg]: https://github.com/mccutchen/urlresolver
[Thresholderbot]: https://thresholderbot.com/
[purell]: https://github.com/PuerkitoBio/purell
[blog]: https://www.agwa.name/blog/post/preventing_server_side_request_forgery_in_golangs
[expvar]: https://golang.org/pkg/expvar/
[pprof]: https://golang.org/pkg/net/http/pprof/
[fly.io]: https://fly.io/
[vpn]: https://fly.io/docs/reference/privatenetwork/#private-network-vpn
