package services

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
)

// Bounds for the in-memory geocode cache (ADR 0031 amendment: bounded size +
// TTL, lost on restart, no table, no migration).
const (
	geocodeCacheMaxEntries = 512
	geocodeCacheTTL        = 24 * time.Hour
)

// Geocoder is the explicit per-address geocoding service: a provider client
// plus an in-memory cache that bounds repeat calls for the same address text.
// A nil *Geocoder, or one built for provider "none", reports itself disabled.
type Geocoder struct {
	client *GeocoderClient
	cache  *geocodeCache
}

// NewGeocoder builds the geocoder for the instance configuration. Provider
// "none" (the default) yields a disabled geocoder with no client at all, so
// nothing can dial out.
func NewGeocoder(cfg *config.Config) (*Geocoder, error) {
	provider := cfg.EffectiveGeocoderProvider()
	if provider == config.GeocoderProviderNone {
		return &Geocoder{}, nil
	}
	client, err := NewGeocoderClient(provider, cfg.GeocoderAPIKey)
	if err != nil {
		return nil, err
	}
	return &Geocoder{client: client, cache: newGeocodeCache(geocodeCacheMaxEntries, geocodeCacheTTL, Now)}, nil
}

// Enabled reports whether a provider is configured.
func (g *Geocoder) Enabled() bool { return g != nil && g.client != nil }

// GeocodeAddress resolves one flat address to a geo: URI. The address text sent
// to the provider (and used as the cache key) is the postal slots only — street,
// city, region, postcode, country — never the PO box / apartment / floor, which
// a geocoder cannot place and which would only add noise (and entropy to the
// cache key). cached reports whether the answer came from the cache, i.e. no
// request left the instance. The cache is scoped per user (userID is part of the
// key): a process-wide key would let one user learn, via cached or timing,
// whether another user had geocoded the same address.
func (g *Geocoder) GeocodeAddress(ctx context.Context, userID uint, addr models.ContactAddress) (coordinates string, cached bool, err error) {
	if !g.Enabled() {
		return "", false, ErrGeocoderDisabled
	}
	query := GeocodeQueryText(addr)
	if query == "" {
		return "", false, ErrGeocoderNoResult
	}
	provider := g.client.Provider()
	if uri, ok := g.cache.get(userID, provider, query); ok {
		return uri, true, nil
	}
	res, err := g.client.Geocode(ctx, query)
	if err != nil {
		return "", false, err // errors are never cached: a failed lookup is retried
	}
	uri := contactmodel.FormatGeoURI(res.Lat, res.Lon)
	g.cache.put(userID, provider, query, uri)
	return uri, false, nil
}

// GeocodeQueryText is the free-form text sent to the geocoder for addr:
// street, city, region, postcode, country, comma-joined, empties skipped.
func GeocodeQueryText(a models.ContactAddress) string {
	parts := make([]string, 0, 5)
	for _, p := range []string{a.Street, a.City, a.Region, a.Postal, a.Country} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// IsGeocoderClientError reports whether err is one of the geocoder client's
// sentinels (so a controller can map it without a type switch on strings).
func IsGeocoderClientError(err error) bool {
	for _, s := range []error{
		ErrGeocoderDisabled, ErrGeocoderUnreachable, ErrGeocoderUnauthorized, ErrGeocoderNotFound,
		ErrGeocoderInvalidData, ErrGeocoderPrivateAddr, ErrGeocoderRequestFailed, ErrGeocoderRateLimited,
		ErrGeocoderNoResult,
	} {
		if errors.Is(err, s) {
			return true
		}
	}
	return false
}

// geocodeCache is a fixed-capacity LRU with a per-entry TTL, safe for
// concurrent use. Keys are SHA-256 digests of user id + provider + normalized address
// text, so the cache never retains a readable copy of an address — only the
// coordinate the provider returned. It lives in process memory only.
type geocodeCache struct {
	mu      sync.Mutex
	max     int
	ttl     time.Duration
	now     func() time.Time
	order   *list.List // front = most recently used
	entries map[string]*list.Element
}

type geocodeCacheEntry struct {
	key     string
	uri     string
	expires time.Time
}

func newGeocodeCache(max int, ttl time.Duration, now func() time.Time) *geocodeCache {
	return &geocodeCache{max: max, ttl: ttl, now: now, order: list.New(), entries: make(map[string]*list.Element, max)}
}

// geocodeCacheKey normalizes address text (case-folded, whitespace-collapsed)
// so trivially different spellings of one address share an entry, then hashes
// it together with the requesting user and the provider, so entries are never
// shared across users.
func geocodeCacheKey(userID uint, provider, text string) string {
	norm := strings.Join(strings.FieldsFunc(strings.ToLower(text), unicode.IsSpace), " ")
	sum := sha256.Sum256([]byte(strconv.FormatUint(uint64(userID), 10) + "\x00" + provider + "\x00" + norm))
	return hex.EncodeToString(sum[:])
}

func (c *geocodeCache) get(userID uint, provider, text string) (string, bool) {
	key := geocodeCacheKey(userID, provider, text)
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return "", false
	}
	e := el.Value.(*geocodeCacheEntry)
	if !c.now().Before(e.expires) {
		c.order.Remove(el)
		delete(c.entries, key)
		return "", false
	}
	c.order.MoveToFront(el)
	return e.uri, true
}

func (c *geocodeCache) put(userID uint, provider, text, uri string) {
	key := geocodeCacheKey(userID, provider, text)
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*geocodeCacheEntry)
		e.uri, e.expires = uri, c.now().Add(c.ttl)
		c.order.MoveToFront(el)
		return
	}
	c.entries[key] = c.order.PushFront(&geocodeCacheEntry{key: key, uri: uri, expires: c.now().Add(c.ttl)})
	for c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*geocodeCacheEntry).key)
	}
}

// len reports the live entry count (test hook).
func (c *geocodeCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
