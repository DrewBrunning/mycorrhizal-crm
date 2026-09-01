package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/monica"
	"mycorrhizal/photostore"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// This file is the Monica import assistant's server-side orchestration
// (issue #549): connect to a live Monica instance, pull a full snapshot in
// the background with progress, build a review preview that already carries
// the loss report, then confirm through the shared source-import engine
// (services.ExecuteSourceImportWithActions). It mirrors ImportSessionManager
// (import_session.go) for the file-based imports; the API token lives only in
// the in-memory session and is never logged or persisted.
//
// The mapping itself (services.MapMonicaSnapshot) and the transactional apply
// (import_source.go) are shared with the Meerkat importer — this layer only
// adds the transport, the wizard state machine, and the per-contact
// add/skip/update review that the engine's action map consumes.

const (
	// monicaSessionExpiry is longer than the CSV/VCF wizard expiry because the
	// rate-limited fetch phase alone can take several minutes. It slides on
	// every status/preview poll.
	monicaSessionExpiry = 60 * time.Minute
	// monicaSessionMaxLifetime bounds how far the sliding expiry can be
	// pushed. The session holds the API token, so a wizard left open (or kept
	// alive by a forgotten browser tab) must still age out.
	monicaSessionMaxLifetime = 6 * time.Hour
	// MaxMonicaContacts caps the account size: the full snapshot is held in
	// memory for the wizard's lifetime.
	MaxMonicaContacts = 5000
	// MaxMonicaEntities caps the whole snapshot, so an account with few
	// contacts but hundreds of thousands of notes is still bounded.
	MaxMonicaEntities = 100000
	// MaxMonicaImportSessionsPerUser bounds concurrent Monica wizards per user
	// (issue #415), matching MaxImportSessionsPerUser's intent.
	MaxMonicaImportSessionsPerUser = 3
	// monicaConnectTimeout bounds the nine serialized probe requests Connect
	// makes (~1 request/second). It must stay under the frontend's connect
	// timeout so a Connect never outlives the browser's wait and strands a
	// session holding the token.
	monicaConnectTimeout = 150 * time.Second
)

// monicaImportSession holds all server-side state of one wizard run,
// including the API client (and thus the token) — in memory only, wiped when
// the session is deleted or expires.
type monicaImportSession struct {
	id            string
	userID        uint
	client        *monica.Client
	counts        monica.EntityCounts
	includeRels   bool
	includeExtras bool
	cancel        context.CancelFunc

	mu         sync.Mutex
	phase      string
	phaseDone  int
	phaseTotal int
	errMsg     string
	plan       *ImportSourcePlan
	previews   []models.MonicaImportRowPreview
	result     *models.MonicaImportResult
	expiresAt  time.Time
	hardExpiry time.Time
}

func (s *monicaImportSession) setPhase(phase string, done, total int) {
	s.mu.Lock()
	s.phase = phase
	s.phaseDone = done
	s.phaseTotal = total
	s.mu.Unlock()
}

func (s *monicaImportSession) setProgress(done, total int) {
	s.mu.Lock()
	s.phaseDone = done
	s.phaseTotal = total
	s.mu.Unlock()
}

func (s *monicaImportSession) fail(msg string) {
	s.mu.Lock()
	s.phase = models.MonicaPhaseFailed
	s.errMsg = msg
	s.mu.Unlock()
}

// MonicaImportManager owns the lifecycle of Monica import sessions.
type MonicaImportManager struct {
	mu       sync.RWMutex
	sessions map[string]*monicaImportSession
}

// NewMonicaImportManager creates an empty manager.
func NewMonicaImportManager() *MonicaImportManager {
	return &MonicaImportManager{sessions: make(map[string]*monicaImportSession)}
}

// CleanupExpired removes expired sessions and cancels their running fetches.
// Safe to call from a goroutine.
func (m *MonicaImportManager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, s := range m.sessions {
		s.mu.Lock()
		expired := now.After(s.expiresAt) || now.After(s.hardExpiry)
		s.mu.Unlock()
		if expired {
			if s.cancel != nil {
				s.cancel()
			}
			delete(m.sessions, id)
		}
	}
}

// CountActive reports how many live sessions a user holds (issue #415).
func (m *MonicaImportManager) CountActive(userID uint) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	now := time.Now()
	for _, s := range m.sessions {
		if s.userID != userID {
			continue
		}
		s.mu.Lock()
		live := now.Before(s.expiresAt) && now.Before(s.hardExpiry)
		s.mu.Unlock()
		if live {
			n++
		}
	}
	return n
}

// get retrieves a session, enforcing ownership and sliding expiry.
func (m *MonicaImportManager) get(sessionID string, userID uint) (*monicaImportSession, *apperrors.AppError) {
	m.mu.RLock()
	s, exists := m.sessions[sessionID]
	m.mu.RUnlock()
	if !exists {
		return nil, apperrors.ErrNotFound("Import session expired or not found")
	}
	if s.userID != userID {
		return nil, apperrors.ErrUnauthorized("Session does not belong to current user")
	}
	s.mu.Lock()
	now := time.Now()
	expired := now.After(s.expiresAt) || now.After(s.hardExpiry)
	if !expired {
		s.expiresAt = now.Add(monicaSessionExpiry)
		if s.expiresAt.After(s.hardExpiry) {
			s.expiresAt = s.hardExpiry
		}
	}
	s.mu.Unlock()
	if expired {
		m.Delete(sessionID)
		return nil, apperrors.ErrNotFound("Import session expired")
	}
	return s, nil
}

// Delete removes a session and cancels any running fetch/import, dropping the
// API token from memory.
func (m *MonicaImportManager) Delete(sessionID string) {
	m.mu.Lock()
	s, exists := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if exists && s.cancel != nil {
		s.cancel()
	}
}

// Cancel is the /cancel endpoint's behaviour. During an in-flight import
// (phase importing/importing_photos) it cancels the transaction context — the
// whole import rolls back, phase becomes "cancelled" — and keeps the session
// so the user can retry from the review step. In any other phase it drops the
// session entirely (the "close the wizard" case). Ownership is enforced by
// get.
func (m *MonicaImportManager) Cancel(userID uint, sessionID string) *apperrors.AppError {
	s, appErr := m.get(sessionID, userID)
	if appErr != nil {
		return appErr
	}
	s.mu.Lock()
	inFlight := s.phase == models.MonicaPhaseImporting || s.phase == models.MonicaPhaseImportingPhotos
	cancel := s.cancel
	if inFlight {
		s.phase = models.MonicaPhaseCancelled
	}
	s.mu.Unlock()

	if inFlight {
		if cancel != nil {
			cancel()
		}
		return nil
	}
	m.Delete(sessionID)
	return nil
}

// userSafeMonicaError converts a client error into a message that never
// carries the token or internal detail.
func userSafeMonicaError(err error) string {
	switch {
	case errors.Is(err, monica.ErrUnauthorized):
		return "Monica rejected the API token"
	case errors.Is(err, monica.ErrInvalidURL):
		return "The Monica URL is invalid"
	case errors.Is(err, monica.ErrPrivateAddress):
		return "The Monica URL resolves to a private address, which this server blocks"
	case errors.Is(err, monica.ErrInvalidData):
		return "The server did not respond like a Monica API — check the URL"
	default:
		return "The Monica instance could not be reached"
	}
}

func monicaAppError(err error) *apperrors.AppError {
	switch {
	case errors.Is(err, monica.ErrUnauthorized):
		return apperrors.ErrInvalidInput("api_token", userSafeMonicaError(err))
	case errors.Is(err, monica.ErrInvalidURL), errors.Is(err, monica.ErrPrivateAddress), errors.Is(err, monica.ErrInvalidData):
		return apperrors.ErrInvalidInput("base_url", userSafeMonicaError(err))
	default:
		return apperrors.ErrExternal("monica", userSafeMonicaError(err)).WithError(err)
	}
}

// Connect validates the URL and token, counts the account's entities, and
// creates a session. Runs synchronously (~1 request/second for nine
// requests).
func (m *MonicaImportManager) Connect(ctx context.Context, userID uint, req models.MonicaConnectRequest, blockPrivate bool) (*models.MonicaConnectResponse, *apperrors.AppError) {
	client, err := monica.NewClient(req.BaseURL, req.APIToken, blockPrivate)
	if err != nil {
		return nil, monicaAppError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, monicaConnectTimeout)
	defer cancel()
	if err := client.TestConnection(ctx); err != nil {
		return nil, monicaAppError(err)
	}
	counts, err := client.CountEntities(ctx)
	if err != nil {
		return nil, monicaAppError(err)
	}
	if counts.Contacts > MaxMonicaContacts {
		return nil, apperrors.ErrInvalidInput("base_url",
			fmt.Sprintf("This Monica account has %d contacts; the import supports at most %d", counts.Contacts, MaxMonicaContacts))
	}
	if total := counts.Total(); total > MaxMonicaEntities {
		return nil, apperrors.ErrInvalidInput("base_url",
			fmt.Sprintf("This Monica account has %d records; the import supports at most %d", total, MaxMonicaEntities))
	}

	sessionID := generateSessionID()
	now := time.Now()
	session := &monicaImportSession{
		id:         sessionID,
		userID:     userID,
		client:     client,
		counts:     counts,
		phase:      models.MonicaPhaseConnecting,
		expiresAt:  now.Add(monicaSessionExpiry),
		hardExpiry: now.Add(monicaSessionMaxLifetime),
	}
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	return &models.MonicaConnectResponse{
		SessionID:             sessionID,
		Totals:                models.MonicaEntityCounts(counts),
		EstimatedFetchSeconds: estimateFetchSeconds(counts),
	}, nil
}

// estimateFetchSeconds approximates the fetch at ~1 request/second: one
// request per 100 records per entity, plus one per contact when relationships
// are included (reported as the worst case).
func estimateFetchSeconds(counts monica.EntityCounts) int {
	pages := 0
	for _, total := range []int{counts.Contacts, counts.Activities, counts.Notes, counts.Reminders, counts.Calls, counts.Tasks, counts.Gifts, counts.Debts} {
		pages += (total + 99) / 100
	}
	return pages + counts.Contacts
}

// StartFetch launches the background snapshot fetch. Conflicts if a fetch is
// already running or finished.
func (m *MonicaImportManager) StartFetch(db *gorm.DB, userID uint, req models.MonicaFetchRequest, log *zerolog.Logger) *apperrors.AppError {
	s, appErr := m.get(req.SessionID, userID)
	if appErr != nil {
		return appErr
	}

	s.mu.Lock()
	if s.phase != models.MonicaPhaseConnecting && s.phase != models.MonicaPhaseFailed {
		s.mu.Unlock()
		return apperrors.ErrConflict("A fetch is already running for this session")
	}
	s.phase = models.MonicaPhaseFetchingContacts
	s.phaseDone = 0
	s.phaseTotal = s.counts.Contacts
	s.errMsg = ""
	s.plan = nil
	s.previews = nil
	s.includeRels = req.IncludeRelationships
	s.includeExtras = req.IncludeExtras
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	go m.runFetch(ctx, db, s, log)
	return nil
}

// runFetch pulls the full account snapshot, maps it, and builds the preview.
func (m *MonicaImportManager) runFetch(ctx context.Context, db *gorm.DB, s *monicaImportSession, log *zerolog.Logger) {
	snapshot := &monica.Snapshot{Relationships: map[int][]monica.Relationship{}}
	fail := func(stage string, err error) {
		if ctx.Err() != nil {
			return // cancelled or expired; nobody is polling
		}
		log.Warn().Err(err).Str("stage", stage).Str("session_id", s.id).Msg("Monica fetch failed")
		s.fail(userSafeMonicaError(err))
	}

	s.setPhase(models.MonicaPhaseFetchingContacts, 0, s.counts.Contacts)
	allContacts, err := s.client.FetchAllContacts(ctx, s.setProgress)
	if err != nil {
		fail("contacts", err)
		return
	}
	for _, mc := range allContacts {
		// Partial contacts are name-only stubs Monica keeps behind
		// relationships; the mapper turns those into relationship names.
		if !mc.IsPartial {
			snapshot.Contacts = append(snapshot.Contacts, mc)
		}
	}

	s.setPhase(models.MonicaPhaseFetchingActivities, 0, s.counts.Activities)
	if snapshot.Activities, err = s.client.FetchAllActivities(ctx, s.setProgress); err != nil {
		fail("activities", err)
		return
	}

	s.setPhase(models.MonicaPhaseFetchingNotes, 0, s.counts.Notes)
	if snapshot.Notes, err = s.client.FetchAllNotes(ctx, s.setProgress); err != nil {
		fail("notes", err)
		return
	}

	s.setPhase(models.MonicaPhaseFetchingReminders, 0, s.counts.Reminders)
	if snapshot.Reminders, err = s.client.FetchAllReminders(ctx, s.setProgress); err != nil {
		fail("reminders", err)
		return
	}

	if s.includeExtras {
		extrasTotal := s.counts.Calls + s.counts.Tasks + s.counts.Gifts + s.counts.Debts
		s.setPhase(models.MonicaPhaseFetchingExtras, 0, extrasTotal)
		fetched := 0
		offset := func(done, total int) { s.setProgress(fetched+done, extrasTotal) }
		if snapshot.Calls, err = s.client.FetchAllCalls(ctx, offset); err != nil {
			fail("calls", err)
			return
		}
		fetched += len(snapshot.Calls)
		if snapshot.Tasks, err = s.client.FetchAllTasks(ctx, offset); err != nil {
			fail("tasks", err)
			return
		}
		fetched += len(snapshot.Tasks)
		if snapshot.Gifts, err = s.client.FetchAllGifts(ctx, offset); err != nil {
			fail("gifts", err)
			return
		}
		fetched += len(snapshot.Gifts)
		if snapshot.Debts, err = s.client.FetchAllDebts(ctx, offset); err != nil {
			fail("debts", err)
			return
		}
	}

	if s.includeRels {
		s.setPhase(models.MonicaPhaseFetchingRelationships, 0, len(snapshot.Contacts))
		for i, mc := range snapshot.Contacts {
			rels, relErr := s.client.FetchContactRelationships(ctx, mc.ID)
			if relErr != nil {
				fail("relationships", relErr)
				return
			}
			if len(rels) > 0 {
				snapshot.Relationships[mc.ID] = rels
			}
			s.setProgress(i+1, len(snapshot.Contacts))
		}
	}

	s.setPhase(models.MonicaPhaseBuildingPreview, 0, len(snapshot.Contacts))
	plan := MapMonicaSnapshot(snapshot, time.Now())
	previews := buildSourceImportPreview(db, s.userID, plan)

	s.mu.Lock()
	s.plan = plan
	s.previews = previews
	s.phase = models.MonicaPhaseReady
	s.phaseDone = len(previews)
	s.phaseTotal = len(previews)
	s.mu.Unlock()

	log.Info().
		Str("session_id", s.id).
		Int("contacts", len(plan.Contacts)).
		Int("relationships", len(plan.Relationships)).
		Int("notes", len(plan.Notes)).
		Int("activities", len(plan.Activities)).
		Int("issues", len(plan.Report.Issues)).
		Msg("Monica snapshot fetched and mapped")
}

// Status returns the current phase and progress for polling.
func (m *MonicaImportManager) Status(userID uint, sessionID string) (*models.MonicaImportStatus, *apperrors.AppError) {
	s, appErr := m.get(sessionID, userID)
	if appErr != nil {
		return nil, appErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status := &models.MonicaImportStatus{
		SessionID:  sessionID,
		Phase:      s.phase,
		PhaseDone:  s.phaseDone,
		PhaseTotal: s.phaseTotal,
		Error:      s.errMsg,
	}
	if s.result != nil {
		resultCopy := *s.result
		status.Result = &resultCopy
	}
	return status, nil
}

// Preview returns the full review payload once the snapshot is mapped.
func (m *MonicaImportManager) Preview(userID uint, sessionID string) (*models.MonicaPreviewResponse, *apperrors.AppError) {
	s, appErr := m.get(sessionID, userID)
	if appErr != nil {
		return nil, appErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != models.MonicaPhaseReady || s.plan == nil {
		return nil, apperrors.ErrInvalidInput("session", "The Monica data is not fetched yet")
	}

	validRows, dupCount, errCount, totals := previewTotals(s.previews, len(s.plan.Relationships))
	return &models.MonicaPreviewResponse{
		SessionID:      sessionID,
		Rows:           s.previews,
		TotalRows:      len(s.previews),
		ValidRows:      validRows,
		DuplicateCount: dupCount,
		ErrorCount:     errCount,
		Totals:         totals,
		LossReport:     mapSourceImportIssues(s.plan.Report.Issues),
	}, nil
}

// avatarTask defers one contact-photo download until after the import commits.
type avatarTask struct {
	contactID uint
	avatarURL string
}

// Confirm starts the import in the background and returns immediately (the
// controller replies 202). The transaction is bound to a session-scoped
// context so an in-flight "Cancel import" (see Cancel) rolls it back whole.
// The caller polls Status: phase advances importing → importing_photos → done
// (or → cancelled / failed), and Status.Result is set from importing_photos on.
func (m *MonicaImportManager) Confirm(db *gorm.DB, userID uint, req models.MonicaConfirmRequest, cfg *config.Config, log *zerolog.Logger) *apperrors.AppError {
	s, appErr := m.get(req.SessionID, userID)
	if appErr != nil {
		return appErr
	}

	s.mu.Lock()
	if s.phase != models.MonicaPhaseReady || s.plan == nil {
		s.mu.Unlock()
		return apperrors.ErrInvalidInput("session", "The Monica data is not fetched yet")
	}
	plan := s.plan
	previews := s.previews
	s.mu.Unlock()

	actions, appErr := resolveSourceContactActions(db, userID, plan, previews, req.Actions)
	if appErr != nil {
		return appErr
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.phase = models.MonicaPhaseImporting
	s.phaseDone = 0
	s.phaseTotal = len(previews) + importGraphKinds
	s.errMsg = ""
	s.cancel = cancel
	s.mu.Unlock()

	go m.runImport(ctx, db, s, plan, previews, actions, cfg, log)
	return nil
}

// runImport applies the plan in one cancellable transaction, then queues
// avatar downloads.
func (m *MonicaImportManager) runImport(ctx context.Context, db *gorm.DB, s *monicaImportSession, plan *ImportSourcePlan, previews []models.MonicaImportRowPreview, actions map[string]SourceContactAction, cfg *config.Config, log *zerolog.Logger) {
	report, refToID, err := ExecuteSourceImportWithActions(ctx, db, s.userID, plan, actions, s.setProgress)
	if err != nil {
		if ctx.Err() != nil {
			s.mu.Lock()
			s.phase = models.MonicaPhaseCancelled
			s.mu.Unlock()
			log.Info().Str("session_id", s.id).Msg("Monica import cancelled")
			return
		}
		log.Error().Err(err).Str("session_id", s.id).Msg("Monica import failed")
		s.fail("The import could not be applied")
		return
	}

	result := sourceImportResultFromReport(report, len(previews))

	var tasks []avatarTask
	for i := range plan.Contacts {
		mc := &plan.Contacts[i]
		if mc.PhotoURL == "" {
			continue
		}
		if id, ok := refToID[mc.Ref.ExternalID]; ok {
			tasks = append(tasks, avatarTask{contactID: id, avatarURL: mc.PhotoURL})
		}
	}
	result.PhotosQueued = len(tasks)

	models.RecordImportRun(context.Background(), db, models.ImportRun{
		UserID:         s.userID,
		Format:         models.ImportFormatMonica,
		TotalProcessed: result.TotalProcessed,
		Created:        result.Created,
		Updated:        result.Updated,
		Skipped:        result.Skipped,
		ErrorCount:     len(result.Errors),
	})

	s.mu.Lock()
	resultCopy := result
	s.result = &resultCopy
	s.plan = nil // import committed; free the snapshot
	s.previews = nil
	if len(tasks) == 0 {
		s.phase = models.MonicaPhaseDone
	} else {
		s.phase = models.MonicaPhaseImportingPhotos
		s.phaseDone = 0
		s.phaseTotal = len(tasks)
	}
	s.mu.Unlock()

	log.Info().
		Str("session_id", s.id).
		Int("created", result.Created).
		Int("updated", result.Updated).
		Int("skipped", result.Skipped).
		Int("relationships", result.RelationshipsCreated).
		Int("notes", result.NotesCreated).
		Int("activities", result.ActivitiesCreated).
		Int("photos_queued", result.PhotosQueued).
		Int("errors", len(result.Errors)).
		Msg("Monica import completed")

	if len(tasks) > 0 {
		// #nosec G118 -- avatar downloads deliberately outlive runImport's ctx: the
		// import has already committed, and the "cancel import" context must not
		// abort the photo tail. processAvatars runs on its own timeout context.
		go m.processAvatars(db, s, tasks, cfg, log)
	}
}

// processAvatars downloads and stores contact photos after the transaction,
// reporting progress through the session for status polling. A per-photo
// failure is logged and counted, never fatal.
func (m *MonicaImportManager) processAvatars(db *gorm.DB, s *monicaImportSession, tasks []avatarTask, cfg *config.Config, log *zerolog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(tasks)+60)*2*time.Second)
	defer cancel()

	for i, task := range tasks {
		saved := false
		photoData, mediaType, err := s.client.FetchAvatar(ctx, task.avatarURL)
		if err != nil {
			log.Warn().Err(err).Uint("contact_id", task.contactID).Msg("Failed to fetch Monica avatar")
		} else if photoPath, thumb, saveErr := photostore.SaveContactPhoto(photoData, mediaType, cfg.ProfilePhotoDir); saveErr != nil {
			log.Warn().Err(saveErr).Uint("contact_id", task.contactID).Msg("Failed to save Monica avatar")
		} else if photoPath != "" {
			// Scoped column update only — never a bare db.Save of a loaded
			// contact (CLAUDE.md backend trap #3).
			if updErr := db.Model(&models.Contact{}).Where("id = ?", task.contactID).
				Updates(map[string]any{"photo": photoPath, "photo_thumbnail": thumb}).Error; updErr != nil {
				log.Warn().Err(updErr).Uint("contact_id", task.contactID).Msg("Failed to attach Monica avatar")
			} else {
				saved = true
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
	s.phase = models.MonicaPhaseDone
	s.mu.Unlock()
}
