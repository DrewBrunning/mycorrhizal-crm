package services

import (
	"errors"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net/url"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// System token used by the Paperless-ngx integration on the generic substrate.
// "paperless" is just one value of the open `system` classifier — the
// substrate (ticket T14) is what makes that possible.
const ExternalSystemPaperless = "paperless"

// PaperlessConfigResponse is the API-visible shape of a PaperlessConfig: the
// encrypted API token is never exposed, only whether one is stored.
type PaperlessConfigResponse struct {
	BaseURL     string `json:"base_url"`
	HasAPIToken bool   `json:"has_api_token"`
}

// PaperlessConnectionTestResult is the outcome of "Test connection" (Settings):
// a stage-by-stage diagnosis rather than a plain success/exception, since the
// point of this action is telling the user *what* is wrong when something is,
// not just that something is. Stage is one of "reachability", "auth", or "ok".
type PaperlessConnectionTestResult struct {
	OK      bool   `json:"ok"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// GetPaperlessConfigForUser returns the user's PaperlessConfig, or nil when
// they have not set one up.
func GetPaperlessConfigForUser(db *gorm.DB, userID uint) (*models.PaperlessConfig, error) {
	var config models.PaperlessConfig
	if err := db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

// NormalizePaperlessBaseURL validates and sanitizes a user-supplied Paperless
// base URL at save time, mirroring NormalizeImmichBaseURL's shape (trim,
// parse, require an explicit http/https scheme + host) rather than guessing at
// what the user meant — the codebase's established precedent for self-hosted-
// server URLs is to reject malformed input, not auto-repair it.
func NormalizePaperlessBaseURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", ErrPaperlessInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrPaperlessInvalidURL
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// UpsertPaperlessConfig creates or updates a user's PaperlessConfig. A
// non-empty APIToken is encrypted at rest (credential_crypto.go); an empty one
// on update keeps the existing stored token unchanged. On create the token is
// required. The base URL is normalized/validated immediately so a malformed
// URL is rejected at save time, not the first time the connection is used.
func UpsertPaperlessConfig(db *gorm.DB, jwtSecret string, userID uint, input models.PaperlessConfigInput) (*models.PaperlessConfig, error) {
	existing, err := GetPaperlessConfigForUser(db, userID)
	if err != nil {
		return nil, err
	}

	baseURL, err := NormalizePaperlessBaseURL(input.BaseURL)
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
		return nil, fmt.Errorf("paperless config: an API token is required when first connecting Paperless")
	}
	enc, err := EncryptCredential(jwtSecret, input.APIToken)
	if err != nil {
		return nil, err
	}
	created := models.PaperlessConfig{
		UserID:            userID,
		BaseURL:           baseURL,
		APITokenEncrypted: enc,
	}
	if err := db.Create(&created).Error; err != nil {
		return nil, err
	}
	return &created, nil
}

// DeletePaperlessConfig removes a user's PaperlessConfig (soft delete). Their
// ExternalIdentity history is kept — unlinking the connection does not forget
// the links.
func DeletePaperlessConfig(db *gorm.DB, userID uint) error {
	return db.Where("user_id = ?", userID).Delete(&models.PaperlessConfig{}).Error
}

// buildPaperlessClient loads the user's config, decrypts the API token, and
// constructs a PaperlessClient. Any failure is mapped to a sentinel error.
func buildPaperlessClient(db *gorm.DB, cfg config.Config, userID uint) (*PaperlessClient, *models.PaperlessConfig, error) {
	pc, err := GetPaperlessConfigForUser(db, userID)
	if err != nil {
		return nil, nil, err
	}
	if pc == nil {
		return nil, nil, fmt.Errorf("%w: no Paperless connection configured", ErrPaperlessUnauthorized)
	}
	token, err := DecryptCredential(cfg.JWTSecretKey, pc.APITokenEncrypted)
	if err != nil {
		return nil, nil, err
	}
	client, err := NewPaperlessClient(pc.BaseURL, token, cfg.PaperlessBlockPrivateURLs)
	if err != nil {
		return nil, nil, err
	}
	return client, pc, nil
}

// TestPaperlessConnection checks the current user's saved Paperless connection
// in two stages — reachability (Ping) then auth (GetMe) — so a failure is
// diagnosed rather than just reported. A Go error here means the check itself
// could not run (no connection configured, stored URL unparseable); the
// controller maps that through abortPaperlessServiceError exactly like every
// other Paperless endpoint. A populated, non-error result with OK:false is a
// *successful* diagnosis of an upstream problem, not an application error —
// this always responds 200.
func TestPaperlessConnection(db *gorm.DB, cfg config.Config, userID uint) (*PaperlessConnectionTestResult, error) {
	client, _, err := buildPaperlessClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(); err != nil {
		stage := "reachability"
		if errors.Is(err, ErrPaperlessUnauthorized) {
			stage = "auth"
		}
		logger.Warn().Err(err).Uint("user_id", userID).Str("stage", stage).Msg("Paperless test connection: ping failed")
		return diagnosePaperlessConnectionFailure(stage, err), nil
	}

	user, err := client.GetMe()
	if err != nil {
		logger.Warn().Err(err).Uint("user_id", userID).Msg("Paperless test connection: auth failed")
		return diagnosePaperlessConnectionFailure("auth", err), nil
	}

	logger.Info().Uint("user_id", userID).Msg("Paperless test connection: ok")
	return &PaperlessConnectionTestResult{
		OK:      true,
		Stage:   "ok",
		Message: fmt.Sprintf("Connected to Paperless as %s", user.UserName),
	}, nil
}

// diagnosePaperlessConnectionFailure turns a sentinel error from the client
// into a specific, stage-appropriate message for the user.
func diagnosePaperlessConnectionFailure(stage string, err error) *PaperlessConnectionTestResult {
	message := err.Error()
	switch {
	case errors.Is(err, ErrPaperlessPrivateAddress):
		message = "The Paperless URL resolves to a private or loopback address, which this server is configured to block (PAPERLESS_BLOCK_PRIVATE_URLS)."
	case errors.Is(err, ErrPaperlessUnauthorized):
		message = "Paperless rejected the API token. Check that it hasn't been revoked, expired, or mistyped."
	case errors.Is(err, ErrPaperlessUnreachable):
		message = fmt.Sprintf("Could not reach the Paperless server: %v", err)
	case errors.Is(err, ErrPaperlessRequestFailed):
		status := "an unexpected status"
		var reqErr *PaperlessRequestError
		if errors.As(err, &reqErr) {
			status = reqErr.Status
		}
		message = fmt.Sprintf("The Paperless server is reachable, but this request failed (%s).", status)
	}
	return &PaperlessConnectionTestResult{OK: false, Stage: stage, Message: message}
}

// ListPaperlessDocumentsForUser browses documents in the user's Paperless
// instance (the L1 picker), optionally filtered by a query string. Requires a
// configured connection.
func ListPaperlessDocumentsForUser(db *gorm.DB, cfg config.Config, userID uint, query string) ([]PaperlessDocument, error) {
	client, _, err := buildPaperlessClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}
	return client.ListDocuments(query)
}

// LinkPaperlessDocument persists a document link as an ExternalIdentity
// (system: "paperless") — the whole reason T14's substrate comes first. The
// document is fetched by id under the user's own token (authoritative metadata
// stored on the identity, never trusted from the client), and the deep-link
// URL points at the Paperless document detail page. A duplicate (system,
// document, user) link is surfaced to the caller as a 409 via the controller's
// own pre-check.
func LinkPaperlessDocument(db *gorm.DB, cfg config.Config, userID uint, contactUID, documentID string) (*models.ExternalIdentity, error) {
	client, pc, err := buildPaperlessClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}
	id, err := parsePaperlessDocumentID(documentID)
	if err != nil {
		return nil, err
	}
	doc, err := client.GetDocument(id)
	if err != nil {
		return nil, err
	}

	metadata := map[string]interface{}{
		"title":      doc.Title,
		"file_name":  doc.FileName,
		"created_at": doc.Created,
	}

	identity := models.ExternalIdentity{
		UserID:     userID,
		EntityID:   contactUID,
		System:     ExternalSystemPaperless,
		ExternalID: strconv.Itoa(doc.ID),
		URL:        strings.TrimRight(pc.BaseURL, "/") + "/documents/" + strconv.Itoa(doc.ID) + "/details",
		Metadata:   metadata,
		SyncStatus: models.ExternalIdentitySyncStatusIdle,
	}
	if err := db.Create(&identity).Error; err != nil {
		return nil, err
	}
	return &identity, nil
}

// parsePaperlessDocumentID converts a stored external_id (a document id) back
// to the numeric form the client expects. Used by the thumbnail/summary-style
// live lookups; for L1 the id is only ever echoed, so a failure surfaces as
// ErrPaperlessInvalidData.
func parsePaperlessDocumentID(externalID string) (int, error) {
	id, err := strconv.Atoi(externalID)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("%w: malformed Paperless document id %q", ErrPaperlessInvalidData, externalID)
	}
	return id, nil
}
