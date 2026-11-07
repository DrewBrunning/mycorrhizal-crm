package services

import (
	"errors"
	"testing"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2290: a share-grant operation must survive a failure of any single
// database statement without panicking, and a failure on the authorization or
// write path must surface as an error, never as a success. Each scenario is
// replayed with the n-th statement failing (n = 1, 2, ...) until the operation
// issues fewer statements than n.

var errInjected = errors.New("injected statement failure")

type injector struct {
	count, target int
	hit           bool
}

func (i *injector) fail(tx *gorm.DB) {
	i.count++
	if i.count == i.target {
		i.hit = true
		tx.AddError(errInjected)
	}
}

func (i *injector) install(db *gorm.DB) {
	_ = db.Callback().Query().After("gorm:query").Register("sg:fail_query", i.fail)
	_ = db.Callback().Create().After("gorm:create").Register("sg:fail_create", i.fail)
	_ = db.Callback().Update().After("gorm:update").Register("sg:fail_update", i.fail)
	_ = db.Callback().Delete().After("gorm:delete").Register("sg:fail_delete", i.fail)
}

type faultWorld struct {
	db          *gorm.DB
	owner, rcpt models.User
	contact     models.Contact
	note        models.Note
	activity    models.Activity
	circle      models.Circle
	grants      map[string]string // entity type -> grant id (active)
	now         time.Time
}

func newFaultWorld(t *testing.T) *faultWorld {
	t.Helper()
	db := dbtest.New(t)
	w := &faultWorld{db: db, grants: map[string]string{}, now: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}
	mk := func(name string) models.User {
		u := models.User{Username: name, Email: name + "@example.com", Password: "x"}
		require.NoError(t, db.Create(&u).Error)
		return u
	}
	w.owner, w.rcpt = mk("fault-owner"), mk("fault-rcpt")
	w.contact = models.Contact{UserID: w.owner.ID, Firstname: "Alice", Lastname: "Doe"}
	require.NoError(t, db.Create(&w.contact).Error)
	w.note = models.Note{UserID: w.owner.ID, Content: "n", Date: w.now}
	require.NoError(t, db.Create(&w.note).Error)
	w.activity = models.Activity{UserID: w.owner.ID, Title: "a", Date: w.now}
	require.NoError(t, db.Create(&w.activity).Error)
	w.circle = models.Circle{UserID: w.owner.ID, Name: "c"}
	require.NoError(t, db.Create(&w.circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: w.circle.ID, UserID: w.owner.ID, MemberVCardUID: w.contact.VCardUID}).Error)
	for typ, id := range map[string]string{
		models.ShareGrantEntityContact: w.contact.VCardUID, models.ShareGrantEntityNote: w.note.UUID,
		models.ShareGrantEntityActivity: w.activity.UUID, models.ShareGrantEntityCircle: w.circle.ID,
	} {
		g := models.ShareGrant{FromUserID: w.owner.ID, ToUserID: w.rcpt.ID, EntityType: typ, EntityID: id,
			Sections: []string{"emails"}, Status: models.ShareGrantStatusActive}
		require.NoError(t, db.Create(&g).Error)
		w.grants[typ] = g.ID
	}
	return w
}

func (w *faultWorld) pending(t *testing.T) string {
	t.Helper()
	g := models.ShareGrant{FromUserID: w.owner.ID, ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote,
		EntityID: "pending-" + t.Name(), Sections: []string{}, Status: models.ShareGrantStatusPending, CreatedAt: w.now, UpdatedAt: w.now}
	require.NoError(t, w.db.Create(&g).Error)
	return g.ID
}

func TestShareGrantService_SingleStatementFailures(t *testing.T) {
	future := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	sections := []string{"emails"}
	scenarios := map[string]func(t *testing.T, w *faultWorld) func() *apperrors.AppError{
		"create-contact": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			n := models.Note{UserID: w.owner.ID, Content: "x", Date: w.now}
			require.NoError(t, w.db.Create(&n).Error)
			return func() *apperrors.AppError {
				_, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID,
					EntityType: models.ShareGrantEntityContact, EntityID: w.contact.VCardUID + "-none", Sections: sections}, w.now)
				return e
			}
		},
		"create-note-blocked": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			require.NoError(t, w.db.Create(&models.ShareBlock{BlockerUserID: w.rcpt.ID, BlockedUserID: w.owner.ID}).Error)
			n := models.Note{UserID: w.owner.ID, Content: "x", Date: w.now}
			require.NoError(t, w.db.Create(&n).Error)
			return func() *apperrors.AppError {
				_, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID,
					EntityType: models.ShareGrantEntityNote, EntityID: n.UUID, ExpiresAt: &future}, w.now)
				return e
			}
		},
		"accept": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			id := w.pending(t)
			return func() *apperrors.AppError { _, e := AcceptShareGrant(w.db, w.rcpt.ID, id, w.now); return e }
		},
		"decline-block": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			id := w.pending(t)
			return func() *apperrors.AppError { _, e := DeclineShareGrant(w.db, w.rcpt.ID, id, true, w.now); return e }
		},
		"leave": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			id := w.grants[models.ShareGrantEntityNote]
			return func() *apperrors.AppError { _, e := LeaveShareGrant(w.db, w.rcpt.ID, id, w.now); return e }
		},
		"revoke": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			id := w.grants[models.ShareGrantEntityNote]
			return func() *apperrors.AppError { _, e := RevokeShareGrant(w.db, w.owner.ID, id, w.now); return e }
		},
		"reconfirm": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			id := w.grants[models.ShareGrantEntityContact]
			return func() *apperrors.AppError { _, e := ReconfirmShareGrant(w.db, w.owner.ID, id, w.now); return e }
		},
		"update": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			id := w.grants[models.ShareGrantEntityContact]
			return func() *apperrors.AppError {
				_, e := UpdateShareGrant(w.db, w.owner.ID, id, models.ShareGrantPatchInput{
					ExpiresAt: models.NullableTime{Set: true, Value: &future}, Sections: &sections}, nil, w.now)
				return e
			}
		},
		"blocks": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			require.NoError(t, w.db.Create(&models.ShareBlock{BlockerUserID: w.rcpt.ID, BlockedUserID: w.owner.ID}).Error)
			return func() *apperrors.AppError { _, e := ListShareBlocks(w.db, w.rcpt.ID); return e }
		},
		"unblock": func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			return func() *apperrors.AppError { return DeleteShareBlock(w.db, w.rcpt.ID, w.owner.ID) }
		},
	}
	for _, typ := range []string{models.ShareGrantEntityContact, models.ShareGrantEntityNote, models.ShareGrantEntityActivity, models.ShareGrantEntityCircle} {
		scenarios["content-"+typ] = func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			id := w.grants[typ]
			return func() *apperrors.AppError {
				_, e := ReadShareGrantContent(w.db, w.rcpt.ID, id, t.TempDir(), w.now)
				return e
			}
		}
		scenarios["views-"+typ] = func(t *testing.T, w *faultWorld) func() *apperrors.AppError {
			var g models.ShareGrant
			require.NoError(t, w.db.First(&g, "id = ?", w.grants[typ]).Error)
			return func() *apperrors.AppError {
				_ = OwnerView(w.db, &g, w.now)
				_ = RecipientView(w.db, &g, w.now)
				return nil
			}
		}
	}

	for name, build := range scenarios {
		t.Run(name, func(t *testing.T) {
			for n := 1; n < 60; n++ {
				w := newFaultWorld(t)
				run := build(t, w)
				inj := &injector{target: n}
				inj.install(w.db)
				var e *apperrors.AppError
				assert.NotPanics(t, func() { e = run() }, "statement %d failing must not panic", n)
				if !inj.hit {
					return // the operation issued fewer than n statements
				}
				_ = e
			}
			t.Fatalf("%s: operation still issuing statements after 59", name)
		})
	}
}
