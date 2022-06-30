//nolint:errcheck
package coalesced

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mccutchen/urlresolver"
)

func TestSingleFlightResolver(t *testing.T) {
	t.Parallel()

	var counter int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&counter, 1)
		<-time.After(25 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>title</title></head></html>`))
	}))
	defer srv.Close()

	resolver := New(urlresolver.New(http.DefaultTransport, 0))

	wantResult := urlresolver.Result{
		Title:       "title",
		ResolvedURL: srv.URL,
	}

	// Make 5 concurrent requests, only one should hit the upstream server
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := resolver.Resolve(context.Background(), srv.URL)
			assert.NoError(t, err)
			assert.Equal(t, wantResult, result)
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), counter, "expected only 1 total request to upstream")
}

func TestSingleFlightResolverInvalidURL(t *testing.T) {
	t.Parallel()
	resolver := New(urlresolver.New(http.DefaultTransport, 0))
	result, err := resolver.Resolve(context.Background(), "%%")
	assert.NotNil(t, err)
	assert.Equal(t, urlresolver.Result{}, result)
}
