# urlresolverapi

[![GoDoc](https://pkg.go.dev/badge/github.com/mccutchen/urlresolverapi)](https://pkg.go.dev/github.com/mccutchen/urlresolverapi)
[![Build status](https://github.com/mccutchen/urlresolverapi/actions/workflows/test.yaml/badge.svg)](https://github.com/mccutchen/urlresolverapi/actions/workflows/test.yaml)
[![Coverage](https://codecov.io/gh/mccutchen/urlresolverapi/branch/main/graph/badge.svg)](https://codecov.io/gh/mccutchen/urlresolverapi)

A golang HTTP server that uses [github.com/mccutchen/urlresolver][pkg] to
"resolve" a URL into its canonical form by following any redirects, normalizing
query parameters, and attempting to fetch its title.

It is used by [Thresholderbot][] to resolve URLs found in tweets, which tend to
be wrapped in one or more URL shorteners (t.co, bit.ly, etc).

## API

There is a single API endpoint, `/resolve`, seen here resolving a t.co URL that redirects a
few times before ending up at the New York Times:

```
GET /resolve?url=https://t.co/1AuEh8FMK0?amp=1

{
  "given_url": "https://t.co/1AuEh8FMK0?amp=1",
  "resolved_url": "https://www.nytimes.com/2021/08/25/style/lil-nas-x.html",
  "title": "Some Said Lil Nas X Was a One-Hit Wonder. They Were Wrong. - The New York Times"
}
```

A `200 OK` status code means that the API successfully resolved the URL and
extracted a title.

A `203 Non-Authoritative Information` status code means that the API ran into
an error while resolving the URL and/or extracting a title (e.g. a request
timeout). In this case, the API returns as much information as it can; it will
at least return a normalized/canonicalized and potentially partially-resolved
`resolved_url` value.


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
