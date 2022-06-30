package coalesced

import (
	"context"
	"net/url"

	"github.com/honeycombio/beeline-go"
	"golang.org/x/sync/singleflight"

	"github.com/mccutchen/urlresolver"
)

// Resolver is a urlresolver.Interface implementation that coalesces concurrent
// requests to upstream URLs.
type Resolver struct {
	group    *singleflight.Group
	resolver urlresolver.Interface
}

// New creates a new coalesced Resolver.
func New(resolver urlresolver.Interface) *Resolver {
	return &Resolver{
		group:    &singleflight.Group{},
		resolver: resolver,
	}
}

// Resolve resolves a URL if it is not already cached.
func (c *Resolver) Resolve(ctx context.Context, givenURL string) (urlresolver.Result, error) {
	// A bit wasteful to canonicalize the URL here since the wrapped resolver
	// will do the same thing, but it should slightly improve our chances of
	// coalescing requests.
	canonicalURL, err := canonicalize(givenURL)
	if err != nil {
		return urlresolver.Result{}, err
	}

	result, err, shared := c.group.Do(canonicalURL, func() (interface{}, error) {
		return c.resolver.Resolve(ctx, canonicalURL)
	})

	beeline.AddField(ctx, "resolver.request_coalesced", shared)
	return result.(urlresolver.Result), err
}

func canonicalize(givenURL string) (string, error) {
	parsedURL, err := url.Parse(givenURL)
	if err != nil {
		return "", err
	}
	return urlresolver.Canonicalize(parsedURL), nil
}
