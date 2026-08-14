package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/photostore"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// sessionExpiry is how long an in-progress import wizard session is kept server-side.
const sessionExpiry = 15 * time.Minute

// importSessionData holds the server-side state for an in-progress import wizard.
// Sessions are kept in memory only (not persisted) and are lost on restart.
type importSessionData struct {
	session     models.ImportSession
	rows        [][]string       // CSV rows (nil for VCF imports)
	importType  string           // "csv" or "vcf"
	vcfContacts []VCFContactData // VCF parsed contacts (nil for CSV imports)
	csvContacts []models.Contact // CSV contacts built during preview (nil for VCF imports)

	// boundShareID, when non-empty, ties this session to the ContactShare
	// (P1, docs/fork-plan/tickets/31-P1-contact-sharing.md) whose accept step
	// created it — see CreateVCFSessionForShare/SessionBelongsToShare below.
	// Empty for ordinary CSV/VCF/JSContact import sessions.
	boundShareID string

	// confirmMu serializes concurrent confirms of this same session so a race
	// cannot apply an import twice (T57 idempotent confirm). confirmed, when
	// non-nil, means a confirm has already committed and its result is
	// authoritative — later confirms replay it instead of re-applying.
	confirmMu sync.Mutex
	confirmed *models.ImportResult
}

// confirmedImport is the tiny post-confirm tombstone kept so a client that
// lost a confirm's response can retry the same session_id idempotently (T57)
// instead of 404ing and re-uploading (which would double-import). The full
// session payload is deleted on confirm; only the result, its owner, and an
// expiry are retained until the normal session window closes.
type confirmedImport struct {
	userID    uint
	result    models.ImportResult
	expiresAt time.Time
}

// ImportSessionManager owns the lifecycle of in-progress import sessions: creation,
// retrieval/validation, preview generation, confirmation, and expiry. It is the single
// owner of import state so controllers only need to validate input and delegate.
type ImportSessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*importSessionData

	// confirmedResults maps a consumed session_id to its import result for
	// the remainder of the session's 15-minute window (T57 idempotency).
	confirmedResults map[string]confirmedImport
}

// NewImportSessionManager creates an empty session manager.
func NewImportSessionManager() *ImportSessionManager {
	return &ImportSessionManager{
		sessions:         make(map[string]*importSessionData),
		confirmedResults: make(map[string]confirmedImport),
	}
}

// generateSessionID creates a random session ID.
func generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// CleanupExpired removes expired import sessions. Safe to call from a goroutine.
func (m *ImportSessionManager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, data := range m.sessions {
		if now.After(data.session.ExpiresAt) {
			delete(m.sessions, id)
		}
	}
	for id, rec := range m.confirmedResults {
		if now.After(rec.expiresAt) {
			delete(m.confirmedResults, id)
		}
	}
}

// rememberConfirmed stores the result of a just-consumed session so a retried
// confirm can replay it (T57). The caller must already hold the session's
// confirmMu; the tombstone itself is guarded by the manager mutex.
func (m *ImportSessionManager) rememberConfirmed(sessionID string, userID uint, result models.ImportResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.confirmedResults[sessionID] = confirmedImport{
		userID:    userID,
		result:    result,
		expiresAt: time.Now().Add(sessionExpiry),
	}
}

// replayConfirmed returns the stored result of an already-consumed session if
// it belongs to userID and has not expired yet. It is the idempotent-confirm
// path (T57): a client that lost the first confirm's response retries the same
// session_id and gets the original result instead of a 404 and a duplicate
// re-upload.
func (m *ImportSessionManager) replayConfirmed(sessionID string, userID uint) (models.ImportResult, bool) {
	m.mu.RLock()
	rec, exists := m.confirmedResults[sessionID]
	m.mu.RUnlock()

	if !exists || rec.userID != userID {
		return models.ImportResult{}, false
	}
	if time.Now().After(rec.expiresAt) {
		m.mu.Lock()
		delete(m.confirmedResults, sessionID)
		m.mu.Unlock()
		return models.ImportResult{}, false
	}
	return rec.result, true
}

// get retrieves and validates an import session, enforcing ownership and expiry.
func (m *ImportSessionManager) get(sessionID string, userID uint) (*importSessionData, *apperrors.AppError) {
	m.mu.RLock()
	sessionData, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return nil, apperrors.ErrNotFound("Import session expired or not found")
	}

	if sessionData.session.UserID != userID {
		return nil, apperrors.ErrUnauthorized("Session does not belong to current user")
	}

	if time.Now().After(sessionData.session.ExpiresAt) {
		m.mu.Lock()
		delete(m.sessions, sessionID)
		m.mu.Unlock()
		return nil, apperrors.ErrNotFound("Import session expired")
	}

	return sessionData, nil
}

// Delete removes a session, typically after a completed import.
func (m *ImportSessionManager) Delete(sessionID string) {
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
}

// CreateCSVSession stores a freshly parsed CSV upload and returns its session ID.
func (m *ImportSessionManager) CreateCSVSession(userID uint, headers []string, rows [][]string) string {
	sessionID := generateSessionID()
	now := time.Now()

	m.mu.Lock()
	m.sessions[sessionID] = &importSessionData{
		session: models.ImportSession{
			ID:        sessionID,
			UserID:    userID,
			Headers:   headers,
			Rows:      rows,
			CreatedAt: now,
			ExpiresAt: now.Add(sessionExpiry),
		},
		rows:       rows,
		importType: "csv",
	}
	m.mu.Unlock()

	return sessionID
}

// CreateVCFSession stores a freshly parsed VCF upload (whose preview is computed at
// upload time) and returns its session ID.
func (m *ImportSessionManager) CreateVCFSession(userID uint, vcfContacts []VCFContactData, previews []models.ImportRowPreview) string {
	return m.createVCFLikeSession(userID, vcfContacts, previews, "vcf")
}

// CreateRecordsSession is CreateVCFSession for a batch of neutral Card/CRM
// records (T96's Android device-contacts import): the records are converted
// to flat []VCFContactData by the caller, so the session is shape-identical
// to a VCF session and the existing ConfirmVCF pipeline — including photo
// handling, which device contacts simply never trigger — confirms it. The
// importType is "records" so ConfirmVCF can label merge notes accurately and
// log the real source.
func (m *ImportSessionManager) CreateRecordsSession(userID uint, vcfContacts []VCFContactData, previews []models.ImportRowPreview) string {
	return m.createVCFLikeSession(userID, vcfContacts, previews, "records")
}

// createVCFLikeSession is the shared implementation behind CreateVCFSession/
// CreateRecordsSession; importType is what ConfirmVCF dispatches and labels on.
func (m *ImportSessionManager) createVCFLikeSession(userID uint, vcfContacts []VCFContactData, previews []models.ImportRowPreview, importType string) string {
	sessionID := generateSessionID()
	now := time.Now()

	m.mu.Lock()
	m.sessions[sessionID] = &importSessionData{
		session: models.ImportSession{
			ID:            sessionID,
			UserID:        userID,
			CreatedAt:     now,
			ExpiresAt:     now.Add(sessionExpiry),
			PreviewRows:   previews,
			PreviewCached: true,
		},
		importType:  importType,
		vcfContacts: vcfContacts,
	}
	m.mu.Unlock()

	return sessionID
}

// CreateVCFSessionForShare is CreateVCFSession plus binding the resulting
// session to shareID (P1's ContactShare.ID). This closes a real gap:
// without the binding, ConfirmContactShare would accept *any* session ID
// the client sends as long as it belongs to the same user — including the
// preview session for a *different* pending share, or an unrelated ordinary
// CSV/VCF/JSContact import — and still flip the requested share to
// accepted, decoupling its status/RespondedAt from the data that actually
// landed. Not a cross-user hole (SessionBelongsToShare still requires the
// session to belong to userID first), but a real integrity gap within one
// account. See SessionBelongsToShare, called by ConfirmContactShare before
// ConfirmVCF ever touches the session.
func (m *ImportSessionManager) CreateVCFSessionForShare(userID uint, shareID string, vcfContacts []VCFContactData, previews []models.ImportRowPreview) string {
	sessionID := m.CreateVCFSession(userID, vcfContacts, previews)
	m.mu.Lock()
	if sd, exists := m.sessions[sessionID]; exists {
		sd.boundShareID = shareID
	}
	m.mu.Unlock()
	return sessionID
}

// SessionBelongsToShare reports whether sessionID (already required to
// belong to userID) was minted for shareID's own accept step via
// CreateVCFSessionForShare — false for an expired/foreign/nonexistent
// session, an ordinary (unbound) import session, or one bound to a
// different share.
func (m *ImportSessionManager) SessionBelongsToShare(sessionID string, userID uint, shareID string) bool {
	sessionData, err := m.get(sessionID, userID)
	if err != nil {
		return false
	}
	return shareID != "" && sessionData.boundShareID == shareID
}

// PreviewCSV applies mappings to a CSV session, caches the built contacts and preview
// rows for the confirm step, and returns the preview response.
func (m *ImportSessionManager) PreviewCSV(db *gorm.DB, userID uint, req models.ImportPreviewRequest) (*models.ImportPreviewResponse, *apperrors.AppError) {
	sessionData, err := m.get(req.SessionID, userID)
	if err != nil {
		return nil, err
	}

	contacts, previews, stats := GenerateCSVPreview(db, userID, sessionData.rows, sessionData.session.Headers, req.Mappings)

	// Cache preview data and built contacts in the session for the confirm step.
	m.mu.Lock()
	if sd, exists := m.sessions[req.SessionID]; exists {
		sd.session.Mappings = req.Mappings
		sd.session.PreviewRows = previews
		sd.session.PreviewCached = true
		sd.csvContacts = contacts
	}
	m.mu.Unlock()

	return &models.ImportPreviewResponse{
		SessionID:      req.SessionID,
		Rows:           previews,
		TotalRows:      len(previews),
		ValidRows:      stats.ValidCount,
		DuplicateCount: stats.DuplicateCount,
		ErrorCount:     stats.ErrorCount,
	}, nil
}

// Confirm executes a CSV or VCF import using the per-row actions in req, then
// consumes the session. Consumed sessions are NOT simply dropped: the result is
// kept as a short-lived tombstone so a client that lost the response can retry
// the same session_id and get the same result back (idempotent replay, T57)
// rather than 404ing and re-uploading. Photo processing is handled by ConfirmVCF.
func (m *ImportSessionManager) Confirm(db *gorm.DB, userID uint, req models.ImportConfirmRequest, log *zerolog.Logger) (*models.ImportResult, *apperrors.AppError) {
	sessionData, sessErr := m.get(req.SessionID, userID)
	if sessErr != nil {
		// The session may already have been consumed by an earlier successful
		// confirm whose response the client lost. Replay instead of failing.
		if res, ok := m.replayConfirmed(req.SessionID, userID); ok {
			log.Info().Str("session_id", req.SessionID).Msg("Import confirm replayed from a consumed session")
			return &res, nil
		}
		return nil, sessErr
	}

	// Serialize concurrent confirms of this session: the second one waits,
	// then sees confirmed set and replays instead of applying again.
	sessionData.confirmMu.Lock()
	defer sessionData.confirmMu.Unlock()
	if sessionData.confirmed != nil {
		return sessionData.confirmed, nil
	}

	// Only CSV (and, by symmetry with the web flow, VCF) sessions belong on
	// this endpoint. A records session has nil csvContacts, so reading one
	// here would panic; it must go through ConfirmVCF like every VCF-like
	// session. Mirrors ConfirmVCF's importType guard.
	if sessionData.importType != "csv" && sessionData.importType != "vcf" {
		return nil, apperrors.ErrInvalidInput("session", "This endpoint is only for CSV or VCF imports")
	}

	if !sessionData.session.PreviewCached {
		return nil, apperrors.ErrInvalidInput("session", "Please generate a preview first")
	}

	actionMap := buildActionMap(req.Actions)
	result := models.ImportResult{Errors: []string{}}
	isVCFImport := sessionData.importType == "vcf"

	txErr := db.Transaction(func(tx *gorm.DB) error {
		for _, preview := range sessionData.session.PreviewRows {
			action := actionMap[preview.RowIndex]
			if action == "" {
				action = "skip"
			}

			result.TotalProcessed++

			switch action {
			case "skip":
				result.Skipped++

			case "add":
				var contact models.Contact
				if isVCFImport {
					contact = *sessionData.vcfContacts[preview.RowIndex].Contact
				} else {
					contact = sessionData.csvContacts[preview.RowIndex]
				}
				contact.UserID = userID

				if err := tx.Create(&contact).Error; err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to create contact: %v", preview.RowIndex+1, err))
					result.Skipped++
				} else {
					// Turn the staged circle/tag names into real Circle/Tag +
					// membership rows (T3). Without this the values sit in the
					// flat Contact.Circles column that no UI surface reads any
					// more, so imported groupings were invisible in the app.
					if err := MaterializeImportedGroupings(tx, userID, &contact); err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to apply circles/tags: %v", preview.RowIndex+1, err))
					}
					result.Created++
					if isVCFImport {
						sessionData.vcfContacts[preview.RowIndex].Contact.ID = contact.ID
					}
				}

			case "update":
				if preview.DuplicateMatch == nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Cannot update - no existing contact found", preview.RowIndex+1))
					result.Skipped++
					continue
				}

				var existing models.Contact
				if err := tx.First(&existing, preview.DuplicateMatch.ExistingContactID).Error; err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to fetch existing contact: %v", preview.RowIndex+1, err))
					result.Skipped++
					continue
				}

				importType := "CSV"
				var incoming *models.Contact
				if isVCFImport {
					importType = "VCF"
					incoming = sessionData.vcfContacts[preview.RowIndex].Contact
				} else {
					csvContact := sessionData.csvContacts[preview.RowIndex]
					incoming = &csvContact
				}

				if err := CreateMergeNote(tx, userID, existing.ID, &existing, incoming, importType); err != nil {
					log.Warn().Err(err).Uint("contact_id", existing.ID).Msg("Failed to create merge note")
				}

				MergeImportedContact(&existing, incoming)

				if err := tx.Save(&existing).Error; err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to update contact: %v", preview.RowIndex+1, err))
					result.Skipped++
				} else {
					if err := MaterializeImportedGroupings(tx, userID, &existing); err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to apply circles/tags: %v", preview.RowIndex+1, err))
					}
					result.Updated++
					if isVCFImport {
						sessionData.vcfContacts[preview.RowIndex].Contact.ID = existing.ID
					}
				}
			}
		}

		return nil
	})

	if txErr != nil {
		return nil, apperrors.ErrDatabase("Import failed").WithError(txErr)
	}

	sessionData.confirmed = &result
	m.rememberConfirmed(req.SessionID, userID, result)
	m.Delete(req.SessionID)

	log.Info().
		Str("session_id", req.SessionID).
		Str("import_type", sessionData.importType).
		Int("created", result.Created).
		Int("updated", result.Updated).
		Int("skipped", result.Skipped).
		Int("errors", len(result.Errors)).
		Msg("Import completed")

	return &result, nil
}

// photoTask queues photo processing for a contact, deferred until after the import
// transaction commits since it involves file I/O and network requests.
type photoTask struct {
	contactID      uint
	photoData      []byte
	photoMediaType string
	photoURL       string // URL to fetch photo from (if not embedded)
}

// ConfirmVCF executes a VCF/JSContact/records import with photo processing,
// then consumes the session — keeping a tombstone so a retried confirm of the
// same session_id replays the original result instead of double-importing
// (T57 idempotency; see Confirm's doc comment).
func (m *ImportSessionManager) ConfirmVCF(db *gorm.DB, userID uint, req models.ImportConfirmRequest, cfg *config.Config, log *zerolog.Logger) (*models.ImportResult, *apperrors.AppError) {
	sessionData, sessErr := m.get(req.SessionID, userID)
	if sessErr != nil {
		if res, ok := m.replayConfirmed(req.SessionID, userID); ok {
			log.Info().Str("session_id", req.SessionID).Msg("Import confirm replayed from a consumed session")
			return &res, nil
		}
		return nil, sessErr
	}

	sessionData.confirmMu.Lock()
	defer sessionData.confirmMu.Unlock()
	if sessionData.confirmed != nil {
		return sessionData.confirmed, nil
	}

	if sessionData.importType != "vcf" && sessionData.importType != "records" {
		return nil, apperrors.ErrInvalidInput("session", "This endpoint is only for VCF or records imports")
	}

	// The import-type label used in the per-contact merge note ("VCF Import
	// updated this contact." vs the device-contacts "records" path).
	importTypeLabel := "VCF"
	if sessionData.importType == "records" {
		importTypeLabel = "Device"
	}

	if !sessionData.session.PreviewCached {
		return nil, apperrors.ErrInvalidInput("session", "Please generate a preview first")
	}

	actionMap := buildActionMap(req.Actions)
	result := models.ImportResult{Errors: []string{}}
	var photoTasks []photoTask

	txErr := db.Transaction(func(tx *gorm.DB) error {
		for _, preview := range sessionData.session.PreviewRows {
			action := actionMap[preview.RowIndex]
			if action == "" {
				action = "skip"
			}

			result.TotalProcessed++
			vcfData := sessionData.vcfContacts[preview.RowIndex]

			switch action {
			case "skip":
				result.Skipped++

			case "add":
				contact := *vcfData.Contact
				contact.UserID = userID

				if err := tx.Create(&contact).Error; err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to create contact: %v", preview.RowIndex+1, err))
					result.Skipped++
				} else {
					// T3: materialize circles/tags into real entities. Also
					// covers P1 contact sharing, which accepts a share through
					// this exact method.
					if err := MaterializeImportedGroupings(tx, userID, &contact); err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to apply circles/tags: %v", preview.RowIndex+1, err))
					}
					result.Created++
					// Queue photo processing (either embedded data or URL)
					if len(vcfData.PhotoData) > 0 || vcfData.PhotoURL != "" {
						photoTasks = append(photoTasks, photoTask{
							contactID:      contact.ID,
							photoData:      vcfData.PhotoData,
							photoMediaType: vcfData.PhotoMediaType,
							photoURL:       vcfData.PhotoURL,
						})
					}
				}

			case "update":
				if preview.DuplicateMatch == nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Cannot update - no existing contact found", preview.RowIndex+1))
					result.Skipped++
					continue
				}

				var existing models.Contact
				if err := tx.First(&existing, preview.DuplicateMatch.ExistingContactID).Error; err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to fetch existing contact: %v", preview.RowIndex+1, err))
					result.Skipped++
					continue
				}

				if err := CreateMergeNote(tx, userID, existing.ID, &existing, vcfData.Contact, importTypeLabel); err != nil {
					log.Warn().Err(err).Uint("contact_id", existing.ID).Msg("Failed to create merge note")
				}

				MergeImportedContact(&existing, vcfData.Contact)

				if err := tx.Save(&existing).Error; err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to update contact: %v", preview.RowIndex+1, err))
					result.Skipped++
				} else {
					if err := MaterializeImportedGroupings(tx, userID, &existing); err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("Row %d: Failed to apply circles/tags: %v", preview.RowIndex+1, err))
					}
					result.Updated++
					// Queue photo processing only if contact doesn't already have a photo
					if existing.Photo == "" && (len(vcfData.PhotoData) > 0 || vcfData.PhotoURL != "") {
						photoTasks = append(photoTasks, photoTask{
							contactID:      existing.ID,
							photoData:      vcfData.PhotoData,
							photoMediaType: vcfData.PhotoMediaType,
							photoURL:       vcfData.PhotoURL,
						})
					}
				}
			}
		}

		return nil
	})

	if txErr != nil {
		return nil, apperrors.ErrDatabase("Import failed").WithError(txErr)
	}

	// Process photos outside the transaction (file I/O and network requests).
	for _, task := range photoTasks {
		var photoData []byte
		var mediaType string

		if len(task.photoData) > 0 {
			// Use embedded photo data
			photoData = task.photoData
			mediaType = task.photoMediaType
		} else if task.photoURL != "" {
			// Fetch photo from URL
			var err error
			photoData, mediaType, err = photostore.FetchPhotoFromURL(task.photoURL)
			if err != nil {
				log.Warn().Err(err).Uint("contact_id", task.contactID).Str("photo_url", task.photoURL).Msg("Failed to fetch photo from URL")
				continue
			}
		}

		if len(photoData) == 0 {
			continue
		}

		photoPath, thumbnailData, err := photostore.SaveContactPhoto(photoData, mediaType, cfg.ProfilePhotoDir)
		if err != nil {
			log.Warn().Err(err).Uint("contact_id", task.contactID).Msg("Failed to save imported photo")
			continue
		}

		// Load the row first, then update through the loaded struct rather
		// than a bare Model(&models.Contact{}).Where(...).Updates(map) bulk
		// call. Contact.AfterSave (models/contact.go) recomputes ETag via
		// tx.Model(c).UpdateColumn using the receiver's own ID/UpdatedAt; a
		// bulk update's zero-value receiver has ID 0, so AfterSave's own
		// sub-update has no WHERE clause and GORM rejects it
		// (ErrMissingWhereClause) — and since GORM wraps this single
		// Updates call (plus its hooks) in an implicit transaction, that
		// hook failure rolls back the photo/thumbnail write too, silently
		// discarding it. Same fix shape as contact_sync_service.go's
		// reconcileContactSync and controllers/contact_controller.go's
		// ArchiveContact.
		var contact models.Contact
		if err := db.First(&contact, task.contactID).Error; err != nil {
			log.Warn().Err(err).Uint("contact_id", task.contactID).Msg("Failed to load contact for photo update")
			continue
		}
		if err := db.Model(&contact).Updates(map[string]interface{}{
			"photo":           photoPath,
			"photo_thumbnail": thumbnailData,
		}).Error; err != nil {
			log.Warn().Err(err).Uint("contact_id", task.contactID).Msg("Failed to update contact with photo")
		}
	}

	sessionData.confirmed = &result
	m.rememberConfirmed(req.SessionID, userID, result)
	m.Delete(req.SessionID)

	log.Info().
		Str("session_id", req.SessionID).
		Str("import_type", sessionData.importType).
		Int("created", result.Created).
		Int("updated", result.Updated).
		Int("skipped", result.Skipped).
		Int("photos_processed", len(photoTasks)).
		Int("errors", len(result.Errors)).
		Msg("VCF/records import completed")

	return &result, nil
}

// buildActionMap indexes per-row import actions by row index.
func buildActionMap(actions []models.RowImportAction) map[int]string {
	actionMap := make(map[int]string, len(actions))
	for _, action := range actions {
		actionMap[action.RowIndex] = action.Action
	}
	return actionMap
}
