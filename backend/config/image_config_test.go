package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_ImageDefaultsAndEnv(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", validConfig().JWTSecretKey)
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")

	cfg := LoadConfig()
	assert.Equal(t, "/tmp/images", cfg.ImagesDir, "default beside PROFILE_PHOTO_DIR")
	assert.Equal(t, 20, cfg.ImageMaxUploadMB)
	assert.Equal(t, 50_000_000, cfg.ImageMaxPixels)
	assert.Equal(t, 10, cfg.ImageMaxPerActivity)
	assert.Zero(t, cfg.ImageProcessConcurrency)

	t.Setenv("IMAGES_DIR", "/var/images")
	t.Setenv("IMAGE_MAX_UPLOAD_MB", "30")
	t.Setenv("IMAGE_MAX_PIXELS", "1000000")
	t.Setenv("IMAGE_MAX_PER_ACTIVITY", "3")
	t.Setenv("IMAGE_PROCESS_CONCURRENCY", "4")
	cfg = LoadConfig()
	assert.Equal(t, "/var/images", cfg.ImagesDir)
	assert.Equal(t, 30, cfg.ImageMaxUploadMB)
	assert.Equal(t, 1_000_000, cfg.ImageMaxPixels)
	assert.Equal(t, 3, cfg.ImageMaxPerActivity)
	assert.Equal(t, 4, cfg.ImageProcessConcurrency)
}

func TestValidate_ImagesDirMustBeAbsolute(t *testing.T) {
	cfg := validConfig()
	cfg.ImagesDir = "relative/images"
	errs := cfg.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Field, "IMAGES_DIR")
}
