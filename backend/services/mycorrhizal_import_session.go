package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// This file is the `mycorrhizal` account-bundle import source's server-side
// orchestration (issue #1260, ADR 0028 Decision 3): the user uploads a bundle
// produced by GET /export/account, reviews the mapped rows (with the loss
// report) and confirms through the shared source-import engine. It mirrors the
// Meerkat assistant (meerkat_import_session.go) but holds the parsed bundle in
// memory rather than staging a file — the upload is a bounded JSON document.
//
// The uploaded bundle is hostile input: it is parsed as JSON and its declared
// format/version are checked before anything is mapped; size and entity caps
// bound the memory one upload can claim.

const (
	mycorrhizalSessionExpiry      = 60 * time.Minute
	mycorrhizalSessionMaxLifetime = 6 * time.Hour
	// MaxMycorrhizalBundleSize caps the uploaded bundle.
	// The account-bundle export (GET /export/account) refuses to produce a
	// bundle larger than this, so an exported bundle is always importable
	// (issue #1313).
	MaxMycorrhizalBundleSize = 64 << 20

	// MycorrhizalUploadOverhead is the multipart framing slack added on top of
	// MaxMycorrhizalBundleSize for the route's request-body cap, so a bundle at
	// exactly the file limit is not rejected by the envelope around it.
	MycorrhizalUploadOverhead = 1 << 20

	// MycorrhizalUploadBodyLimit is the route-level request-body cap.
	MycorrhizalUploadBodyLimit = MaxMycorrhizalBundleSize + MycorrhizalUploadOverhead
	// MaxMycorrhizalContacts / MaxMycorrhizalEntities bound what one import
	// can hold in memory for the wizard's lifetime (issue #415).
	MaxMycorrhizalContacts = 20000
	MaxMycorrhizalEntities = 500000
	// MaxMycorrhizalImportSessionsPerUser bounds concurrent bundle wizards per
	// user, matching the Monica/Meerkat cap.
	MaxMycorrhizalImportSessionsPerUser = 3
)

// ErrMycorrhizalBundle is the log-safe sentinel for a rejected upload (never
// carries file content).
var ErrMycorrhizalBundle = errors.New("the uploaded file is not a readable Mycorrhizal account bundle")

type mycorrhizalImportSession struct {
	id     string
	userID uint
	bundle *models.AccountBundle
	cancel context.CancelFunc

	mu         sync.Mutex
	phase      string
	phaseDone  int
	phaseTotal int
	errMsg     string
	plan       *ImportSourcePlan
	previews   []models.SourceImportRowPreview
	result     *models.SourceImportResult
	expiresAt  time.Time
	hardExpiry time.Time
}

func (s *mycorrhizalImportSession) setPhase(phase string, done, total int) {
	s.mu.Lock()
	s.phase, s.phaseDone, s.phaseTotal = phase, done, total
	s.mu.Unlock()
}

func (s *mycorrhizalImportSession) setProgress(done, total int) {
	s.mu.Lock()
	s.phaseDone, s.phaseTotal = done, total
	s.mu.Unlock()
}

func (s *mycorrhizalImportSession) fail(msg string) {
	s.mu.Lock()
	s.phase = models.SourceImportPhaseFailed
	s.errMsg = msg
	s.mu.Unlock()
}

// MycorrhizalImportManager owns the lifecycle of account-bundle import
// sessions.
type MycorrhizalImportManager struct {
	mu       sync.RWMutex
	sessions map[string]*mycorrhizalImportSession
}

// NewMycorrhizalImportManager creates an empty manager.
func NewMycorrhizalImportManager() *MycorrhizalImportManager {
	return &MycorrhizalImportManager{sessions: make(map[string]*mycorrhizalImportSession)}
}

// CleanupExpired removes expired sessions and cancels any running work.
func (m *MycorrhizalImportManager) CleanupExpired() {
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
func (m *MycorrhizalImportManager) CountActive(userID uint) int {
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

func (m *MycorrhizalImportManager) get(sessionID string, userID uint) (*mycorrhizalImportSession, *apperrors.AppError) {
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
		s.expiresAt = now.Add(mycorrhizalSessionExpiry)
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

// Delete removes a session, cancelling any running work.
func (m *MycorrhizalImportManager) Delete(sessionID string) {
	m.mu.Lock()
	s, exists := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if exists && s.cancel != nil {
		s.cancel()
	}
}

// Cancel is the /cancel endpoint's behaviour: an in-flight import (importing)
// is rolled back (phase "cancelled") and the session kept for a retry; any
// other phase drops the session.
func (m *MycorrhizalImportManager) Cancel(userID uint, sessionID string) *apperrors.AppError {
	s, appErr := m.get(sessionID, userID)
	if appErr != nil {
		return appErr
	}
	s.mu.Lock()
	inFlight := s.phase == models.SourceImportPhaseImporting
	cancel := s.cancel
	if inFlight {
		s.phase = models.SourceImportPhaseCancelled
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

// Upload validates and stores an uploaded account bundle and opens a session.
// It parses synchronously (a JSON document is fast) so the response can carry
// the per-section totals.
func (m *MycorrhizalImportManager) Upload(userID uint, header *multipart.FileHeader) (*models.MycorrhizalUploadResponse, *apperrors.AppError) {
	if header.Size <= 0 || header.Size > MaxMycorrhizalBundleSize {
		return nil, apperrors.ErrInvalidInput("file", "The bundle is empty or larger than the 64 MiB limit")
	}
	src, err := header.Open()
	if err != nil { // # pragma: no cover — defensive: header.Open on a staged multipart file
		return nil, apperrors.ErrInvalidInput("file", "The upload could not be read") // # pragma: no cover — defensive: header.Open on a staged multipart file
	}
	defer src.Close()

	data, err := io.ReadAll(io.LimitReader(src, MaxMycorrhizalBundleSize+1))
	if err != nil || int64(len(data)) > MaxMycorrhizalBundleSize { // # pragma: no cover — defensive: reading a bounded in-memory upload
		return nil, apperrors.ErrInvalidInput("file", "The upload could not be read") // # pragma: no cover — defensive: reading a bounded in-memory upload
	}

	var bundle models.AccountBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, apperrors.ErrInvalidInput("file", "That file is not a Mycorrhizal account bundle")
	}
	if bundle.Format != models.AccountBundleFormat {
		return nil, apperrors.NewError(apperrors.ErrCodeInvalidInput,
			"That file is not a Mycorrhizal account bundle", http.StatusUnprocessableEntity)
	}
	if bundle.Version != models.AccountBundleVersion {
		return nil, apperrors.NewError(apperrors.ErrCodeInvalidInput,
			"Unsupported account bundle version", http.StatusUnprocessableEntity)
	}
	if len(bundle.Plan.Contacts) > MaxMycorrhizalContacts { // # pragma: no cover — requires a >20k-contact document; the cap is enforced by the constant and the route's body limit bounds memory first
		return nil, apperrors.ErrInvalidInput("file", "This bundle has more contacts than the import supports") // # pragma: no cover — requires a >20k-contact document
	}
	if mycorrhizalEntityTotal(&bundle) > MaxMycorrhizalEntities { // # pragma: no cover — requires a >500k-entity document; see the contact cap
		return nil, apperrors.ErrInvalidInput("file", "This bundle has more records than the import supports") // # pragma: no cover — requires a >500k-entity document
	}

	sessionID := generateSessionID()
	now := time.Now()
	session := &mycorrhizalImportSession{
		id:         sessionID,
		userID:     userID,
		bundle:     &bundle,
		phase:      models.SourceImportPhaseConnecting,
		expiresAt:  now.Add(mycorrhizalSessionExpiry),
		hardExpiry: now.Add(mycorrhizalSessionMaxLifetime),
	}
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	return &models.MycorrhizalUploadResponse{
		SessionID: sessionID,
		Version:   bundle.Version,
		Totals:    mycorrhizalBundleCounts(&bundle),
	}, nil
}

func mycorrhizalBundleCounts(b *models.AccountBundle) models.MycorrhizalBundleCounts {
	return models.MycorrhizalBundleCounts{
		Contacts:               len(b.Plan.Contacts),
		Relationships:          len(b.Plan.Relationships),
		Notes:                  len(b.Plan.Notes),
		Reminders:              len(b.Plan.Reminders),
		ReminderCompletions:    len(b.Plan.ReminderCompletions),
		Activities:             len(b.Plan.Activities),
		LifeEvents:             len(b.Plan.LifeEvents),
		Gifts:                  len(b.Plan.Gifts),
		Preferences:            len(b.Plan.Preferences),
		ConversationAgenda:     len(b.Plan.ConversationAgenda),
		CadencePolicies:        len(b.Plan.CadencePolicies),
		DataDecayPolicies:      len(b.Plan.DataDecayPolicies),
		Households:             len(b.Plan.Households),
		Circles:                len(b.Plan.Circles),
		Tags:                   len(b.Plan.Tags),
		CustomFieldDefinitions: len(b.Plan.CustomFieldDefinitions),
		CustomFieldValues:      len(b.Plan.CustomFieldValues),
		Occasions:              len(b.Plan.Occasions),
		OccasionEvents:         len(b.Plan.OccasionEvents),
	}
}

func mycorrhizalEntityTotal(b *models.AccountBundle) int {
	c := mycorrhizalBundleCounts(b)
	return c.Contacts + c.Relationships + c.Notes + c.Reminders + c.ReminderCompletions +
		c.Activities + c.LifeEvents + c.Gifts + c.Preferences + c.ConversationAgenda +
		c.CadencePolicies + c.DataDecayPolicies + c.Households + c.Circles + c.Tags +
		c.CustomFieldDefinitions + c.CustomFieldValues + c.Occasions + c.OccasionEvents
}

// StartFetch launches the background map + preview build for a session.
func (m *MycorrhizalImportManager) StartFetch(db *gorm.DB, userID uint, req models.MycorrhizalFetchRequest, log *zerolog.Logger) *apperrors.AppError {
	s, appErr := m.get(req.SessionID, userID)
	if appErr != nil {
		return appErr
	}
	s.mu.Lock()
	if s.phase != models.SourceImportPhaseConnecting && s.phase != models.SourceImportPhaseFailed {
		s.mu.Unlock()
		return apperrors.ErrConflict("This import is already being prepared")
	}
	s.phase = models.SourceImportPhaseMapping
	s.phaseDone, s.phaseTotal, s.errMsg = 0, 0, ""
	s.plan, s.previews = nil, nil
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	go m.runFetch(ctx, db, s, log)
	return nil
}

func (m *MycorrhizalImportManager) runFetch(ctx context.Context, db *gorm.DB, s *mycorrhizalImportSession, log *zerolog.Logger) {
	if ctx.Err() != nil { // # pragma: no cover — cancel-during-fetch guard; the in-memory mapping completes before a cancel can race it
		return // # pragma: no cover — cancel-during-fetch guard
	}
	s.setPhase(models.SourceImportPhaseMapping, 0, 0)
	plan := MapAccountBundle(s.bundle)

	if ctx.Err() != nil { // # pragma: no cover — cancel-during-fetch guard
		return // # pragma: no cover — cancel-during-fetch guard
	}
	s.setPhase(models.SourceImportPhaseBuildingPreview, 0, len(plan.Contacts))
	previews := buildSourceImportPreview(db, s.userID, plan)

	s.mu.Lock()
	s.plan = plan
	s.previews = previews
	s.phase = models.SourceImportPhaseReady
	s.phaseDone, s.phaseTotal = len(previews), len(previews)
	s.mu.Unlock()

	log.Info().
		Str("session_id", s.id).
		Int("contacts", len(plan.Contacts)).
		Int("relationships", len(plan.Relationships)).
		Int("issues", len(plan.Report.Issues)).
		Msg("Account bundle mapped")
}

// Status returns the current phase and progress for polling.
func (m *MycorrhizalImportManager) Status(userID uint, sessionID string) (*models.SourceImportStatus, *apperrors.AppError) {
	s, appErr := m.get(sessionID, userID)
	if appErr != nil {
		return nil, appErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status := &models.SourceImportStatus{
		SessionID:  sessionID,
		Phase:      s.phase,
		PhaseDone:  s.phaseDone,
		PhaseTotal: s.phaseTotal,
		Error:      s.errMsg,
	}
	if s.result != nil {
		rc := *s.result
		status.Result = &rc
	}
	return status, nil
}

// Preview returns the full review payload once the bundle is mapped.
func (m *MycorrhizalImportManager) Preview(userID uint, sessionID string) (*models.SourceImportPreviewResponse, *apperrors.AppError) {
	s, appErr := m.get(sessionID, userID)
	if appErr != nil {
		return nil, appErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != models.SourceImportPhaseReady || s.plan == nil {
		return nil, apperrors.ErrInvalidInput("session", "The account bundle is not prepared yet")
	}
	validRows, dupCount, errCount, totals := previewTotals(s.previews, len(s.plan.Relationships))
	return &models.SourceImportPreviewResponse{
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

// Confirm starts the import in the background (the endpoint returns 202); the
// client polls Status until phase done / cancelled / failed.
func (m *MycorrhizalImportManager) Confirm(db *gorm.DB, userID uint, req models.SourceImportConfirmRequest, log *zerolog.Logger) *apperrors.AppError {
	s, appErr := m.get(req.SessionID, userID)
	if appErr != nil {
		return appErr
	}
	s.mu.Lock()
	if s.phase != models.SourceImportPhaseReady || s.plan == nil {
		s.mu.Unlock()
		return apperrors.ErrInvalidInput("session", "The account bundle is not prepared yet")
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
	s.phase = models.SourceImportPhaseImporting
	s.phaseDone, s.phaseTotal = 0, len(previews)+importGraphKinds
	s.errMsg = ""
	s.cancel = cancel
	s.mu.Unlock()

	go m.runImport(ctx, db, s, plan, len(previews), actions, log)
	return nil
}

func (m *MycorrhizalImportManager) runImport(ctx context.Context, db *gorm.DB, s *mycorrhizalImportSession, plan *ImportSourcePlan, totalRows int, actions map[string]SourceContactAction, log *zerolog.Logger) {
	report, _, err := ExecuteSourceImportWithActions(ctx, db, s.userID, plan, actions, s.setProgress)
	if err != nil {
		if ctx.Err() != nil {
			s.mu.Lock()
			s.phase = models.SourceImportPhaseCancelled
			s.mu.Unlock()
			log.Info().Str("session_id", s.id).Msg("Account bundle import cancelled")
			return
		}
		log.Error().Err(err).Str("session_id", s.id).Msg("Account bundle import failed")
		s.fail("The import could not be applied")
		return
	}

	result := sourceImportResultFromReport(report, totalRows)
	models.RecordImportRun(context.Background(), db, models.ImportRun{
		UserID:         s.userID,
		Format:         models.ImportFormatMycorrhizal,
		TotalProcessed: result.TotalProcessed,
		Created:        result.Created,
		Updated:        result.Updated,
		Skipped:        result.Skipped,
		ErrorCount:     len(result.Errors),
	})

	s.mu.Lock()
	rc := result
	s.result = &rc
	s.plan, s.previews = nil, nil
	s.phase = models.SourceImportPhaseDone
	s.mu.Unlock()

	log.Info().
		Str("session_id", s.id).
		Int("created", result.Created).
		Int("updated", result.Updated).
		Int("skipped", result.Skipped).
		Int("relationships", result.RelationshipsCreated).
		Int("errors", len(result.Errors)).
		Msg("Account bundle import completed")
}
