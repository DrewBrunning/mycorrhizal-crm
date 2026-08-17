package services

import (
	"errors"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net/url"
	"strings"

	"gorm.io/gorm"
)

// System token used by the Nextcloud / ownCloud (WebDAV) integration on the
// generic substrate. "nextcloud" is the shared token for the fork pair —
// both serve the same standard WebDAV surface, so they share one client (the
// P2c design-pass decision: no real behavioral divergence for L1).
const ExternalSystemWebDAV = "nextcloud"

// WebDAVConfigResponse is the API-visible shape of a WebDAVConfig: the
// encrypted app password is never exposed, only whether one is stored.
type WebDAVConfigResponse struct {
	BaseURL        string `json:"base_url"`
	Username       string `json:"username"`
	HasAppPassword bool   `json:"has_app_password"`
}

// WebDAVConnectionTestResult is the outcome of "Test connection" (Settings): a
// diagnosis rather than a plain success/exception. WebDAV cannot separate
// "reachable" from "authenticated" without a separate unauthenticated request
// (the protocol has none), so this is a single "auth" stage: a 401/403 is a
// credential problem, anything else transport. Stage is "reachability" or
// "ok".
type WebDAVConnectionTestResult struct {
	OK      bool   `json:"ok"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// GetWebDAVConfigForUser returns the user's WebDAVConfig, or nil when they
// have not set one up.
func GetWebDAVConfigForUser(db *gorm.DB, userID uint) (*models.WebDAVConfig, error) {
	var config models.WebDAVConfig
	if err := db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

// NormalizeWebDAVBaseURL validates and sanitizes a user-supplied Nextcloud base
// URL at save time, mirroring NormalizeImmichBaseURL's shape.
func NormalizeWebDAVBaseURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", ErrWebDAVInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrWebDAVInvalidURL
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// UpsertWebDAVConfig creates or updates a user's WebDAVConfig. A non-empty
// AppPassword is encrypted at rest (credential_crypto.go); an empty one on
// update keeps the existing stored password unchanged. On create the password
// is required. Only an app password is accepted — never the account password.
func UpsertWebDAVConfig(db *gorm.DB, jwtSecret string, userID uint, input models.WebDAVConfigInput) (*models.WebDAVConfig, error) {
	existing, err := GetWebDAVConfigForUser(db, userID)
	if err != nil {
		return nil, err
	}

	baseURL, err := NormalizeWebDAVBaseURL(input.BaseURL)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		existing.BaseURL = baseURL
		existing.Username = input.Username
		if input.AppPassword != "" {
			enc, err := EncryptCredential(jwtSecret, input.AppPassword)
			if err != nil {
				return nil, err
			}
			existing.AppPasswordEncrypted = enc
		}
		if err := db.Save(existing).Error; err != nil {
			return nil, err
		}
		return existing, nil
	}

	if input.AppPassword == "" {
		return nil, fmt.Errorf("nextcloud config: an app password is required when first connecting Nextcloud")
	}
	enc, err := EncryptCredential(jwtSecret, input.AppPassword)
	if err != nil {
		return nil, err
	}
	created := models.WebDAVConfig{
		UserID:               userID,
		BaseURL:              baseURL,
		Username:             input.Username,
		AppPasswordEncrypted: enc,
	}
	if err := db.Create(&created).Error; err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteWebDAVConfig removes a user's WebDAVConfig (soft delete). Their
// ExternalIdentity history is kept.
func DeleteWebDAVConfig(db *gorm.DB, userID uint) error {
	return db.Where("user_id = ?", userID).Delete(&models.WebDAVConfig{}).Error
}

// buildWebDAVClient loads the user's config, decrypts the app password, and
// constructs a WebDAVClient. Any failure is mapped to a sentinel error.
func buildWebDAVClient(db *gorm.DB, cfg config.Config, userID uint) (*WebDAVClient, *models.WebDAVConfig, error) {
	wc, err := GetWebDAVConfigForUser(db, userID)
	if err != nil {
		return nil, nil, err
	}
	if wc == nil {
		return nil, nil, fmt.Errorf("%w: no Nextcloud connection configured", ErrWebDAVUnauthorized)
	}
	password, err := DecryptCredential(cfg.JWTSecretKey, wc.AppPasswordEncrypted)
	if err != nil {
		return nil, nil, err
	}
	client, err := NewWebDAVClient(wc.BaseURL, wc.Username, password, cfg.WebDAVBlockPrivateURLs)
	if err != nil {
		return nil, nil, err
	}
	return client, wc, nil
}

// TestWebDAVConnection checks the current user's saved Nextcloud connection
// with a single PROPFIND on the dav root (WebDAV has no unauthenticated
// reachability probe — the request either authenticates or it doesn't). A Go
// error here means the check itself could not run; the controller maps that
// through abortWebDAVServiceError. A populated, non-error result with OK:false
// is a *successful* diagnosis of an upstream problem, not an application error
// — this always responds 200.
func TestWebDAVConnection(db *gorm.DB, cfg config.Config, userID uint) (*WebDAVConnectionTestResult, error) {
	client, _, err := buildWebDAVClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(); err != nil {
		logger.Warn().Err(err).Uint("user_id", userID).Msg("Nextcloud test connection failed")
		return diagnoseWebDAVConnectionFailure(err), nil
	}

	logger.Info().Uint("user_id", userID).Msg("Nextcloud test connection: ok")
	return &WebDAVConnectionTestResult{
		OK:      true,
		Stage:   "ok",
		Message: "Connected to Nextcloud/ownCloud",
	}, nil
}

// diagnoseWebDAVConnectionFailure turns a sentinel error from the client into
// a specific message for the user.
func diagnoseWebDAVConnectionFailure(err error) *WebDAVConnectionTestResult {
	message := err.Error()
	switch {
	case errors.Is(err, ErrWebDAVPrivateAddress):
		message = "The Nextcloud URL resolves to a private or loopback address, which this server is configured to block (WEBDAV_BLOCK_PRIVATE_URLS)."
	case errors.Is(err, ErrWebDAVUnauthorized):
		message = "Nextcloud rejected the app password. Check that it hasn't been revoked, expired, or mistyped, and that the username is correct."
	case errors.Is(err, ErrWebDAVUnreachable):
		message = fmt.Sprintf("Could not reach the Nextcloud server: %v", err)
	case errors.Is(err, ErrWebDAVRequestFailed):
		status := "an unexpected status"
		var reqErr *WebDAVRequestError
		if errors.As(err, &reqErr) {
			status = reqErr.Status
		}
		message = fmt.Sprintf("The Nextcloud server is reachable, but this request failed (%s).", status)
	}
	return &WebDAVConnectionTestResult{OK: false, Stage: "reachability", Message: message}
}

// ListWebDAVDirForUser lists a directory's children (the L1 picker's
// navigation), relative to the dav root. Requires a configured connection.
func ListWebDAVDirForUser(db *gorm.DB, cfg config.Config, userID uint, path string) ([]WebDAVItem, error) {
	client, _, err := buildWebDAVClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}
	return client.ListDir(path)
}

// WebDAVLinkMetadata is the subset of a browsed WebDAV item the link request
// carries.
type WebDAVLinkMetadata struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Type       string `json:"type"` // "file" | "dir"
	Size       int64  `json:"size,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
	FileID     string `json:"file_id,omitempty"`
}

// webdavFilesAppURL builds the browser URL for a Nextcloud item in the files
// app. A folder deep-links to its directory; a file deep-links to the
// containing directory and, when a file id is available, opens the file
// directly (the files app's `openfile` parameter).
func webdavFilesAppURL(baseURL string, item WebDAVLinkMetadata) string {
	base := strings.TrimRight(baseURL, "/")
	if item.Type == "dir" {
		return base + "/apps/files/?dir=" + url.QueryEscape(normalizeWebDAVPath(item.Path))
	}
	parent := normalizeWebDAVPath(item.Path)
	if idx := strings.LastIndex(parent, "/"); idx > 0 {
		parent = parent[:idx]
	} else {
		parent = "/"
	}
	u := base + "/apps/files/?dir=" + url.QueryEscape(parent)
	if item.FileID != "" {
		u += "&openfile=" + url.QueryEscape(item.FileID)
	}
	return u
}

// LinkWebDAVItem persists a file/folder link as an ExternalIdentity (system:
// "nextcloud"). A duplicate (system, external_id, user) link is surfaced to
// the caller as a 409 via the controller's own pre-check.
func LinkWebDAVItem(db *gorm.DB, cfg config.Config, userID uint, contactUID string, item WebDAVLinkMetadata) (*models.ExternalIdentity, error) {
	wc, err := GetWebDAVConfigForUser(db, userID)
	if err != nil {
		return nil, err
	}
	if wc == nil {
		return nil, fmt.Errorf("%w: no Nextcloud connection configured", ErrWebDAVUnauthorized)
	}

	metadata := map[string]interface{}{
		"name": item.Name,
		"type": item.Type,
	}
	if item.Size > 0 {
		metadata["size"] = item.Size
	}
	if item.ModifiedAt != "" {
		metadata["modified_at"] = item.ModifiedAt
	}
	if item.FileID != "" {
		metadata["file_id"] = item.FileID
	}

	identity := models.ExternalIdentity{
		UserID:     userID,
		EntityID:   contactUID,
		System:     ExternalSystemWebDAV,
		ExternalID: normalizeWebDAVPath(item.Path),
		URL:        webdavFilesAppURL(wc.BaseURL, item),
		Metadata:   metadata,
		SyncStatus: models.ExternalIdentitySyncStatusIdle,
	}
	if err := db.Create(&identity).Error; err != nil {
		return nil, err
	}
	return &identity, nil
}

// NormalizeWebDAVPathForKey normalizes a WebDAV path into the canonical
// leading-slash external_id form (no trailing slash for files). Exported so
// the controller's duplicate-link pre-check keys on exactly the value
// LinkWebDAVItem stores.
func NormalizeWebDAVPathForKey(path string) string {
	return normalizeWebDAVPath(path)
}
