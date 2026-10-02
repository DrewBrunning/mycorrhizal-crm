package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contact map settings (ADR 0031, issue #694).

// fakeJWTSecret is a throwaway value long enough to get LoadConfig past its
// own checks; none of these tests assert anything about it.
func fakeJWTSecret() string { return strings.Repeat("fake-jwt-", 5) }

func TestLoadConfig_MapDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", fakeJWTSecret())
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	for _, k := range []string{"MAP_TILE_STYLE_URL", "GEOCODER_PROVIDER", "GEOCODER_API_KEY"} {
		t.Setenv(k, "") // registers the restore; the Unsetenv below makes it truly unset
		require.NoError(t, os.Unsetenv(k))
	}

	cfg := LoadConfig()
	assert.Equal(t, DefaultMapTileStyleURL, cfg.MapTileStyleURL, "unset MAP_TILE_STYLE_URL falls back to OpenFreeMap")
	assert.Equal(t, GeocoderProviderNone, cfg.GeocoderProvider, "geocoding is default-off")
	assert.Empty(t, cfg.GeocoderAPIKey)
}

// A compose file that passes the variable through as an empty string (MAP_TILE_STYLE_URL=)
// is "set but empty": the config keeps the empty value, validation accepts it,
// and the Effective accessors still resolve the defaults.
func TestLoadConfig_MapEmptyEnvStillResolvesDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", fakeJWTSecret())
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("MAP_TILE_STYLE_URL", "")
	t.Setenv("GEOCODER_PROVIDER", "")

	cfg := LoadConfig()
	assert.False(t, hasFieldError(cfg.Validate(), "MAP_TILE_STYLE_URL"))
	assert.False(t, hasFieldError(cfg.Validate(), "GEOCODER_PROVIDER"))
	assert.Equal(t, DefaultMapTileStyleURL, cfg.EffectiveMapTileStyleURL())
	assert.Equal(t, GeocoderProviderNone, cfg.EffectiveGeocoderProvider())
}

func TestLoadConfig_MapEnvReadThrough(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", fakeJWTSecret())
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("MAP_TILE_STYLE_URL", "https://tiles.example.org/style.json")
	t.Setenv("GEOCODER_PROVIDER", "  MapTiler ")
	t.Setenv("GEOCODER_API_KEY", "k3y")

	cfg := LoadConfig()
	assert.Equal(t, "https://tiles.example.org/style.json", cfg.MapTileStyleURL)
	assert.Equal(t, GeocoderProviderMapTiler, cfg.GeocoderProvider, "provider is trimmed and lower-cased")
	assert.Equal(t, "k3y", cfg.GeocoderAPIKey)
}

func TestValidate_MapTileStyleURL(t *testing.T) {
	for _, ok := range []string{"", DefaultMapTileStyleURL, "http://localhost:8080/style.json"} {
		cfg := validConfig()
		cfg.MapTileStyleURL = ok
		assert.False(t, hasFieldError(cfg.Validate(), "MAP_TILE_STYLE_URL"), "%q must be accepted", ok)
	}
	for _, bad := range []string{"not a url", "ftp://tiles.example.org/s.json", "javascript:alert(1)", "https://", "/relative/style.json"} {
		cfg := validConfig()
		cfg.MapTileStyleURL = bad
		assert.True(t, hasFieldError(cfg.Validate(), "MAP_TILE_STYLE_URL"), "%q must be rejected", bad)
	}
}

func TestValidate_GeocoderProvider(t *testing.T) {
	cases := []struct {
		name, provider, key string
		wantField           string // "" = valid
	}{
		{"unset", "", "", ""},
		{"none", GeocoderProviderNone, "", ""},
		{"nominatim needs no key", GeocoderProviderNominatim, "", ""},
		{"maptiler with key", GeocoderProviderMapTiler, "abc", ""},
		{"maptiler without key", GeocoderProviderMapTiler, "", "GEOCODER_API_KEY"},
		{"maptiler with blank key", GeocoderProviderMapTiler, "   ", "GEOCODER_API_KEY"},
		{"unknown provider", "google", "abc", "GEOCODER_PROVIDER"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.GeocoderProvider, cfg.GeocoderAPIKey = tc.provider, tc.key
			errs := cfg.Validate()
			if tc.wantField == "" {
				assert.False(t, hasFieldError(errs, "GEOCODER_PROVIDER") || hasFieldError(errs, "GEOCODER_API_KEY"), "%+v", errs)
				return
			}
			assert.True(t, hasFieldError(errs, tc.wantField), "%+v", errs)
		})
	}
}

func TestEffectiveMapSettings(t *testing.T) {
	cfg := &Config{}
	assert.Equal(t, DefaultMapTileStyleURL, cfg.EffectiveMapTileStyleURL())
	assert.Equal(t, GeocoderProviderNone, cfg.EffectiveGeocoderProvider())

	cfg = &Config{MapTileStyleURL: " https://t.example/s.json ", GeocoderProvider: GeocoderProviderNominatim}
	assert.Equal(t, "https://t.example/s.json", cfg.EffectiveMapTileStyleURL())
	assert.Equal(t, GeocoderProviderNominatim, cfg.EffectiveGeocoderProvider())
}

func TestNew_RejectsBadMapSettings(t *testing.T) {
	_, err := New(func(c *Config) {
		c.JWTSecretKey = fakeJWTSecret()
		c.ProfilePhotoDir = "/var/data/photos"
		c.GeocoderProvider = GeocoderProviderMapTiler // no key
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GEOCODER_API_KEY")
}
