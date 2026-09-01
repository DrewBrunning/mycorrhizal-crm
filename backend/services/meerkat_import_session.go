package services

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/meerkat"
	"mycorrhizal/models"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// This file is the Meerkat import assistant's server-side orchestration
// (issue #550): the user uploads their Meerkat CRM SQLite file, picks which
// source user to import, and the mapped plan is reviewed (with the loss
// report) and confirmed through the shared source-import engine. Unlike
// Monica there is no network and no credential — the uploaded database is
// held only as a 0600 temp file for the session's lifetime and deleted on
// cancel/expiry/restart (see docs/security/data-retention-lifecycle.md).
//
// The uploaded file is hostile input: it is a bare SQLite file from outside.
// meerkat.Open validates the SQLite magic header, opens read-only
// (mode=ro), never writes/migrates/executes anything, and tolerates schema
// drift. Size and row caps below bound the memory a single upload can claim.

const (
	meerkatSessionExpiry      = 60 * time.Minute
	meerkatSessionMaxLifetime = 6 * time.Hour
	// MaxMeerkatDBSize caps the uploaded file. A real Meerkat deployment's
	// database is small; 100 MiB is generous and keeps the parse bounded.
	MaxMeerkatDBSize = 100 << 20
	// MaxMeerkatContacts / MaxMeerkatEntities bound what one import can hold
	// in memory for the wizard's lifetime (issue #415).
	MaxMeerkatContacts = 20000
	MaxMeerkatEntities = 200000
	// MaxMeerkatImportSessionsPerUser bounds concurrent Meerkat wizards per
	// user, matching the Monica cap.
	MaxMeerkatImportSessionsPerUser = 3
)

var meerkatDBExtensions = map[string]bool{".db": true, ".sqlite": true, ".sqlite3": true}

// ErrMeerkatFile is the log-safe sentinel for a rejected upload (never
// carries the temp path).
var ErrMeerkatFile = errors.New("the uploaded file is not a readable Meerkat database")

type meerkatImportSession struct {
	id           string
	userID       uint
	snapshot     *meerkat.Snapshot
	tempDir      string
	sourceUserID *int64
	cancel       context.CancelFunc

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

func (s *meerkatImportSession) setPhase(phase string, done, total int) {
	s.mu.Lock()
	s.phase, s.phaseDone, s.phaseTotal = phase, done, total
	s.mu.Unlock()
}

func (s *meerkatImportSession) setProgress(done, total int) {
	s.mu.Lock()
	s.phaseDone, s.phaseTotal = done, total
	s.mu.Unlock()
}

func (s *meerkatImportSession) fail(msg string) {
	s.mu.Lock()
	s.phase = models.SourceImportPhaseFailed
	s.errMsg = msg
	s.mu.Unlock()
}

// MeerkatImportManager owns the lifecycle of Meerkat import sessions,
// including their uploaded-file temp directories.
type MeerkatImportManager struct {
	mu       sync.RWMutex
	sessions map[string]*meerkatImportSession
}

// NewMeerkatImportManager creates an empty manager.
func NewMeerkatImportManager() *MeerkatImportManager {
	return &MeerkatImportManager{sessions: make(map[string]*meerkatImportSession)}
}

// CleanupExpired removes expired sessions, cancels any running work, and
// deletes their temp directories. Safe to call from a goroutine.
func (m *MeerkatImportManager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, s := range m.sessions {
		s.mu.Lock()
		expired := now.After(s.expiresAt) || now.After(s.hardExpiry)
		s.mu.Unlock()
		if expired {
			m.teardown(s)
			delete(m.sessions, id)
		}
	}
}

// teardown cancels a session's work and removes its temp directory.
func (m *MeerkatImportManager) teardown(s *meerkatImportSession) {
	if s.cancel != nil {
		s.cancel()
	}
	if s.tempDir != "" {
		_ = os.RemoveAll(s.tempDir)
	}
}

// CountActive reports how many live sessions a user holds (issue #415).
func (m *MeerkatImportManager) CountActive(userID uint) int {
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

func (m *MeerkatImportManager) get(sessionID string, userID uint) (*meerkatImportSession, *apperrors.AppError) {
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
		s.expiresAt = now.Add(meerkatSessionExpiry)
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

// Delete removes a session, cancelling any running work and deleting its
// uploaded-file temp directory.
func (m *MeerkatImportManager) Delete(sessionID string) {
	m.mu.Lock()
	s, exists := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if exists {
		m.teardown(s)
	}
}

// Cancel is the /cancel endpoint's behaviour: an in-flight import
// (importing) is rolled back (phase "cancelled") and the session kept for a
// retry; any other phase drops the session and its temp file.
func (m *MeerkatImportManager) Cancel(userID uint, sessionID string) *apperrors.AppError {
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

// Upload validates and stores the uploaded Meerkat database and opens a
// session. It parses the file synchronously (a local SQLite read is fast) so
// the response can carry the per-source-user picker and the whole-file
// totals.
func (m *MeerkatImportManager) Upload(userID uint, header *multipart.FileHeader) (*models.MeerkatUploadResponse, *apperrors.AppError) {
	if header.Size <= 0 || header.Size > MaxMeerkatDBSize {
		return nil, apperrors.ErrInvalidInput("file", "The database file is empty or larger than the 100 MB limit")
	}
	if !meerkatDBExtensions[strings.ToLower(filepath.Ext(header.Filename))] {
		return nil, apperrors.ErrInvalidInput("file", "Upload a Meerkat SQLite database (.db, .sqlite or .sqlite3)")
	}

	src, err := header.Open()
	if err != nil {
		return nil, apperrors.ErrInvalidInput("file", "The upload could not be read")
	}
	defer src.Close()

	tempDir, err := os.MkdirTemp("", "mycorrhizal-meerkat-*")
	if err != nil { // # pragma: no cover — defensive: the OS temp dir is writable in every supported deployment
		return nil, apperrors.ErrInternal("Could not stage the upload")
	}
	dbPath := filepath.Join(tempDir, "meerkat.sqlite")
	// #nosec G304 -- dbPath is a server-generated name under a freshly created os.MkdirTemp dir, never request input
	dst, err := os.OpenFile(dbPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil { // # pragma: no cover — defensive: the temp dir was just created
		_ = os.RemoveAll(tempDir)
		return nil, apperrors.ErrInternal("Could not stage the upload")
	}
	if _, err := io.CopyN(dst, src, MaxMeerkatDBSize+1); err != nil && !errors.Is(err, io.EOF) {
		_ = dst.Close()
		_ = os.RemoveAll(tempDir)
		return nil, apperrors.ErrInvalidInput("file", "The upload could not be read")
	}
	_ = dst.Close()

	snap, err := meerkat.Open(dbPath)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		if errors.Is(err, meerkat.ErrNotSQLite) {
			return nil, apperrors.ErrInvalidInput("file", "That file is not a Meerkat database")
		}
		return nil, apperrors.ErrInvalidInput("file", "The database could not be read as a Meerkat database")
	}
	if len(snap.Contacts) > MaxMeerkatContacts {
		_ = os.RemoveAll(tempDir)
		return nil, apperrors.ErrInvalidInput("file",
			"This database has more contacts than the import supports")
	}
	if meerkatEntityTotal(snap) > MaxMeerkatEntities {
		_ = os.RemoveAll(tempDir)
		return nil, apperrors.ErrInvalidInput("file",
			"This database has more records than the import supports")
	}

	sessionID := generateSessionID()
	now := time.Now()
	session := &meerkatImportSession{
		id:           sessionID,
		userID:       userID,
		snapshot:     snap,
		tempDir:      tempDir,
		sourceUserID: snap.SourceUserID,
		phase:        models.SourceImportPhaseConnecting,
		expiresAt:    now.Add(meerkatSessionExpiry),
		hardExpiry:   now.Add(meerkatSessionMaxLifetime),
	}
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	return &models.MeerkatUploadResponse{
		SessionID:           sessionID,
		SourceUsers:         meerkatSourceUsers(snap),
		DefaultSourceUserID: snap.SourceUserID,
		Totals: models.MeerkatEntityCounts{
			Contacts:      len(snap.Contacts),
			Relationships: len(snap.Relationships),
			Notes:         len(snap.Notes),
			Activities:    len(snap.Activities),
			Reminders:     len(snap.Reminders),
		},
	}, nil
}

func meerkatEntityTotal(snap *meerkat.Snapshot) int {
	return len(snap.Contacts) + len(snap.Relationships) + len(snap.Notes) +
		len(snap.Activities) + len(snap.Reminders)
}

// meerkatSourceUsers builds the picker list: every user_id seen on a contact,
// with a display name from the users table when present and a contact count.
func meerkatSourceUsers(snap *meerkat.Snapshot) []models.MeerkatSourceUser {
	counts := map[int64]int{}
	var order []int64
	for _, c := range snap.Contacts {
		if c.UserID == nil {
			continue
		}
		if _, seen := counts[*c.UserID]; !seen {
			order = append(order, *c.UserID)
		}
		counts[*c.UserID]++
	}
	byID := map[int64]meerkat.User{}
	for _, u := range snap.Users {
		byID[u.ID] = u
	}
	out := make([]models.MeerkatSourceUser, 0, len(order))
	for _, id := range order {
		mu := models.MeerkatSourceUser{ID: id, Contacts: counts[id]}
		if u, ok := byID[id]; ok {
			mu.Username = deref(u.Username)
			mu.Email = deref(u.Email)
			mu.Name = deref(u.Name)
		}
		out = append(out, mu)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// StartFetch launches the background map + preview build for a session,
// scoped to req.SourceUserID (default: the first source user).
func (m *MeerkatImportManager) StartFetch(db *gorm.DB, userID uint, req models.MeerkatFetchRequest, log *zerolog.Logger) *apperrors.AppError {
	s, appErr := m.get(req.SessionID, userID)
	if appErr != nil {
		return appErr
	}
	s.mu.Lock()
	if s.phase != models.SourceImportPhaseConnecting && s.phase != models.SourceImportPhaseFailed {
		s.mu.Unlock()
		return apperrors.ErrConflict("This import is already being prepared")
	}
	if req.SourceUserID != nil {
		s.sourceUserID = req.SourceUserID
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

func (m *MeerkatImportManager) runFetch(ctx context.Context, db *gorm.DB, s *meerkatImportSession, log *zerolog.Logger) {
	if ctx.Err() != nil {
		return
	}
	s.setPhase(models.SourceImportPhaseMapping, 0, 0)
	plan := MapMeerkatSnapshot(s.snapshot, s.sourceUserID)

	if ctx.Err() != nil {
		return
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
		Msg("Meerkat snapshot mapped")
}

// Status returns the current phase and progress for polling.
func (m *MeerkatImportManager) Status(userID uint, sessionID string) (*models.SourceImportStatus, *apperrors.AppError) {
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

// Preview returns the full review payload once the snapshot is mapped.
func (m *MeerkatImportManager) Preview(userID uint, sessionID string) (*models.SourceImportPreviewResponse, *apperrors.AppError) {
	s, appErr := m.get(sessionID, userID)
	if appErr != nil {
		return nil, appErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != models.SourceImportPhaseReady || s.plan == nil {
		return nil, apperrors.ErrInvalidInput("session", "The Meerkat database is not prepared yet")
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
func (m *MeerkatImportManager) Confirm(db *gorm.DB, userID uint, req models.SourceImportConfirmRequest, log *zerolog.Logger) *apperrors.AppError {
	s, appErr := m.get(req.SessionID, userID)
	if appErr != nil {
		return appErr
	}
	s.mu.Lock()
	if s.phase != models.SourceImportPhaseReady || s.plan == nil {
		s.mu.Unlock()
		return apperrors.ErrInvalidInput("session", "The Meerkat database is not prepared yet")
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

func (m *MeerkatImportManager) runImport(ctx context.Context, db *gorm.DB, s *meerkatImportSession, plan *ImportSourcePlan, totalRows int, actions map[string]SourceContactAction, log *zerolog.Logger) {
	report, _, err := ExecuteSourceImportWithActions(ctx, db, s.userID, plan, actions, s.setProgress)
	if err != nil {
		if ctx.Err() != nil {
			s.mu.Lock()
			s.phase = models.SourceImportPhaseCancelled
			s.mu.Unlock()
			log.Info().Str("session_id", s.id).Msg("Meerkat import cancelled")
			return
		}
		log.Error().Err(err).Str("session_id", s.id).Msg("Meerkat import failed")
		s.fail("The import could not be applied")
		return
	}

	result := sourceImportResultFromReport(report, totalRows)
	models.RecordImportRun(context.Background(), db, models.ImportRun{
		UserID:         s.userID,
		Format:         models.ImportFormatMeerkat,
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
		Msg("Meerkat import completed")
}
