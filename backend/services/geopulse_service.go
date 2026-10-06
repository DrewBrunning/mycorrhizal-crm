package services

import (
	"context"
	"errors"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	// GeoPulseStayRefPrefix namespaces the Activity.ExternalRef a confirmed stay
	// carries (`geopulse:stay:<id>`). It is also what makes confirming
	// idempotent: see FindActivityByExternalRef.
	GeoPulseStayRefPrefix = "geopulse:stay:"

	// GeoPulsePhotoRadiusMeters is the radius passed to GeoPulse's photo search
	// (ADR 0033 amendment, 2026-10-01): a stay is a clustered place, photos land
	// within a building or yard of its centroid, GPS error is tens of metres, and
	// past ~500 m neighbouring stays bleed in.
	GeoPulsePhotoRadiusMeters = 200

	// geopulsePhotosPerStay bounds one stay's photo list; it is display-only
	// context, not an export.
	geopulsePhotosPerStay = 12

	// geopulseMaxStayLookups bounds how many stays get a photo lookup in one
	// request, so a pathological day cannot fan out into hundreds of outbound
	// calls. Stays past the cap are still suggested, without photos.
	geopulseMaxStayLookups = 50

	// geopulseSuggestionsBudget is the overall wall-clock budget of one
	// suggestions request (timeline call + every photo lookup). It sits under
	// nginx's 30 s proxy_read_timeout so a slow-but-alive GeoPulse degrades to
	// "photos unavailable" instead of a proxy 504 while the backend keeps
	// fanning out for minutes after the client left.
	geopulseSuggestionsBudgetDefault = 20 * time.Second
)

// geopulseSuggestionsBudget is a var so a test can shrink it.
var geopulseSuggestionsBudget = geopulseSuggestionsBudgetDefault

var (
	// ErrGeoPulseConfigUnreadable wraps a failure to load the user's stored
	// connection: a local database fault, not anything GeoPulse did.
	ErrGeoPulseConfigUnreadable = errors.New("GeoPulse data could not be loaded")
	// ErrGeoPulseKeyUndecryptable wraps a failure to decrypt the stored API token
	// — typically a rotated JWT_SECRET_KEY — which the user fixes by re-entering it.
	ErrGeoPulseKeyUndecryptable = errors.New("stored GeoPulse API token could not be decrypted")
)

// ErrGeoPulseTokenRequired is returned when the base URL moves to a different
// origin without a freshly entered API token.
var ErrGeoPulseTokenRequired = errors.New("re-enter the API token when changing the GeoPulse server")

// ErrGeoPulseInvalidDate is returned for an unparseable date or timezone.
var ErrGeoPulseInvalidDate = errors.New("GeoPulse date must be YYYY-MM-DD and timezone a valid IANA name")

// GeoPulseConfigResponse is the API-visible shape of a GeoPulseConfig: the
// encrypted API token is never exposed, only whether one is stored.
type GeoPulseConfigResponse struct {
	BaseURL   string `json:"base_url"`
	HasAPIKey bool   `json:"has_api_key"`
}

// GeoPulseConnectionTestResult is the outcome of "Test connection" (Settings): a
// stage-by-stage diagnosis. Stage is one of "reachability", "auth", or "ok".
type GeoPulseConnectionTestResult struct {
	OK      bool   `json:"ok"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// GeoPulsePhotoSuggestion is one photo shown alongside a stay. Display-only: it
// is never written to the Activity (ADR 0033).
type GeoPulsePhotoSuggestion struct {
	ID       string `json:"id"`
	FileName string `json:"file_name"`
	TakenAt  string `json:"taken_at"`
}

// GeoPulseStaySuggestion is one ephemeral, unpersisted "you were here" suggestion.
// Nothing is written until the user confirms it through POST /activities, which
// they pre-fill from Location/Date/ExternalRef. No contact is ever inferred.
type GeoPulseStaySuggestion struct {
	StayID          int64                     `json:"stay_id"`
	ExternalRef     string                    `json:"external_ref"`
	Location        string                    `json:"location"`
	City            string                    `json:"city"`
	Country         string                    `json:"country"`
	Latitude        float64                   `json:"latitude"`
	Longitude       float64                   `json:"longitude"`
	Timestamp       time.Time                 `json:"timestamp"`
	DurationSeconds int64                     `json:"duration_seconds"`
	Photos          []GeoPulsePhotoSuggestion `json:"photos"`
	// PhotosUnavailable is true when the photo lookup failed or was skipped, so
	// the UI can say "no photos" apart from "couldn't check".
	PhotosUnavailable bool `json:"photos_unavailable"`
	// ExistingActivityID is set when an Activity already carries this stay's
	// ExternalRef, so the UI can show it as already logged.
	ExistingActivityID *uint `json:"existing_activity_id,omitempty"`
}

// GeoPulseSuggestionsResponse is the response of the suggestions endpoint.
type GeoPulseSuggestionsResponse struct {
	Date        string                   `json:"date"`
	Suggestions []GeoPulseStaySuggestion `json:"suggestions"`
}

// GetGeoPulseConfigForUser returns the user's GeoPulseConfig, or nil when they
// have not set one up.
func GetGeoPulseConfigForUser(db *gorm.DB, userID uint) (*models.GeoPulseConfig, error) {
	var cfg models.GeoPulseConfig
	if err := db.Where("user_id = ?", userID).First(&cfg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

// NormalizeGeoPulseBaseURL validates and sanitizes a user-supplied GeoPulse base
// URL at save time (trim, parse, require an explicit http/https scheme + host),
// rejecting malformed input rather than auto-repairing it.
func NormalizeGeoPulseBaseURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", ErrGeoPulseInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrGeoPulseInvalidURL
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// UpsertGeoPulseConfig creates or updates a user's GeoPulseConfig. A non-empty
// APIKey is encrypted at rest (credential_crypto.go); an empty one on update
// keeps the stored token. On create the token is required.
//
// Credential-exfiltration guard: an empty token on an update that moves the
// base URL to a different origin (scheme+host+port) is rejected, otherwise a
// hijacked session could point the saved token at an attacker host and press
// "Test connection". A path-only change on the same origin keeps the token.
func UpsertGeoPulseConfig(db *gorm.DB, jwtSecret string, userID uint, input models.GeoPulseConfigInput) (*models.GeoPulseConfig, error) {
	existing, err := GetGeoPulseConfigForUser(db, userID)
	if err != nil {
		return nil, err
	}

	baseURL, err := NormalizeGeoPulseBaseURL(input.BaseURL)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if input.APIKey == "" && !SameOrigin(existing.BaseURL, baseURL) {
			return nil, ErrGeoPulseTokenRequired
		}
		existing.BaseURL = baseURL
		if input.APIKey != "" {
			enc, err := EncryptCredential(jwtSecret, input.APIKey)
			if err != nil {
				return nil, err // # pragma: no cover — AES-GCM construction over a fixed-size derived key cannot fail; the error path exists only because the signature returns one
			}
			existing.APIKeyEncrypted = enc
		}
		if err := db.Save(existing).Error; err != nil {
			return nil, err
		}
		return existing, nil
	}

	if input.APIKey == "" {
		return nil, fmt.Errorf("geopulse config: an API token is required when first connecting GeoPulse")
	}
	enc, err := EncryptCredential(jwtSecret, input.APIKey)
	if err != nil {
		return nil, err // # pragma: no cover — see the update path above
	}
	created := models.GeoPulseConfig{UserID: userID, BaseURL: baseURL, APIKeyEncrypted: enc}
	if err := db.Create(&created).Error; err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteGeoPulseConfig removes a user's GeoPulseConfig (soft delete). Activities
// already confirmed from GeoPulse stays are ordinary Activities and are kept.
func DeleteGeoPulseConfig(db *gorm.DB, userID uint) error {
	return db.Where("user_id = ?", userID).Delete(&models.GeoPulseConfig{}).Error
}

// buildGeoPulseClient loads the user's config, decrypts the API token, and
// constructs a GeoPulseClient. A missing connection is reported as unauthorized
// (the caller's own setup, not a server fault — issue #524's convention).
func buildGeoPulseClient(db *gorm.DB, cfg config.Config, userID uint) (*GeoPulseClient, error) {
	gc, err := GetGeoPulseConfigForUser(db, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGeoPulseConfigUnreadable, err)
	}
	if gc == nil {
		return nil, fmt.Errorf("%w: no GeoPulse connection configured", ErrGeoPulseUnauthorized)
	}
	token, err := DecryptCredential(cfg.JWTSecretKey, gc.APIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGeoPulseKeyUndecryptable, err)
	}
	return NewGeoPulseClient(gc.BaseURL, token, cfg.GeoPulseBlockPrivateURLs)
}

// TestGeoPulseConnection checks the user's saved connection (GET /api/users/me:
// reachability and token validity in one call) and diagnoses a failure. A Go
// error means the check itself could not run (no connection configured, stored
// URL unparseable); a non-error result with OK:false is a *successful* diagnosis
// of an upstream problem.
func TestGeoPulseConnection(ctx context.Context, db *gorm.DB, cfg config.Config, userID uint) (*GeoPulseConnectionTestResult, error) {
	client, err := buildGeoPulseClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}

	user, err := client.GetMe(ctx)
	if err != nil {
		stage := "reachability"
		if errors.Is(err, ErrGeoPulseUnauthorized) {
			stage = "auth"
		}
		logger.Warn().Err(err).Uint("user_id", userID).Str("stage", stage).Msg("GeoPulse test connection failed")
		return diagnoseGeoPulseConnectionFailure(stage, err), nil
	}

	logger.Info().Uint("user_id", userID).Msg("GeoPulse test connection: ok")
	name := user.FullName
	if name == "" {
		name = user.UserID
	}
	return &GeoPulseConnectionTestResult{OK: true, Stage: "ok", Message: fmt.Sprintf("Connected to GeoPulse as %s", name)}, nil
}

// diagnoseGeoPulseConnectionFailure turns a client sentinel error into a
// specific, stage-appropriate message for the user.
func diagnoseGeoPulseConnectionFailure(stage string, err error) *GeoPulseConnectionTestResult {
	message := err.Error()
	switch {
	case errors.Is(err, ErrGeoPulsePrivateAddress):
		message = "The GeoPulse URL resolves to a private or loopback address, which this server is configured to block (GEOPULSE_BLOCK_PRIVATE_URLS)."
	case errors.Is(err, ErrGeoPulseUnauthorized):
		message = "GeoPulse rejected the API token. Check that it hasn't been revoked, expired, or mistyped."
	case errors.Is(err, ErrGeoPulseRedirect):
		message = "GeoPulse answered with a redirect — check the base URL (http vs https, path). Redirects are not followed because they would forward your API token."
	case errors.Is(err, ErrGeoPulseUnreachable):
		message = fmt.Sprintf("Could not reach the GeoPulse server: %v", err)
	case errors.Is(err, ErrGeoPulseNotFound):
		message = "The GeoPulse server is reachable, but did not recognise the request. Check the base URL and the GeoPulse version."
	case errors.Is(err, ErrGeoPulseRequestFailed):
		status := "an unexpected status"
		var reqErr *GeoPulseRequestError
		if errors.As(err, &reqErr) {
			status = reqErr.Status
		}
		message = fmt.Sprintf("The GeoPulse server is reachable, but this request failed (%s).", status)
	case errors.Is(err, ErrGeoPulseInvalidData):
		message = "GeoPulse returned a response that could not be parsed. The API may have changed — check GeoPulse version compatibility."
	}
	return &GeoPulseConnectionTestResult{OK: false, Stage: stage, Message: message}
}

// geopulseDayBounds returns the [start, end] instants of a calendar date in the
// given IANA timezone (UTC when empty). end is the last second of the day, so
// consecutive days never overlap.
func geopulseDayBounds(date, timezone string) (time.Time, time.Time, error) {
	loc := time.UTC
	if timezone != "" {
		l, err := time.LoadLocation(timezone)
		if err != nil {
			return time.Time{}, time.Time{}, ErrGeoPulseInvalidDate
		}
		loc = l
	}
	day, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, time.Time{}, ErrGeoPulseInvalidDate
	}
	return day, day.AddDate(0, 0, 1).Add(-time.Second), nil
}

// GeoPulseSuggestionsForDate is the "log activity from location history" lookup:
// one synchronous GeoPulse timeline call for the day, then one synchronous photo
// search per stay. The result is ephemeral — nothing is written. A failed timeline
// call fails the request; a failed photo lookup only marks that and the remaining
// stays PhotosUnavailable, because photos are context, not the point.
func GeoPulseSuggestionsForDate(ctx context.Context, db *gorm.DB, cfg config.Config, userID uint, date, timezone string) (*GeoPulseSuggestionsResponse, error) {
	start, end, err := geopulseDayBounds(date, timezone)
	if err != nil {
		return nil, err
	}
	client, err := buildGeoPulseClient(db, cfg, userID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, geopulseSuggestionsBudget)
	defer cancel()
	stays, err := client.GetStays(ctx, start, end)
	if err != nil {
		return nil, err
	}

	refs := make([]string, 0, len(stays))
	for _, s := range stays {
		refs = append(refs, geopulseStayRef(s.ID))
	}
	existing, err := activityIDsByExternalRef(db, userID, refs)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGeoPulseConfigUnreadable, err)
	}

	out := &GeoPulseSuggestionsResponse{Date: date, Suggestions: make([]GeoPulseStaySuggestion, 0, len(stays))}

	var gpUserID string
	photosOK := true
	lookups := 0
	for _, s := range stays {
		ts, perr := time.Parse(time.RFC3339, s.Timestamp)
		if perr != nil {
			return nil, fmt.Errorf("%w: stay %d has an unparseable timestamp %q", ErrGeoPulseInvalidData, s.ID, s.Timestamp)
		}
		ref := geopulseStayRef(s.ID)
		sug := GeoPulseStaySuggestion{
			StayID:          s.ID,
			ExternalRef:     ref,
			Location:        geopulseLocationLabel(s),
			City:            s.City,
			Country:         s.Country,
			Latitude:        s.Latitude,
			Longitude:       s.Longitude,
			Timestamp:       ts,
			DurationSeconds: s.StayDuration,
			Photos:          []GeoPulsePhotoSuggestion{},
		}
		if id, ok := existing[ref]; ok {
			id := id
			sug.ExistingActivityID = &id
		}

		photosChecked := false
		if photosOK && lookups < geopulseMaxStayLookups {
			lookups++
			if gpUserID == "" {
				me, merr := client.GetMe(ctx)
				if merr != nil {
					logger.Warn().Err(merr).Uint("user_id", userID).Msg("GeoPulse photo lookup skipped: could not resolve GeoPulse user id")
					photosOK = false
				} else {
					gpUserID = me.UserID
				}
			}
			if photosOK {
				photos, perr := client.SearchPhotos(ctx, gpUserID, s.Latitude, s.Longitude, GeoPulsePhotoRadiusMeters, start, end, geopulsePhotosPerStay)
				if perr != nil {
					// Once the overall budget is spent every further lookup fails instantly
					// on the expired context, so this one branch also covers "budget exhausted".
					logger.Warn().Err(perr).Uint("user_id", userID).Bool("budget_exhausted", ctx.Err() != nil).Msg("GeoPulse photo lookup failed; remaining stays are suggested without photos")
					photosOK = false
				} else {
					photosChecked = true
					for _, p := range photos {
						sug.Photos = append(sug.Photos, GeoPulsePhotoSuggestion{ID: p.ID, FileName: p.OriginalFileName, TakenAt: p.TakenAt})
					}
				}
			}
		}
		sug.PhotosUnavailable = !photosChecked
		out.Suggestions = append(out.Suggestions, sug)
	}
	return out, nil
}

func geopulseStayRef(id int64) string {
	return GeoPulseStayRefPrefix + strconv.FormatInt(id, 10)
}

// geopulseLocationLabel is the Activity.Location a stay pre-fills: its place
// name when GeoPulse has one, falling back to the city.
func geopulseLocationLabel(s GeoPulseStay) string {
	if s.LocationName != "" {
		return s.LocationName
	}
	return s.City
}

// activityIDsByExternalRef maps each of refs that an Activity of this user
// already carries to that Activity's id.
func activityIDsByExternalRef(db *gorm.DB, userID uint, refs []string) (map[string]uint, error) {
	out := map[string]uint{}
	if len(refs) == 0 {
		return out, nil
	}
	var rows []models.Activity
	if err := db.Select("id", "external_ref").Where("user_id = ? AND external_ref IN ?", userID, refs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ExternalRef] = r.ID
	}
	return out, nil
}

// FindActivityByExternalRef returns the user's live Activity carrying ref, or nil.
// It is the lookup-before-create that makes confirming a GeoPulse stay
// idempotent (ADR 0033 amendment, 2026-10-01): dedupe is application code, not a
// unique index, so Activity's schema is untouched.
func FindActivityByExternalRef(db *gorm.DB, userID uint, ref string) (*models.Activity, error) {
	var activity models.Activity
	err := db.Preload("Contacts").Where("user_id = ? AND external_ref = ?", userID, ref).Order("id ASC").First(&activity).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &activity, nil
}
