package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/fireandforget"
	"mycorrhizal/meerkatapi"
	"mycorrhizal/models"
	"mycorrhizal/photostore"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// This file is the live-API half of the Meerkat import assistant (issue
// #2013): connect to a running Meerkat with a token or a username/password,
// detect its version by feature probing, pull the account through the REST
// API in the background, and hand the resulting meerkat.Snapshot to the same
// mapper, preview, loss report and apply engine the uploaded-database path
// uses (meerkat_import_session.go). Photos — which the DB file cannot carry —
// are downloaded after the import commits and re-encoded by photostore.
//
// Credentials live only in the session's meerkatapi.Client (memory), are
// never logged or echoed, and the Meerkat session is logged out when the
// import finishes, is cancelled, or the session expires (closeClient).

const (
	// meerkatConnectTimeout bounds Connect's handful of probe requests so it
	// never outlives the browser's wait and strands a credential-holding
	// session.
	meerkatConnectTimeout = 60 * time.Second
	// meerkatPhotoTimeoutPerContact + base bound the post-commit photo tail.
	meerkatPhotoTimeoutPerContact = 3 * time.Second
	meerkatPhotoTimeoutBase       = 60 * time.Second
)

func userSafeMeerkatAPIError(err error) string {
	switch {
	case errors.Is(err, meerkatapi.ErrUnauthorized):
		return "Meerkat rejected the credentials"
	case errors.Is(err, meerkatapi.ErrLocked):
		return "Meerkat has temporarily locked this account after failed logins — wait and try again"
	case errors.Is(err, meerkatapi.ErrInvalidURL):
		return "The Meerkat URL is invalid"
	case errors.Is(err, meerkatapi.ErrPrivateAddress):
		return "The Meerkat URL resolves to a private address, which this server blocks"
	case errors.Is(err, meerkatapi.ErrNotMeerkat):
		return "The server did not respond like a Meerkat instance — check the URL"
	case errors.Is(err, meerkatapi.ErrInvalidData):
		return "The server sent a response the importer could not read — check the URL, or use the database upload"
	case errors.Is(err, meerkatapi.ErrTooLarge):
		return "This Meerkat account is larger than the import supports"
	default:
		return "The Meerkat instance could not be reached — if it is offline, use the database upload instead"
	}
}

func meerkatAPIAppError(err error) *apperrors.AppError {
	switch {
	case errors.Is(err, meerkatapi.ErrUnauthorized), errors.Is(err, meerkatapi.ErrLocked):
		return apperrors.ErrInvalidInput("credentials", userSafeMeerkatAPIError(err))
	case errors.Is(err, meerkatapi.ErrInvalidURL), errors.Is(err, meerkatapi.ErrPrivateAddress),
		errors.Is(err, meerkatapi.ErrNotMeerkat), errors.Is(err, meerkatapi.ErrTooLarge):
		return apperrors.ErrInvalidInput("base_url", userSafeMeerkatAPIError(err))
	default:
		return apperrors.ErrExternal("meerkat", userSafeMeerkatAPIError(err)).WithError(err)
	}
}

// closeClient logs the Meerkat session out and wipes the credential. It is
// idempotent and never blocks the caller (the logout is best effort).
func (s *meerkatImportSession) closeClient() {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return
	}
	fireandforget.Run(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client.Close(ctx)
	})
}

// Connect validates the URL and credentials, detects the Meerkat version,
// counts contacts, and opens a session. Nothing is read beyond a /health
// probe, the user, and one contact page.
func (m *MeerkatImportManager) Connect(ctx context.Context, userID uint, req models.MeerkatConnectRequest, cfg *config.Config) (*models.MeerkatConnectResponse, *apperrors.AppError) {
	token := strings.TrimSpace(req.APIToken)
	hasLogin := req.Username != "" && req.Password != ""
	if token == "" && !hasLogin {
		return nil, apperrors.ErrInvalidInput("credentials", "Enter a Meerkat API token, or a username and password")
	}
	client, err := meerkatapi.NewClient(req.BaseURL, cfg.MeerkatBlockPrivateURLs)
	if err != nil {
		return nil, meerkatAPIAppError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, meerkatConnectTimeout)
	defer cancel()

	// Every failure path below must drop the credential.
	ok := false
	defer func() {
		if !ok {
			client.Close(context.Background())
		}
	}()

	// A host that does not answer like Meerkat gets no credential at all.
	if herr := client.CheckHealth(ctx); herr != nil {
		return nil, meerkatAPIAppError(herr)
	}
	if token != "" {
		client.SetToken(token)
	} else if lerr := client.Login(ctx, req.Username, req.Password); lerr != nil {
		return nil, meerkatAPIAppError(lerr)
	}
	version, err := client.Detect(ctx)
	if err != nil {
		return nil, meerkatAPIAppError(err)
	}
	me, err := client.Me(ctx)
	if err != nil {
		return nil, meerkatAPIAppError(err)
	}
	total, err := client.CountContacts(ctx)
	if err != nil {
		return nil, meerkatAPIAppError(err)
	}
	if total > MaxMeerkatContacts {
		return nil, apperrors.ErrInvalidInput("base_url",
			fmt.Sprintf("This Meerkat account has %d contacts; the import supports at most %d", total, MaxMeerkatContacts))
	}

	sessionID := generateSessionID()
	now := Now()
	meID := me.ID
	session := &meerkatImportSession{
		id:           sessionID,
		userID:       userID,
		client:       client,
		version:      version,
		me:           me,
		photoDir:     cfg.ProfilePhotoDir,
		sourceUserID: &meID,
		phase:        models.SourceImportPhaseConnecting,
		phaseTotal:   total,
		expiresAt:    now.Add(meerkatSessionExpiry),
		hardExpiry:   now.Add(meerkatSessionMaxLifetime),
	}
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()
	ok = true

	return &models.MeerkatConnectResponse{
		SessionID: sessionID,
		Version: models.MeerkatVersionInfo{
			Generation: version.Generation, Supported: version.Supported, Reason: version.Reason,
			ReasonCode: version.ReasonCode,
		},
		SourceUser:            models.MeerkatSourceUser{ID: me.ID, Username: me.Username, Email: me.Email, Contacts: total},
		Totals:                models.MeerkatEntityCounts{Contacts: total},
		InsecureTransport:     client.InsecureRemote(),
		EstimatedFetchSeconds: (total+99)/100 + 1,
	}, nil
}

// startAPIFetch launches the background account fetch → map → preview build.
// The session lock is already released by the caller.
func (m *MeerkatImportManager) startAPIFetch(db *gorm.DB, s *meerkatImportSession, req models.MeerkatFetchRequest, log *zerolog.Logger) *apperrors.AppError {
	s.mu.Lock()
	if s.phase != models.SourceImportPhaseConnecting && s.phase != models.SourceImportPhaseFailed {
		s.mu.Unlock()
		return apperrors.ErrConflict("This import is already being prepared")
	}
	if !s.version.Supported && !req.AcceptUntested {
		s.mu.Unlock()
		return apperrors.ErrInvalidInput("accept_untested",
			"This Meerkat version is not tested with the importer. Confirm to try anyway; nothing is written until you review the preview.")
	}
	s.phase = models.SourceImportPhaseFetchingContacts
	s.phaseDone, s.errMsg = 0, ""
	s.plan, s.previews, s.snapshot = nil, nil, nil
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	client, me := s.client, s.me
	s.mu.Unlock()

	fireandforget.Run(func() { m.runAPIFetch(ctx, db, s, client, me, log) })
	return nil
}

func (m *MeerkatImportManager) runAPIFetch(ctx context.Context, db *gorm.DB, s *meerkatImportSession, client *meerkatapi.Client, me meerkatapi.Me, log *zerolog.Logger) {
	snap, err := client.FetchSnapshot(ctx, me, meerkatapi.Limits{MaxContacts: MaxMeerkatContacts, MaxEntities: MaxMeerkatEntities},
		func(done, total int) { s.setProgress(done, total) })
	if err != nil {
		if ctx.Err() != nil {
			return // cancelled: the session is being torn down
		}
		log.Warn().Err(err).Str("session_id", s.id).Msg("Meerkat API fetch failed")
		s.fail(userSafeMeerkatAPIError(err))
		return
	}
	s.mu.Lock()
	s.snapshot = snap
	sourceUserID := s.sourceUserID
	s.mu.Unlock()
	m.runFetch(ctx, db, s, sourceUserID, log)
}

// meerkatPhotoTask is one deferred profile-picture download.
type meerkatPhotoTask struct {
	contactID uint
	sourceID  int64
}

// photoTasks lists the contacts whose pictures should be downloaded after the
// commit. Only API sessions have a client to download with. Merge targets are
// in refToID too, so contacts that already have a photo are filtered out: the
// import never replaces a destination photo.
func (m *MeerkatImportManager) photoTasks(db *gorm.DB, s *meerkatImportSession, plan *ImportSourcePlan, refToID map[string]uint, log *zerolog.Logger) []meerkatPhotoTask {
	if s.client == nil || s.photoDir == "" {
		return nil
	}
	ids := make([]uint, 0, len(refToID))
	for _, id := range refToID {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	var photoless []uint
	if err := db.Model(&models.Contact{}).
		Where("id IN ? AND user_id = ? AND (photo IS NULL OR photo = '')", ids, s.userID).
		Pluck("id", &photoless).Error; err != nil {
		// Fail safe: without knowing which contacts are empty, queue nothing.
		log.Warn().Err(err).Str("session_id", s.id).Msg("Meerkat photo eligibility lookup failed; skipping pictures")
		return nil
	}
	eligible := make(map[uint]bool, len(photoless))
	for _, id := range photoless {
		eligible[id] = true
	}
	var tasks []meerkatPhotoTask
	for i := range plan.Contacts {
		mc := &plan.Contacts[i]
		if mc.PhotoURL == "" {
			continue
		}
		id, ok := refToID[mc.Ref.ExternalID]
		if !ok || !eligible[id] {
			continue
		}
		srcID, err := strconv.ParseInt(strings.TrimPrefix(mc.Ref.ExternalID, "contact/"), 10, 64)
		if err != nil {
			continue
		}
		tasks = append(tasks, meerkatPhotoTask{contactID: id, sourceID: srcID})
	}
	return tasks
}

// processPhotos downloads and stores contact pictures after the import
// transaction commits. A per-photo failure is logged and counted, never
// fatal. The bytes are untrusted: photostore validates and re-encodes them
// exactly as it does for an upload.
func (m *MeerkatImportManager) processPhotos(db *gorm.DB, s *meerkatImportSession, tasks []meerkatPhotoTask, log *zerolog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), meerkatPhotoTimeoutBase+time.Duration(len(tasks))*meerkatPhotoTimeoutPerContact)
	defer cancel()
	s.mu.Lock()
	s.cancel = cancel // lets Cancel / expiry stop the tail
	client, photoDir := s.client, s.photoDir
	s.mu.Unlock()
	defer s.closeClient()

	for i, task := range tasks {
		saved := false
		if ctx.Err() == nil {
			data, mediaType, err := client.FetchProfilePicture(ctx, task.sourceID)
			if err != nil {
				log.Warn().Err(err).Uint("contact_id", task.contactID).Msg("Failed to fetch Meerkat profile picture")
			} else if photoPath, thumb, saveErr := photostore.SaveContactPhoto(data, mediaType, photoDir); saveErr != nil {
				log.Warn().Err(saveErr).Uint("contact_id", task.contactID).Msg("Failed to save Meerkat profile picture")
			} else if photoPath != "" {
				// Scoped column update only, ownership-scoped (backend traps #3, #5).
				// The empty-photo guard is a race backstop: a photo that landed
				// since the task was queued is never replaced.
				res := db.Model(&models.Contact{}).
					Where("id = ? AND user_id = ? AND (photo IS NULL OR photo = '')", task.contactID, s.userID).
					Updates(map[string]any{"photo": photoPath, "photo_thumbnail": thumb})
				if res.Error != nil || res.RowsAffected == 0 {
					log.Warn().Err(res.Error).Uint("contact_id", task.contactID).Msg("Failed to attach Meerkat profile picture")
					removeOrphanedPhoto(photoDir, photoPath)
				} else {
					saved = true
				}
			}
		}
		s.mu.Lock()
		if s.result != nil {
			if saved {
				s.result.PhotosSaved++
			} else {
				s.result.PhotosFailed++
			}
		}
		s.phaseDone = i + 1
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.phase = models.SourceImportPhaseDone
	s.mu.Unlock()
}
