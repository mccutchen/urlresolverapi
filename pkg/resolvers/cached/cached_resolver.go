package cached

import (
	"context"

	"github.com/honeycombio/beeline-go"

	"github.com/mccutchen/urlresolver"
)

// Resolver is a Resolver implementation that caches its results.
type Resolver struct {
	cache    Cache
	resolver urlresolver.Interface
}

// NewResolver creates a new cached Resolver.
func NewResolver(resolver urlresolver.Interface, cache Cache) *Resolver {
	return &Resolver{
		cache:    cache,
		resolver: resolver,
	}
}

// Resolve resolves a URL if it is not already cached.
func (c *Resolver) Resolve(ctx context.Context, url string) (urlresolver.Result, error) {
	beeline.AddField(ctx, "resolver.cache_name", c.cache.Name())

	if result, ok := c.cache.Get(ctx, url); ok {
		beeline.AddField(ctx, "resolver.cache_result", "hit")
		return result, nil
	}

	result, err := c.resolver.Resolve(ctx, url)
	if err == nil {
		c.cache.Add(ctx, url, result)
	}

	beeline.AddField(ctx, "resolver.cache_result", "miss")
	return result, err
}
