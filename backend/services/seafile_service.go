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

// System token used by the Seafile integration on the generic substrate.
// "seafile" is just one value of the open `system` classifier.
const ExternalSystemSeafile = "seafile"

// SeafileConfigResponse is the API-visible shape of a SeafileConfig: the
// encrypted API token is never exposed, only whether one is stored.
type SeafileConfigResponse struct {
	BaseURL     string `json:"base_url"`
	HasAPIToken bool   `json:"has_api_token"`
}

// SeafileConnectionTestResult is the outcome of "Test connection" (Settings):
// a stage-by-stage diagnosis rather than a plain success/exception. Stage is
// one of "reachability", "auth", or "ok".
type SeafileConnectionTestResult struct {
	OK      bool   `json:"ok"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// GetSeafileConfigForUser returns the user's SeafileConfig, or nil when they
// have not set one up.
func GetSeafileConfigForUser(db *gorm.DB, userID uint) (*models.SeafileConfig, error) {
	var config models.SeafileConfig
	if err := db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

// NormalizeSeafileBaseURL validates and sanitizes a user-supplied Seafile base
// URL at save time, mirroring NormalizeImmichBaseURL's shape (trim, parse,
// require an explicit http/https scheme + host) rather than guessing at what
// the user meant.
func NormalizeSeafileBaseURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", ErrSeafileInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrSeafileInvalidURL
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// UpsertSeafileConfig creates or updates a user's SeafileConfig. A non-empty
// APIToken is encrypted at rest (credential_crypto.go); an empty one on update
// keeps the existing stored token unchanged. On create the token is required.
func UpsertSeafileConfig(db *gorm.DB, jwtSecret string, userID uint, input models.SeafileConfigInput) (*models.SeafileConfig, error) {
	existing, err := GetSeafileConfigForUser(db, userID)
	if err != nil {
		return nil, err
	}

	baseURL, err := NormalizeSeafileBaseURL(input.BaseURL)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		existing.BaseURL = baseURL
		if input.APIToken != "" {
			enc, err := EncryptCredential(jwtSecret, input.APIToken)
			if err != nil {
				return nil, err
			}
			existing.APITokenEncrypted = enc
		}
		if err := db.Save(existing).Error; err != nil {
			return nil, err
		}
		return existing, nil
	}

	if input.APIToken == "" {
		return nil, fmt.Errorf("seafile config: an API token is required when first connecting Seafile")
	}
	enc, err := EncryptCredential(jwtSecret, input.APIToken)
	if err != nil {
		return nil, err
	}
	created := models.SeafileConfig{
		UserID:            userID,
		BaseURL:           baseURL,
		APITokenEncrypted: enc,
	}
	if err := db.Create(&created).Error; err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteSeafileConfig removes a user's SeafileConfig (soft delete). Their
// ExternalIdentity history is kept.
func DeleteSeafileConfig(db *gorm.DB, userID uint) error {
	return db.Where("user_id = ?", userID).Delete(&models.SeafileConfig{}).Error
}

// buildSeafileClient loads the user's config, decrypts the API token, and
// constructs a SeafileClient. Any failure is mapped to a sentinel error.
func buildSeafileClient(db *gorm.DB, cfg config.Config, userID uint) (*SeafileClient, *models.SeafileConfig, error) {
	sc, err := GetSeafileConfigForUser(db, userID)
	if err != nil {
		return nil, nil, err
	}
	if sc == nil {
		return nil, nil, fmt.Errorf("%w: no Seafile connection configured", ErrSeafileUnauthorized)
	}
	token, err := DecryptCredential(cfg.JWTSecretKey, sc.APITokenEncrypted)
	if err != nil {
		return nil, nil, err
	}
	client, err := NewSeafileClient(sc.BaseURL, token, cfg.SeafileBlockPrivateURLs)
	if err != nil {
		return nil, nil, err
	}
	return client, sc, nil
}

// TestSeafileConnection checks the current user's saved Seafile connection in
// two stages — reachability (Ping, unauthenticated) then auth (PingAuth) — so
// a failure is diagnosed rather than just reported.
func TestSeafileConnection(db *gorm.DB, cfg config.Config, userID uint) (*SeafileConnectionTestResult, error) {
	client, _, err := buildSeafileClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(); err != nil {
		stage := "reachability"
		if errors.Is(err, ErrSeafileUnauthorized) {
			stage = "auth"
		}
		logger.Warn().Err(err).Uint("user_id", userID).Str("stage", stage).Msg("Seafile test connection: ping failed")
		return diagnoseSeafileConnectionFailure(stage, err), nil
	}

	if err := client.PingAuth(); err != nil {
		logger.Warn().Err(err).Uint("user_id", userID).Msg("Seafile test connection: auth failed")
		return diagnoseSeafileConnectionFailure("auth", err), nil
	}

	logger.Info().Uint("user_id", userID).Msg("Seafile test connection: ok")
	return &SeafileConnectionTestResult{
		OK:      true,
		Stage:   "ok",
		Message: "Connected to Seafile",
	}, nil
}

// diagnoseSeafileConnectionFailure turns a sentinel error from the client into
// a specific, stage-appropriate message for the user.
func diagnoseSeafileConnectionFailure(stage string, err error) *SeafileConnectionTestResult {
	message := err.Error()
	switch {
	case errors.Is(err, ErrSeafilePrivateAddress):
		message = "The Seafile URL resolves to a private or loopback address, which this server is configured to block (SEAFILE_BLOCK_PRIVATE_URLS)."
	case errors.Is(err, ErrSeafileUnauthorized):
		message = "Seafile rejected the API token. Check that it hasn't been revoked, expired, or mistyped."
	case errors.Is(err, ErrSeafileUnreachable):
		message = fmt.Sprintf("Could not reach the Seafile server: %v", err)
	case errors.Is(err, ErrSeafileRequestFailed):
		status := "an unexpected status"
		var reqErr *SeafileRequestError
		if errors.As(err, &reqErr) {
			status = reqErr.Status
		}
		message = fmt.Sprintf("The Seafile server is reachable, but this request failed (%s).", status)
	}
	return &SeafileConnectionTestResult{OK: false, Stage: stage, Message: message}
}

// ListSeafileLibrariesForUser browses every library the token can access (the
// L1 picker's first level). Requires a configured connection.
func ListSeafileLibrariesForUser(db *gorm.DB, cfg config.Config, userID uint) ([]SeafileLibrary, error) {
	client, _, err := buildSeafileClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}
	return client.ListLibraries()
}

// ListSeafileDirForUser lists a library folder's contents (the L1 picker's
// navigation). Requires a configured connection.
func ListSeafileDirForUser(db *gorm.DB, cfg config.Config, userID uint, repoID, path string) ([]SeafileItem, error) {
	client, _, err := buildSeafileClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}
	return client.ListDir(repoID, path)
}

// SeafileLinkMetadata is the subset of a browsed Seafile item the link request
// carries. Name and Type are validated by the controller; Size/MTime are
// informational (the L1 display, mirrored from the browse response).
type SeafileLinkMetadata struct {
	RepoID string `json:"repo_id"`
	Path   string `json:"path"`
	Name   string `json:"name"`
	Type   string `json:"type"` // "file" | "dir"
	Size   int64  `json:"size,omitempty"`
	MTime  int64  `json:"mtime,omitempty"`
}

// seafileExternalID builds the natural-key external_id for a Seafile link:
// repo_id + ":" + repo-relative path. The path is what uniquely identifies the
// item within the repo (two files in one repo cannot share a path), and the
// repo_id disambiguates across libraries. This is the stored identity — NOT a
// share link (which can itself carry a password/expiry, P2b's trap): the CRM
// stores the stable repo-relative location and a web URL, and the user opens
// it in their own authenticated browser session.
func seafileExternalID(repoID, path string) string {
	return repoID + ":" + path
}

// seafileWebURL builds the browser URL for a Seafile library item: the web app
// serves libraries at /lib/<repo_id>/<path>. A directory keeps a trailing
// slash so the web app renders it as a folder. Path segments are URL-escaped
// so a filename with a space or special character still yields a valid URL.
func seafileWebURL(baseURL, repoID, path, itemType string) string {
	clean := strings.TrimRight(strings.TrimSpace(path), "/")
	if clean == "" {
		clean = "/"
	}
	if itemType == "dir" && !strings.HasSuffix(clean, "/") {
		clean += "/"
	}
	return strings.TrimRight(baseURL, "/") + "/lib/" + url.PathEscape(repoID) + escapeURLPath(clean)
}

// escapeURLPath percent-encodes each segment of a "/"-delimited path so the
// separators survive but spaces and special characters in names do not.
func escapeURLPath(path string) string {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

// LinkSeafileItem persists a file/folder link as an ExternalIdentity (system:
// "seafile"). A duplicate (system, external_id, user) link is surfaced to the
// caller as a 409 via the controller's own pre-check.
func LinkSeafileItem(db *gorm.DB, cfg config.Config, userID uint, contactUID string, item SeafileLinkMetadata) (*models.ExternalIdentity, error) {
	sc, err := GetSeafileConfigForUser(db, userID)
	if err != nil {
		return nil, err
	}
	if sc == nil {
		return nil, fmt.Errorf("%w: no Seafile connection configured", ErrSeafileUnauthorized)
	}

	externalID := seafileExternalID(item.RepoID, item.Path)
	metadata := map[string]interface{}{
		"name": item.Name,
		"type": item.Type,
	}
	if item.Size > 0 {
		metadata["size"] = item.Size
	}
	if item.MTime > 0 {
		metadata["mtime"] = item.MTime
	}

	identity := models.ExternalIdentity{
		UserID:     userID,
		EntityID:   contactUID,
		System:     ExternalSystemSeafile,
		ExternalID: externalID,
		URL:        seafileWebURL(sc.BaseURL, item.RepoID, item.Path, item.Type),
		Metadata:   metadata,
		SyncStatus: models.ExternalIdentitySyncStatusIdle,
	}
	if err := db.Create(&identity).Error; err != nil {
		return nil, err
	}
	return &identity, nil
}
