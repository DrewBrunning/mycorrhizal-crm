package services

import (
	"errors"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

// System token and activity type used by the Immich integration on the
// generic substrate. "immich" is just one value of the open `system`
// classifier — the substrate (ticket T14) is what makes that possible.
const (
	ExternalSystemImmich  = "immich"
	ImmichActivityType    = "photo-appearance"
	immichSyncMinInterval = 30 * time.Minute
	immichSyncLimitAssets = 25
)

// ImmichConfigResponse is the API-visible shape of an ImmichConfig: the
// encrypted API key is never exposed, only whether one is stored.
type ImmichConfigResponse struct {
	BaseURL        string     `json:"base_url"`
	HasAPIKey      bool       `json:"has_api_key"`
	SyncEnabled    bool       `json:"sync_enabled"`
	LastSyncedAt   *time.Time `json:"last_synced_at,omitempty"`
	LastSyncStatus string     `json:"last_sync_status"`
	LastSyncError  string     `json:"last_sync_error"`
}

// ImmichConnectionTestResult is the outcome of "Test connection" (Settings):
// a stage-by-stage diagnosis rather than a plain success/exception, since
// the point of this action is telling the user *what* is wrong when
// something is, not just that something is. Stage is one of "reachability",
// "auth", or "ok".
type ImmichConnectionTestResult struct {
	OK      bool   `json:"ok"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// ImmichPersonSummary is what the contact page needs to render the Immich
// link: the identity plus live (or cached, on failure) person info.
type ImmichPersonSummary struct {
	Identity      models.ExternalIdentity `json:"identity"`
	PersonName    string                  `json:"person_name"`
	PhotoCount    int                     `json:"photo_count"`
	LatestAssetID string                  `json:"latest_asset_id,omitempty"`
	LatestAt      *time.Time              `json:"latest_at,omitempty"`
}

// GetImmichConfigForUser returns the user's ImmichConfig, or nil when they
// have not set one up.
func GetImmichConfigForUser(db *gorm.DB, userID uint) (*models.ImmichConfig, error) {
	var config models.ImmichConfig
	if err := db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

// NormalizeImmichBaseURL validates and sanitizes a user-supplied Immich base
// URL at save time, mirroring NormalizeCalendarURL/NormalizeContactSubscriptionURL's
// shape (trim, parse, require an explicit http/https scheme + host) rather
// than guessing at what the user meant — this codebase's established
// precedent for self-hosted-server URLs is to reject malformed input, not
// auto-repair it. A missing scheme is rejected (there is no way to tell
// whether the user meant http or https), which is why this returns
// ErrImmichInvalidURL — the same sentinel NewImmichClient already uses at
// *use* time — rather than accepting it and failing later, confusingly, the
// first time the connection is actually used.
//
// The one exception is stripping a trailing "/api" (or "/api/") path
// segment: this client always appends "/api/..." itself (immich_client.go's
// do()), so a base URL that already ends in exactly that segment can only
// produce broken double "/api/api/..." requests — unlike a missing scheme,
// there is exactly one correct interpretation, so it's safe to correct
// automatically. It's also a common mistake: Immich's own API docs live at
// "/api", and users copy that root by habit.
func NormalizeImmichBaseURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", ErrImmichInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrImmichInvalidURL
	}

	trimmedPath := strings.TrimRight(parsed.Path, "/")
	segments := strings.Split(trimmedPath, "/")
	if last := segments[len(segments)-1]; strings.EqualFold(last, "api") {
		parsed.Path = strings.Join(segments[:len(segments)-1], "/")
	}

	return strings.TrimRight(parsed.String(), "/"), nil
}

// UpsertImmichConfig creates or updates a user's ImmichConfig. A non-empty
// APIKey is encrypted at rest (credential_crypto.go); an empty one on update
// keeps the existing stored key unchanged. On create the key is required.
// The base URL is normalized/validated immediately (NormalizeImmichBaseURL)
// so a malformed URL is rejected at save time, not the first time the
// connection is actually used.
func UpsertImmichConfig(db *gorm.DB, jwtSecret string, userID uint, input models.ImmichConfigInput) (*models.ImmichConfig, error) {
	existing, err := GetImmichConfigForUser(db, userID)
	if err != nil {
		return nil, err
	}

	baseURL, err := NormalizeImmichBaseURL(input.BaseURL)
	if err != nil {
		return nil, err
	}

	syncEnabled := true
	if input.SyncEnabled != nil {
		syncEnabled = *input.SyncEnabled
	}

	if existing != nil {
		existing.BaseURL = baseURL
		existing.SyncEnabled = syncEnabled
		if input.APIKey != "" {
			enc, err := EncryptCredential(jwtSecret, input.APIKey)
			if err != nil {
				return nil, err
			}
			existing.APIKeyEncrypted = enc
		}
		if err := db.Save(existing).Error; err != nil {
			return nil, err
		}
		return existing, nil
	}

	if input.APIKey == "" {
		return nil, fmt.Errorf("immich config: an API key is required when first connecting Immich")
	}
	enc, err := EncryptCredential(jwtSecret, input.APIKey)
	if err != nil {
		return nil, err
	}
	created := models.ImmichConfig{
		UserID:          userID,
		BaseURL:         baseURL,
		APIKeyEncrypted: enc,
		SyncEnabled:     syncEnabled,
	}
	if err := db.Create(&created).Error; err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteImmichConfig removes a user's ImmichConfig (soft delete). Their
// ExternalIdentity/ExternalActivity history is kept — unlinking the
// connection does not forget the timeline.
func DeleteImmichConfig(db *gorm.DB, userID uint) error {
	return db.Where("user_id = ?", userID).Delete(&models.ImmichConfig{}).Error
}

// buildImmichClient loads the user's config, decrypts the API key, and
// constructs an ImmichClient. Any failure is mapped to a sentinel error.
func buildImmichClient(db *gorm.DB, cfg config.Config, userID uint) (*ImmichClient, *models.ImmichConfig, error) {
	ic, err := GetImmichConfigForUser(db, userID)
	if err != nil {
		return nil, nil, err
	}
	if ic == nil {
		return nil, nil, fmt.Errorf("%w: no Immich connection configured", ErrImmichUnauthorized)
	}
	apiKey, err := DecryptCredential(cfg.JWTSecretKey, ic.APIKeyEncrypted)
	if err != nil {
		return nil, nil, err
	}
	client, err := NewImmichClient(ic.BaseURL, apiKey, cfg.ImmichBlockPrivateURLs)
	if err != nil {
		return nil, nil, err
	}
	return client, ic, nil
}

// TestImmichConnection checks the current user's saved Immich connection in
// two stages — reachability (Ping, unauthenticated) then auth (GetMyUser) —
// so a failure is diagnosed rather than just reported. A Go error here means
// the check itself could not run (no connection configured, stored URL
// unparseable); the controller maps that through abortImmichServiceError
// exactly like every other Immich endpoint. A populated, non-error result
// with OK:false is a *successful* diagnosis of an upstream problem, not an
// application error — this always responds 200.
func TestImmichConnection(db *gorm.DB, cfg config.Config, userID uint) (*ImmichConnectionTestResult, error) {
	client, _, err := buildImmichClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(); err != nil {
		// Ping is expected to be unauthenticated on real Immich, but if a
		// proxy or version in front of it gates on the key anyway, an
		// unauthorized response is still an auth problem, not a
		// reachability one — classify by the actual sentinel, not by which
		// call surfaced it.
		stage := "reachability"
		if errors.Is(err, ErrImmichUnauthorized) {
			stage = "auth"
		}
		logger.Warn().Err(err).Uint("user_id", userID).Str("stage", stage).Msg("Immich test connection: ping failed")
		return diagnoseImmichConnectionFailure(stage, err), nil
	}

	user, err := client.GetMyUser()
	if err != nil {
		logger.Warn().Err(err).Uint("user_id", userID).Msg("Immich test connection: auth failed")
		return diagnoseImmichConnectionFailure("auth", err), nil
	}

	logger.Info().Uint("user_id", userID).Msg("Immich test connection: ok")
	return &ImmichConnectionTestResult{
		OK:      true,
		Stage:   "ok",
		Message: fmt.Sprintf("Connected to Immich as %s", user.Email),
	}, nil
}

// diagnoseImmichConnectionFailure turns a sentinel error from the client
// into a specific, stage-appropriate message for the user.
func diagnoseImmichConnectionFailure(stage string, err error) *ImmichConnectionTestResult {
	message := err.Error()
	switch {
	case errors.Is(err, ErrImmichPrivateAddress):
		message = "The Immich URL resolves to a private or loopback address, which this server is configured to block (IMMICH_BLOCK_PRIVATE_URLS)."
	case errors.Is(err, ErrImmichUnauthorized):
		message = "Immich rejected the API key. Check that it hasn't been revoked, expired, or mistyped."
	case errors.Is(err, ErrImmichUnreachable):
		message = fmt.Sprintf("Could not reach the Immich server: %v", err)
	}
	return &ImmichConnectionTestResult{OK: false, Stage: stage, Message: message}
}

// ListImmichPeopleForUser browses every person in the user's Immich instance
// (the L1 picker). Requires a configured connection.
func ListImmichPeopleForUser(db *gorm.DB, cfg config.Config, userID uint) ([]ImmichPerson, error) {
	client, _, err := buildImmichClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}
	return client.ListPeople()
}

// LinkImmichPerson persists a person link as an ExternalIdentity (system:
// "immich") — the whole reason T14's substrate comes first. The deep-link URL
// points at the Immich person page. A duplicate (system, person, user) link
// is surfaced to the caller as an ErrImmichNotFound-free ErrAlreadyExists via
// the controller's own check.
func LinkImmichPerson(db *gorm.DB, cfg config.Config, userID uint, contactUID, personID, personName string) (*models.ExternalIdentity, error) {
	ic, err := GetImmichConfigForUser(db, userID)
	if err != nil {
		return nil, err
	}
	if ic == nil {
		return nil, fmt.Errorf("%w: no Immich connection configured", ErrImmichUnauthorized)
	}

	identity := models.ExternalIdentity{
		UserID:     userID,
		EntityID:   contactUID,
		System:     ExternalSystemImmich,
		ExternalID: personID,
		URL:        strings.TrimRight(ic.BaseURL, "/") + "/people/" + personID,
		Metadata: map[string]interface{}{
			"person_name": personName,
		},
		SyncStatus: models.ExternalIdentitySyncStatusIdle,
	}
	if err := db.Create(&identity).Error; err != nil {
		return nil, err
	}
	return &identity, nil
}

// FetchImmichPersonSummary returns a linked person's live summary (photo
// count + latest appearance) for the contact page. On any upstream failure it
// degrades to the identity's cached metadata (photo_count / latest_at from
// the last successful sync) rather than erroring the contact page (T16 trap:
// "an Immich instance that is down must degrade to no-photos rather than
// erroring").
func FetchImmichPersonSummary(db *gorm.DB, cfg config.Config, userID uint, identity *models.ExternalIdentity) *ImmichPersonSummary {
	summary := &ImmichPersonSummary{
		Identity: *identity,
	}
	if name, ok := identity.Metadata["person_name"].(string); ok {
		summary.PersonName = name
	}
	// Seed from the cache first so any live-fetch failure degrades to the
	// last successful sync instead of zeros.
	if count, ok := numericMetadata(identity.Metadata["photo_count"]); ok {
		summary.PhotoCount = count
	}
	if id, ok := identity.Metadata["latest_asset_id"].(string); ok {
		summary.LatestAssetID = id
	}
	if at, ok := parseMetadataTime(identity.Metadata["latest_at"]); ok {
		summary.LatestAt = &at
	}

	client, _, err := buildImmichClient(db, cfg, userID)
	if err != nil {
		return summary
	}

	count, err := client.GetStatistics(identity.ExternalID)
	if err != nil {
		return summary
	}
	summary.PhotoCount = count

	assets, err := client.RecentAssets(identity.ExternalID, 1)
	if err != nil {
		return summary
	}
	if len(assets) > 0 {
		summary.LatestAssetID = assets[0].ID
		at := assetOccurredAt(&assets[0])
		summary.LatestAt = &at
	}
	return summary
}

// numericMetadata coerces a metadata value (JSON numbers arrive as float64)
// to an int, mirroring how the sync caches photo_count.
func numericMetadata(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

// parseMetadataTime parses a cached timestamp (RFC3339 string) from metadata.
func parseMetadataTime(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// SyncImmichForUser is the T16 enrichment sync for one user: for every linked
// Immich person, fetch statistics and recent appearances, upsert
// ExternalActivity rows (system: immich, type: photo-appearance) so they land
// on the contact's timeline, and refresh the identity's cached metadata.
// Failure policy: a whole-failure (unreachable instance / expired key) records
// sync_status=error and stops; a single person's failure (e.g. unlinked
// upstream) is logged and skipped so one bad person cannot take down the rest.
func SyncImmichForUser(db *gorm.DB, cfg config.Config, userID uint) error {
	ic, err := GetImmichConfigForUser(db, userID)
	if err != nil {
		return err
	}
	if ic == nil || !ic.SyncEnabled {
		return nil
	}

	client, _, err := buildImmichClient(db, cfg, userID)
	if err != nil {
		recordImmichSyncResult(db, userID, false, err)
		return err
	}

	var identities []models.ExternalIdentity
	if err := db.Where("user_id = ? AND system = ?", userID, ExternalSystemImmich).Find(&identities).Error; err != nil {
		recordImmichSyncResult(db, userID, false, err)
		return err
	}

	now := time.Now().UTC()
	var firstErr error

	for i := range identities {
		identity := &identities[i]
		if err := syncImmichPerson(db, client, userID, identity, now); err != nil {
			logger.Warn().Err(err).Uint("user_id", userID).Str("person_id", identity.ExternalID).
				Msg("Immich sync: skipping person (will retry next run)")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
	}

	recordImmichSyncResult(db, userID, firstErr == nil, firstErr)
	return firstErr
}

// syncImmichPerson syncs one linked person: fetch statistics + recent
// appearances, upsert ExternalActivity rows (deduped on the natural key), and
// refresh the identity's cached metadata + sync lifecycle.
func syncImmichPerson(db *gorm.DB, client *ImmichClient, userID uint, identity *models.ExternalIdentity, now time.Time) error {
	stats, err := client.GetStatistics(identity.ExternalID)
	if err != nil {
		return err
	}
	assets, err := client.RecentAssets(identity.ExternalID, immichSyncLimitAssets)
	if err != nil {
		return err
	}

	for _, asset := range assets {
		occurredAt := assetOccurredAt(&asset)
		var existing models.ExternalActivity
		lookupErr := db.Where("source_system = ? AND external_id = ? AND user_id = ?", ExternalSystemImmich, asset.ID, userID).First(&existing).Error
		if lookupErr == nil {
			// Already known — refresh the timestamp in case Immich moved it.
			existing.OccurredAt = occurredAt
			if err := db.Save(&existing).Error; err != nil {
				return err
			}
			continue
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}

		activity := models.ExternalActivity{
			UserID:       userID,
			EntityID:     identity.EntityID,
			SourceSystem: ExternalSystemImmich,
			ExternalID:   asset.ID,
			Type:         ImmichActivityType,
			OccurredAt:   occurredAt,
			Provenance:   models.ExternalActivityProvenanceExternal,
			SyncState:    models.ExternalActivitySyncStateSynced,
			Payload: map[string]interface{}{
				"person_id":   identity.ExternalID,
				"person_name": identity.Metadata["person_name"],
				"asset_id":    asset.ID,
			},
		}
		if err := db.Create(&activity).Error; err != nil {
			return err
		}
	}

	// Refresh the identity's cached metadata + sync lifecycle.
	var latestAt *time.Time
	var latestAssetID string
	if len(assets) > 0 {
		at := assetOccurredAt(&assets[0])
		latestAt = &at
		latestAssetID = assets[0].ID
	}
	if identity.Metadata == nil {
		identity.Metadata = map[string]interface{}{}
	}
	identity.Metadata["photo_count"] = stats
	identity.Metadata["latest_at"] = latestAt
	identity.Metadata["latest_asset_id"] = latestAssetID
	identity.SyncStatus = models.ExternalIdentitySyncStatusSynced
	identity.LastSyncedAt = &now

	return db.Save(identity).Error
}

// recordImmichSyncResult writes the connection-level sync outcome.
func recordImmichSyncResult(db *gorm.DB, userID uint, ok bool, syncErr error) {
	status := "success"
	errMsg := ""
	if !ok {
		status = "error"
		if syncErr != nil {
			errMsg = syncErr.Error()
		}
	}
	now := time.Now().UTC()
	update := map[string]interface{}{
		"last_synced_at":   &now,
		"last_sync_status": status,
		"last_sync_error":  errMsg,
	}
	if err := db.Model(&models.ImmichConfig{}).Where("user_id = ?", userID).Updates(update).Error; err != nil {
		logger.Error().Err(err).Uint("user_id", userID).Msg("Immich sync: failed to record connection status")
	}
}

// SyncImmichWithRateLimit is the scheduled entry point (T16 item 3): reuse
// the existing job-lock pattern (acquireJobLock/releaseJobLock with a
// minInterval) rather than a bare cron, so rapid restarts or multiple
// instances never double-sync. Iterates every user with an enabled config.
func SyncImmichWithRateLimit(db *gorm.DB, cfg config.Config) {
	acquired, err := acquireJobLock(db, models.JobNameImmichSync, immichSyncMinInterval)
	if err != nil {
		logger.Error().Err(err).Msg("Immich sync: failed to check job lock")
		return
	}
	if !acquired {
		logger.Info().Msg("Immich sync: skipping job - rate limited")
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameImmichSync, true); err != nil {
			logger.Error().Err(err).Msg("Immich sync: failed to release job lock")
		}
	}()

	var configs []models.ImmichConfig
	if err := db.Where("sync_enabled = ?", true).Find(&configs).Error; err != nil {
		logger.Error().Err(err).Msg("Immich sync: failed to load enabled connections")
		return
	}
	for _, ic := range configs {
		if err := SyncImmichForUser(db, cfg, ic.UserID); err != nil {
			logger.Warn().Err(err).Uint("user_id", ic.UserID).Msg("Immich sync: user sync failed")
		}
	}
}

// GetImmichIdentityForContact resolves a contact's Immich ExternalIdentity —
// the shared "does this contact have a link at all" lookup used by the
// thumbnail proxy, the summary, and the contact-photo picker.
func GetImmichIdentityForContact(db *gorm.DB, userID uint, contactUID string) (*models.ExternalIdentity, error) {
	var identity models.ExternalIdentity
	if err := db.Where("user_id = ? AND system = ? AND entity_id = ?", userID, ExternalSystemImmich, contactUID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrImmichNotFound
		}
		return nil, err
	}
	return &identity, nil
}

// FetchImmichThumbnail proxies a linked person's thumbnail image for the
// contact page (T16 item 4: prefer proxying over copying image data into this
// app). The person is resolved through the contact's ExternalIdentity, and
// the fetch runs with the user's own connection credentials. The caller
// (controller) applies the same SVG rejection + Content-Disposition/CSP
// hardening as the existing image proxy.
func FetchImmichThumbnail(db *gorm.DB, cfg config.Config, userID uint, contactUID string) ([]byte, string, error) {
	identity, err := GetImmichIdentityForContact(db, userID, contactUID)
	if err != nil {
		return nil, "", err
	}
	client, _, err := buildImmichClient(db, cfg, userID)
	if err != nil {
		return nil, "", err
	}
	return client.Thumbnail(identity.ExternalID)
}

// ImmichAssetSummary is the slice of an Immich asset the contact-photo picker
// needs to render a candidate: enough to display and to fetch the image via
// GetImmichAssetImage.
type ImmichAssetSummary struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
}

// immichPhotoPickerAssetLimit bounds how many recent photos the contact-photo
// picker offers to browse — enough to be useful, small enough that fetching
// each one's thumbnail (the picker renders them as a grid) stays cheap.
const immichPhotoPickerAssetLimit = 30

// ListImmichRecentAssetsForContact returns a linked contact's recent Immich
// photos (id + occurred_at) for the contact-photo picker (browse-then-pick,
// T16 follow-on): requires an existing link — the same trust boundary as
// FetchImmichThumbnail, reusing the identity lookup.
func ListImmichRecentAssetsForContact(db *gorm.DB, cfg config.Config, userID uint, contactUID string) ([]ImmichAssetSummary, error) {
	identity, err := GetImmichIdentityForContact(db, userID, contactUID)
	if err != nil {
		return nil, err
	}
	client, _, err := buildImmichClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}
	assets, err := client.RecentAssets(identity.ExternalID, immichPhotoPickerAssetLimit)
	if err != nil {
		return nil, err
	}
	summaries := make([]ImmichAssetSummary, len(assets))
	for i, asset := range assets {
		summaries[i] = ImmichAssetSummary{ID: asset.ID, OccurredAt: assetOccurredAt(&asset)}
	}
	return summaries, nil
}

// FetchImmichAssetImage proxies one of a linked contact's recent Immich
// photos (the contact-photo picker's "load this candidate photo" step). Gated
// on the contact having an existing link at all — like FetchImmichThumbnail,
// this fetches under the user's own Immich credentials for their own
// contact, so an arbitrary assetID under that key is "can see any of my own
// Immich photos," not a cross-user access issue; the link-exists check is
// what gates the feature being reachable at all.
func FetchImmichAssetImage(db *gorm.DB, cfg config.Config, userID uint, contactUID, assetID string) ([]byte, string, error) {
	if _, err := GetImmichIdentityForContact(db, userID, contactUID); err != nil {
		return nil, "", err
	}
	client, _, err := buildImmichClient(db, cfg, userID)
	if err != nil {
		return nil, "", err
	}
	return client.AssetThumbnail(assetID)
}
