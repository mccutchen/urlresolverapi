module github.com/mccutchen/urlresolverapi

go 1.16

require (
	github.com/alicebob/miniredis/v2 v2.14.3
	github.com/go-redis/cache/v8 v8.4.0
	github.com/go-redis/redis/v8 v8.8.2
	github.com/honeycombio/beeline-go v1.0.0
	github.com/mccutchen/urlresolver v0.1.0
	github.com/rs/zerolog v1.21.0
	github.com/stretchr/testify v1.7.0
	golang.org/x/net v0.0.0-20210501142056-aec3718b3fa0 // indirect
)

// https://github.com/honeycombio/beeline-go/pull/216
replace github.com/honeycombio/beeline-go v1.0.0 => github.com/mccutchen/beeline-go v1.0.1
