package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/fireandforget"
	"mycorrhizal/meerkat"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImportSessionPreviewAndConfirmShareSynchronization(t *testing.T) {
	db := setupImportSessionTestDB(t)
	log := testImportLogger()

	headers := []string{"First Name", "Last Name", "Email"}
	rows := make([][]string, 0, 20)
	for i := 0; i < 20; i++ {
		rows = append(rows, []string{"First", fmt.Sprintf("Last%d", i), fmt.Sprintf("race%d@example.com", i)})
	}
	actions := make([]models.RowImportAction, 0, len(rows))
	for i := range rows {
		actions = append(actions, models.RowImportAction{RowIndex: i, Action: "add"})
	}

	for round := 0; round < 15; round++ {
		m := NewImportSessionManager()
		id := m.CreateCSVSession(1, headers, rows)
		_, appErr := m.PreviewCSV(db, 1, models.ImportPreviewRequest{SessionID: id, Mappings: csvMappings()})
		require.Nil(t, appErr)

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 2; i++ {
				_, _ = m.PreviewCSV(db, 1, models.ImportPreviewRequest{SessionID: id, Mappings: csvMappings()})
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			_, _ = m.Confirm(db, 1, models.ImportConfirmRequest{SessionID: id, Actions: actions}, log)
		}()
		close(start)
		wg.Wait()
	}
	fireandforget.Wait()
}

func sourceConfirmFixture(t *testing.T, db *gorm.DB, userID uint) (*ImportSourcePlan, []models.SourceImportRowPreview) {
	t.Helper()
	existing := models.Contact{UserID: userID, Firstname: "Existing", Lastname: "Person"}
	require.NoError(t, db.Create(&existing).Error)

	plan := &ImportSourcePlan{
		System: "test",
		Contacts: []MappedContact{{
			Ref:    SourceRef{System: "test", ExternalID: "contact/1"},
			Record: minimalRecord("Ada", "Lovelace"),
		}},
	}
	previews := []models.SourceImportRowPreview{{
		ImportRowPreview: models.ImportRowPreview{
			RowIndex:        0,
			DuplicateMatch:  &models.DuplicateMatch{ExistingContactID: existing.ID, MatchReason: "name"},
			SuggestedAction: "update",
		},
	}}
	return plan, previews
}

func assertConcurrentConfirmLaunchesOnce(t *testing.T, db *gorm.DB, confirm func() *apperrors.AppError) {
	t.Helper()

	var block atomic.Bool
	block.Store(true)
	var entered atomic.Int32
	gate := make(chan struct{})
	const callback = "issue2058_block_confirm_target_query"
	require.NoError(t, db.Callback().Query().Before("gorm:query").
		Register(callback, func(tx *gorm.DB) {
			if block.Load() && tx.Statement.Table == "contacts" {
				entered.Add(1)
				<-gate
			}
		}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })

	var wg sync.WaitGroup
	errs := make([]*apperrors.AppError, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = confirm()
		}(i)
	}

	require.Eventually(t, func() bool { return entered.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(25 * time.Millisecond)
	block.Store(false)
	close(gate)
	wg.Wait()

	nilCount := 0
	for _, err := range errs {
		if err == nil {
			nilCount++
		}
	}
	assert.Equal(t, 1, nilCount, "the ready-to-importing transition must be atomic: exactly one confirm may launch")
	fireandforget.Wait()
}

func TestMonicaImportManager_ConcurrentConfirmLaunchesOnce(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	plan, previews := sourceConfirmFixture(t, db, user.ID)
	mgr := NewMonicaImportManager()
	log := testImportLogger()

	now := Now()
	s := &monicaImportSession{
		id: "monica-confirm-race", userID: user.ID, phase: models.MonicaPhaseReady,
		plan: plan, previews: previews,
		expiresAt: now.Add(time.Hour), hardExpiry: now.Add(time.Hour),
	}
	mgr.mu.Lock()
	mgr.sessions[s.id] = s
	mgr.mu.Unlock()

	assertConcurrentConfirmLaunchesOnce(t, db, func() *apperrors.AppError {
		return mgr.Confirm(db, user.ID, models.MonicaConfirmRequest{
			SessionID: s.id,
			Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
		}, &config.Config{ProfilePhotoDir: t.TempDir()}, log)
	})
}

func TestMeerkatImportManager_ConcurrentConfirmLaunchesOnce(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	plan, previews := sourceConfirmFixture(t, db, user.ID)
	mgr := NewMeerkatImportManager()
	log := testImportLogger()

	now := Now()
	s := &meerkatImportSession{
		id: "meerkat-confirm-race", userID: user.ID, phase: models.SourceImportPhaseReady,
		plan: plan, previews: previews, snapshot: &meerkat.Snapshot{},
		expiresAt: now.Add(time.Hour), hardExpiry: now.Add(time.Hour),
	}
	mgr.mu.Lock()
	mgr.sessions[s.id] = s
	mgr.mu.Unlock()

	assertConcurrentConfirmLaunchesOnce(t, db, func() *apperrors.AppError {
		return mgr.Confirm(db, user.ID, models.SourceImportConfirmRequest{
			SessionID: s.id,
			Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
		}, log)
	})
}

func TestMycorrhizalImportManager_ConcurrentConfirmLaunchesOnce(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	plan, previews := sourceConfirmFixture(t, db, user.ID)
	mgr := NewMycorrhizalImportManager()
	log := testImportLogger()

	now := Now()
	s := &mycorrhizalImportSession{
		id: "bundle-confirm-race", userID: user.ID, phase: models.SourceImportPhaseReady,
		plan: plan, previews: previews, bundle: &models.AccountBundle{},
		expiresAt: now.Add(time.Hour), hardExpiry: now.Add(time.Hour),
	}
	mgr.mu.Lock()
	mgr.sessions[s.id] = s
	mgr.mu.Unlock()

	assertConcurrentConfirmLaunchesOnce(t, db, func() *apperrors.AppError {
		return mgr.Confirm(db, user.ID, models.SourceImportConfirmRequest{
			SessionID: s.id,
			Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
		}, log)
	})
}

func TestMycorrhizalImportManager_StartFetchVsDeleteIsRaceFree(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	log := testImportLogger()

	for round := 0; round < 100; round++ {
		mgr := NewMycorrhizalImportManager()
		sid := fmt.Sprintf("bundle-fetch-race-%d", round)
		now := Now()
		s := &mycorrhizalImportSession{
			id: sid, userID: user.ID, bundle: &models.AccountBundle{},
			phase:     models.SourceImportPhaseConnecting,
			expiresAt: now.Add(time.Hour), hardExpiry: now.Add(time.Hour),
		}
		mgr.mu.Lock()
		mgr.sessions[sid] = s
		mgr.mu.Unlock()

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_ = mgr.StartFetch(db, user.ID, models.MycorrhizalFetchRequest{SessionID: sid}, log)
		}()
		go func() {
			defer wg.Done()
			<-start
			mgr.Delete(sid)
		}()
		close(start)
		wg.Wait()
	}
	fireandforget.Wait()
}

func TestMeerkatImportManager_StartFetchVsCleanupIsRaceFree(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	log := testImportLogger()

	for round := 0; round < 100; round++ {
		mgr := NewMeerkatImportManager()
		sid := fmt.Sprintf("meerkat-fetch-race-%d", round)
		now := Now()
		s := &meerkatImportSession{
			id: sid, userID: user.ID, snapshot: &meerkat.Snapshot{},
			phase:     models.SourceImportPhaseConnecting,
			expiresAt: now.Add(-time.Second), hardExpiry: now.Add(-time.Second),
		}
		mgr.mu.Lock()
		mgr.sessions[sid] = s
		mgr.mu.Unlock()

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_ = mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: sid}, log)
		}()
		go func() {
			defer wg.Done()
			<-start
			mgr.CleanupExpired()
		}()
		close(start)
		wg.Wait()
	}
	fireandforget.Wait()
}

func TestMonicaImportManager_CancelReplacementVsDeleteAndCleanupIsRaceFree(t *testing.T) {
	for round := 0; round < 200; round++ {
		mgr := NewMonicaImportManager()
		sid := fmt.Sprintf("monica-cancel-race-%d", round)
		now := Now()
		s := &monicaImportSession{
			id: sid, userID: 1, phase: models.MonicaPhaseConnecting,
			cancel:     func() {},
			expiresAt:  now.Add(-time.Second),
			hardExpiry: now.Add(-time.Second),
		}
		mgr.mu.Lock()
		mgr.sessions[sid] = s
		mgr.mu.Unlock()

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
			defer wg.Done()
			<-start
			s.mu.Lock()
			s.cancel = func() {}
			s.mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			mgr.Delete(sid)
		}()
		go func() {
			defer wg.Done()
			<-start
			mgr.CleanupExpired()
		}()
		close(start)
		wg.Wait()
	}
}

func TestMycorrhizalImportManager_ImportIsShutdownTracked(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	log := testImportLogger()

	plan := &ImportSourcePlan{
		System: "test",
		Contacts: []MappedContact{{
			Ref:    SourceRef{System: "test", ExternalID: "contact/1"},
			Record: minimalRecord("Ada", "Lovelace"),
		}},
	}
	previews := []models.SourceImportRowPreview{{
		ImportRowPreview: models.ImportRowPreview{RowIndex: 0, SuggestedAction: "add"},
	}}

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	const callback = "issue2058_block_import_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").
		Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "contacts" {
				once.Do(func() { close(entered) })
				<-release
			}
		}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })

	mgr := NewMycorrhizalImportManager()
	now := Now()
	s := &mycorrhizalImportSession{
		id: "bundle-shutdown-race", userID: user.ID, phase: models.SourceImportPhaseReady,
		plan: plan, previews: previews, bundle: &models.AccountBundle{},
		expiresAt: now.Add(time.Hour), hardExpiry: now.Add(time.Hour),
	}
	mgr.mu.Lock()
	mgr.sessions[s.id] = s
	mgr.mu.Unlock()

	require.Nil(t, mgr.Confirm(db, user.ID, models.SourceImportConfirmRequest{
		SessionID: s.id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}, log))

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("import never reached the contact create")
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, fireandforget.WaitContext(waitCtx), context.DeadlineExceeded,
		"shutdown must wait for a tracked import goroutine instead of closing the DB under it")

	close(release)
	fireandforget.Wait()
}

func assertConfirmResolveFailureRevertsToReady(t *testing.T, db *gorm.DB, confirm func() *apperrors.AppError, phase func() string) {
	t.Helper()
	const callback = "issue2058_force_resolve_failure"
	require.NoError(t, db.Callback().Query().Before("gorm:query").
		Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "contacts" {
				tx.AddError(errors.New("forced resolve failure"))
			}
		}))
	appErr := confirm()
	require.NoError(t, db.Callback().Query().Remove(callback))
	require.NotNil(t, appErr, "an action-resolution failure must be returned")
	assert.Equal(t, models.SourceImportPhaseReady, phase(), "a failed confirm must leave the session retryable")
}

func TestMonicaImportManager_ConfirmResolveFailureRevertsToReady(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	plan, previews := sourceConfirmFixture(t, db, user.ID)
	mgr := NewMonicaImportManager()
	now := Now()
	s := &monicaImportSession{
		id: "monica-resolve-failure", userID: user.ID, phase: models.MonicaPhaseReady,
		plan: plan, previews: previews,
		expiresAt: now.Add(time.Hour), hardExpiry: now.Add(time.Hour),
	}
	mgr.mu.Lock()
	mgr.sessions[s.id] = s
	mgr.mu.Unlock()

	assertConfirmResolveFailureRevertsToReady(t, db, func() *apperrors.AppError {
		return mgr.Confirm(db, user.ID, models.MonicaConfirmRequest{
			SessionID: s.id,
			Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
		}, &config.Config{ProfilePhotoDir: t.TempDir()}, testImportLogger())
	}, func() string {
		st, appErr := mgr.Status(user.ID, s.id)
		require.Nil(t, appErr)
		return st.Phase
	})
}

func TestMeerkatImportManager_ConfirmResolveFailureRevertsToReady(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	plan, previews := sourceConfirmFixture(t, db, user.ID)
	mgr := NewMeerkatImportManager()
	now := Now()
	s := &meerkatImportSession{
		id: "meerkat-resolve-failure", userID: user.ID, phase: models.SourceImportPhaseReady,
		plan: plan, previews: previews, snapshot: &meerkat.Snapshot{},
		expiresAt: now.Add(time.Hour), hardExpiry: now.Add(time.Hour),
	}
	mgr.mu.Lock()
	mgr.sessions[s.id] = s
	mgr.mu.Unlock()

	assertConfirmResolveFailureRevertsToReady(t, db, func() *apperrors.AppError {
		return mgr.Confirm(db, user.ID, models.SourceImportConfirmRequest{
			SessionID: s.id,
			Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
		}, testImportLogger())
	}, func() string {
		st, appErr := mgr.Status(user.ID, s.id)
		require.Nil(t, appErr)
		return st.Phase
	})
}

func TestMycorrhizalImportManager_ConfirmResolveFailureRevertsToReady(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	plan, previews := sourceConfirmFixture(t, db, user.ID)
	mgr := NewMycorrhizalImportManager()
	now := Now()
	s := &mycorrhizalImportSession{
		id: "bundle-resolve-failure", userID: user.ID, phase: models.SourceImportPhaseReady,
		plan: plan, previews: previews, bundle: &models.AccountBundle{},
		expiresAt: now.Add(time.Hour), hardExpiry: now.Add(time.Hour),
	}
	mgr.mu.Lock()
	mgr.sessions[s.id] = s
	mgr.mu.Unlock()

	assertConfirmResolveFailureRevertsToReady(t, db, func() *apperrors.AppError {
		return mgr.Confirm(db, user.ID, models.SourceImportConfirmRequest{
			SessionID: s.id,
			Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
		}, testImportLogger())
	}, func() string {
		st, appErr := mgr.Status(user.ID, s.id)
		require.Nil(t, appErr)
		return st.Phase
	})
}

func TestMonicaImportManager_DeleteCancelsSession(t *testing.T) {
	mgr := NewMonicaImportManager()
	var cancelled atomic.Bool
	now := Now()
	s := &monicaImportSession{
		id: "monica-delete-cancel", userID: 1, phase: models.MonicaPhaseImporting,
		cancel:     func() { cancelled.Store(true) },
		expiresAt:  now.Add(time.Hour),
		hardExpiry: now.Add(time.Hour),
	}
	mgr.mu.Lock()
	mgr.sessions[s.id] = s
	mgr.mu.Unlock()

	mgr.Delete(s.id)
	assert.True(t, cancelled.Load(), "deleting a session must cancel its running work")
}

func TestMeerkatImportManager_DeleteCancelsSession(t *testing.T) {
	mgr := NewMeerkatImportManager()
	var cancelled atomic.Bool
	now := Now()
	s := &meerkatImportSession{
		id: "meerkat-delete-cancel", userID: 1, phase: models.SourceImportPhaseImporting,
		cancel:     func() { cancelled.Store(true) },
		expiresAt:  now.Add(time.Hour),
		hardExpiry: now.Add(time.Hour),
	}
	mgr.mu.Lock()
	mgr.sessions[s.id] = s
	mgr.mu.Unlock()

	mgr.Delete(s.id)
	assert.True(t, cancelled.Load(), "deleting a session must cancel its running work")
}
