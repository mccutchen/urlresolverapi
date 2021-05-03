# urlresolverapi

[![GoDoc](https://pkg.go.dev/badge/github.com/mccutchen/urlresolverapi)](https://pkg.go.dev/github.com/mccutchen/urlresolverapi)
[![Build status](https://github.com/mccutchen/urlresolverapi/actions/workflows/test.yaml/badge.svg)](https://github.com/mccutchen/urlresolverapi/actions/workflows/test.yaml)
[![Coverage](https://codecov.io/gh/mccutchen/urlresolverapi/branch/main/graph/badge.svg)](https://codecov.io/gh/mccutchen/urlresolverapi)

A golang HTTP server that "resolves" a URL into its canonical form by following
any redirects, normalizing query parameters, and attempting to fetch its title.

It is used by [Thresholderbot][] to resolve URLs found in tweets, which tend to
be wrapped in one or more URL shorteners (t.co, bit.ly, etc).

[Thresholderbot]: https://thresholderbot.com/
[purell]: https://github.com/PuerkitoBio/purell
[blog]: https://www.agwa.name/blog/post/preventing_server_side_request_forgery_in_golangs
