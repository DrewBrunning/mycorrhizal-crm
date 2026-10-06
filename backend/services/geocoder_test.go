package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/config"
	"mycorrhizal/models"
)

// fakeGeocoder is a Geocoder backed by an httptest server that answers every
// nominatim search with the coordinate in body and counts the requests.
func fakeGeocoder(t *testing.T, handler http.HandlerFunc) (*Geocoder, *int32, *geocodeCache) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	cache := newGeocodeCache(8, time.Hour, time.Now)
	return &Geocoder{client: nominatimClient(srv.URL), cache: cache}, &hits, cache
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`[{"lat":"51.5007292","lon":"-0.1246254"}]`))
}

func TestNewGeocoder(t *testing.T) {
	for _, provider := range []string{"", config.GeocoderProviderNone} {
		g, err := NewGeocoder(&config.Config{GeocoderProvider: provider})
		require.NoError(t, err)
		assert.False(t, g.Enabled(), "provider %q is the default-off state", provider)
		assert.Nil(t, g.client, "a disabled geocoder owns no client, so nothing can dial out")
	}

	g, err := NewGeocoder(&config.Config{GeocoderProvider: config.GeocoderProviderNominatim})
	require.NoError(t, err)
	assert.True(t, g.Enabled())
	assert.NotNil(t, g.cache)

	g, err = NewGeocoder(&config.Config{GeocoderProvider: config.GeocoderProviderMapTiler, GeocoderAPIKey: "k"})
	require.NoError(t, err)
	assert.True(t, g.Enabled())

	_, err = NewGeocoder(&config.Config{GeocoderProvider: config.GeocoderProviderMapTiler})
	assert.ErrorIs(t, err, ErrGeocoderUnauthorized, "maptiler without a key is refused at construction")
}

func TestGeocoder_DisabledNeverGeocodes(t *testing.T) {
	var nilGeocoder *Geocoder
	assert.False(t, nilGeocoder.Enabled())

	for name, g := range map[string]*Geocoder{"nil": nilGeocoder, "none": {}} {
		_, _, err := g.GeocodeAddress(context.Background(), 1, models.ContactAddress{Street: "1 Main St"})
		assert.ErrorIs(t, err, ErrGeocoderDisabled, name)
	}
}

func TestGeocoder_GeocodeAddressFormatsAndCaches(t *testing.T) {
	g, hits, cache := fakeGeocoder(t, okHandler)
	addr := models.ContactAddress{Street: "Westminster", City: "London", Country: "UK"}

	uri, cached, err := g.GeocodeAddress(context.Background(), 1, addr)
	require.NoError(t, err)
	assert.Equal(t, "geo:51.500729,-0.124625", uri, "rounded to 6 decimals, as a geo: URI")
	assert.False(t, cached)

	uri2, cached, err := g.GeocodeAddress(context.Background(), 1, addr)
	require.NoError(t, err)
	assert.Equal(t, uri, uri2)
	assert.True(t, cached, "the repeat is answered from memory")
	assert.EqualValues(t, 1, atomic.LoadInt32(hits), "and never reaches the provider")
	assert.Equal(t, 1, cache.len())
}

func TestGeocoder_CacheKeySharesEquivalentSpellings(t *testing.T) {
	g, hits, _ := fakeGeocoder(t, okHandler)
	ctx := context.Background()

	_, _, err := g.GeocodeAddress(ctx, 1, models.ContactAddress{Street: "1 Main St", City: "Springfield"})
	require.NoError(t, err)
	_, cached, err := g.GeocodeAddress(ctx, 1, models.ContactAddress{Street: "  1 MAIN   st ", City: "springfield"})
	require.NoError(t, err)
	assert.True(t, cached, "case and whitespace differences share one entry")
	assert.EqualValues(t, 1, atomic.LoadInt32(hits))

	// PO box / apartment / floor are not sent and so do not split the key.
	_, cached, err = g.GeocodeAddress(ctx, 1, models.ContactAddress{Street: "1 Main St", City: "Springfield", Apartment: "4B", Floor: "2", POBox: "99"})
	require.NoError(t, err)
	assert.True(t, cached)

	// A genuinely different address is a miss.
	_, cached, err = g.GeocodeAddress(ctx, 1, models.ContactAddress{Street: "2 Main St", City: "Springfield"})
	require.NoError(t, err)
	assert.False(t, cached)
	assert.EqualValues(t, 2, atomic.LoadInt32(hits))
}

func TestGeocoder_FailuresAreNotCached(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	g, hits, cache := fakeGeocoder(t, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		okHandler(w, r)
	})
	addr := models.ContactAddress{Street: "1 Main St"}

	_, _, err := g.GeocodeAddress(context.Background(), 1, addr)
	require.ErrorIs(t, err, ErrGeocoderRequestFailed)
	assert.Zero(t, cache.len(), "a failed lookup leaves nothing behind")

	fail.Store(false)
	_, cached, err := g.GeocodeAddress(context.Background(), 1, addr)
	require.NoError(t, err)
	assert.False(t, cached, "so the retry really asks again")
	assert.EqualValues(t, 2, atomic.LoadInt32(hits))
}

func TestGeocoder_NoMatchIsNotCachedEither(t *testing.T) {
	g, hits, cache := fakeGeocoder(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	for i := 0; i < 2; i++ {
		_, _, err := g.GeocodeAddress(context.Background(), 1, models.ContactAddress{Street: "Nowhere"})
		require.ErrorIs(t, err, ErrGeocoderNoResult)
	}
	assert.Zero(t, cache.len())
	assert.EqualValues(t, 2, atomic.LoadInt32(hits))
}

func TestGeocoder_AddressWithNoPostalTextNeverCallsProvider(t *testing.T) {
	g, hits, _ := fakeGeocoder(t, okHandler)
	_, _, err := g.GeocodeAddress(context.Background(), 1, models.ContactAddress{POBox: "99", Apartment: "4B", Type: "home"})
	assert.ErrorIs(t, err, ErrGeocoderNoResult)
	assert.Zero(t, atomic.LoadInt32(hits))
}

func TestGeocodeQueryText(t *testing.T) {
	assert.Equal(t, "1 Main St, Springfield, IL, 62701, USA", GeocodeQueryText(models.ContactAddress{
		Street: "1 Main St", City: "Springfield", Region: "IL", Postal: "62701", Country: "USA",
		POBox: "PO Box 9", Apartment: "Apt 4", Floor: "2", Type: "home",
	}), "postal slots only, in order, never PO box / apartment / floor / type")
	assert.Equal(t, "Springfield, USA", GeocodeQueryText(models.ContactAddress{City: " Springfield ", Region: "  ", Country: "USA"}), "blanks skipped, parts trimmed")
	assert.Empty(t, GeocodeQueryText(models.ContactAddress{POBox: "1"}))
}

func TestIsGeocoderClientError(t *testing.T) {
	for _, e := range []error{
		ErrGeocoderDisabled, ErrGeocoderUnreachable, ErrGeocoderUnauthorized, ErrGeocoderNotFound,
		ErrGeocoderInvalidData, ErrGeocoderPrivateAddr, ErrGeocoderRequestFailed, ErrGeocoderRateLimited,
		ErrGeocoderNoResult,
		fmt.Errorf("wrapped: %w", ErrGeocoderUnreachable),
		&GeocoderRequestError{StatusCode: 500, Status: "500"},
	} {
		assert.True(t, IsGeocoderClientError(e), "%v", e)
	}
	assert.False(t, IsGeocoderClientError(errors.New("something else")))
	assert.False(t, IsGeocoderClientError(nil))
}

// --- cache ---------------------------------------------------------------

func TestGeocodeCache_LRUEviction(t *testing.T) {
	c := newGeocodeCache(2, time.Hour, time.Now)
	c.put(1, "p", "a", "geo:1,1")
	c.put(1, "p", "b", "geo:2,2")
	_, ok := c.get(1, "p", "a") // a is now most recent; b is the eviction candidate
	require.True(t, ok)
	c.put(1, "p", "c", "geo:3,3")

	assert.Equal(t, 2, c.len(), "bounded at capacity")
	_, ok = c.get(1, "p", "b")
	assert.False(t, ok, "the least recently used entry was evicted")
	_, ok = c.get(1, "p", "a")
	assert.True(t, ok)
	_, ok = c.get(1, "p", "c")
	assert.True(t, ok)
}

func TestGeocodeCache_TTLExpiry(t *testing.T) {
	now := time.Unix(5_000_000, 0)
	c := newGeocodeCache(4, time.Hour, func() time.Time { return now })
	c.put(1, "p", "a", "geo:1,1")

	now = now.Add(time.Hour - time.Second)
	uri, ok := c.get(1, "p", "a")
	assert.True(t, ok)
	assert.Equal(t, "geo:1,1", uri)

	now = now.Add(time.Second) // exactly TTL old
	_, ok = c.get(1, "p", "a")
	assert.False(t, ok, "expired at the TTL boundary")
	assert.Zero(t, c.len(), "and dropped on read")
}

func TestGeocodeCache_PutRefreshesExistingEntry(t *testing.T) {
	now := time.Unix(6_000_000, 0)
	c := newGeocodeCache(4, time.Hour, func() time.Time { return now })
	c.put(1, "p", "a", "geo:1,1")
	now = now.Add(50 * time.Minute)
	c.put(1, "p", "a", "geo:9,9") // same key: updates value and TTL, no second entry
	now = now.Add(50 * time.Minute)

	uri, ok := c.get(1, "p", "a")
	require.True(t, ok, "the TTL restarted at the second put")
	assert.Equal(t, "geo:9,9", uri)
	assert.Equal(t, 1, c.len())
}

func TestGeocodeCache_KeysAreProviderScopedAndOpaque(t *testing.T) {
	c := newGeocodeCache(4, time.Hour, time.Now)
	c.put(1, "nominatim", "1 Main St", "geo:1,1")
	_, ok := c.get(1, "maptiler", "1 Main St")
	assert.False(t, ok, "a different provider is a different entry")

	for key := range c.entries {
		assert.Len(t, key, 64, "keys are SHA-256 hex digests")
		assert.NotContains(t, key, "main", "the cache never holds a readable copy of the address text")
	}
	assert.Equal(t, geocodeCacheKey(1, "p", "A  b"), geocodeCacheKey(1, "p", " a b "))
	assert.NotEqual(t, geocodeCacheKey(1, "p", "a b"), geocodeCacheKey(1, "q", "a b"))
	assert.NotEqual(t, geocodeCacheKey(1, "p", "a b"), geocodeCacheKey(2, "p", "a b"), "keys are per user")
}

func TestGeocodeCache_ConcurrentUse(t *testing.T) {
	c := newGeocodeCache(16, time.Hour, time.Now)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				text := fmt.Sprintf("addr-%d", (g*7+i)%40)
				c.put(1, "p", text, "geo:1,1")
				c.get(1, "p", text)
			}
		}(g)
	}
	wg.Wait()
	assert.LessOrEqual(t, c.len(), 16)
}

func TestGeocoder_CacheIsScopedPerUser(t *testing.T) {
	g, hits, _ := fakeGeocoder(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"lat":"1.5","lon":"2.5"}]`))
	})
	addr := models.ContactAddress{Street: "1 Main St", City: "Springfield"}
	ctx := context.Background()

	_, cached, err := g.GeocodeAddress(ctx, 1, addr)
	require.NoError(t, err)
	assert.False(t, cached)
	_, cached, err = g.GeocodeAddress(ctx, 2, addr)
	require.NoError(t, err)
	assert.False(t, cached, "another user's lookup must not reveal (or reuse) user 1's cache entry")
	assert.EqualValues(t, 2, atomic.LoadInt32(hits), "the provider is called again for user 2")
	_, cached, err = g.GeocodeAddress(ctx, 1, addr)
	require.NoError(t, err)
	assert.True(t, cached)
	assert.EqualValues(t, 2, atomic.LoadInt32(hits))
}
