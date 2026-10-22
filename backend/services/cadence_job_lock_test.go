package services

import (
	"encoding/json"
	"io"
	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCadenceJobTestDB(t *testing.T) *gorm.DB {
	logtest.Guard(t) // issue #1474: happy-path tests fail on unexpected warn/error logs
	t.Helper()

	db := dbtest.New(t)
	return db
}

func seedOverduePolicy(t *testing.T, db *gorm.DB, user models.User, contact models.Contact) models.CadencePolicy {
	t.Helper()
	// Last qualifying interaction 40 days ago, 30-day interval -> ~10 days overdue.
	activity := models.Activity{
		UserID: user.ID, Title: "Last call", Date: time.Now().AddDate(0, 0, -40),
		Type: models.InteractionTypeCall, Contacts: []models.Contact{contact},
	}
	require.NoError(t, db.Create(&activity).Error)
	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	require.NoError(t, db.Create(&policy).Error)
	return policy
}

func TestProcessOverdueCadencesSkipsWhenLocked(t *testing.T) {
	db := setupCadenceJobTestDB(t)
	cfg := config.Config{}

	user := models.User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	seedOverduePolicy(t, db, user, contact)

	// A webhook that WOULD receive the emission if the job ran.
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wh := models.Webhook{UserID: user.ID, Name: "vikunja", URL: server.URL, Events: []string{"cadence.overdue"}, Secret: "s", IsActive: true}
	require.NoError(t, db.Create(&wh).Error)

	// Lock row with a recent run -> acquireJobLock must refuse.
	require.NoError(t, db.Create(&models.JobExecution{
		JobName: models.JobNameCadenceOverdue, LastRunAt: time.Now(),
	}).Error)

	require.NotPanics(t, func() { ProcessOverdueCadences(db, cfg) })

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&hits), "the locked job must not emit any webhook")
	var deliveries int64
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Count(&deliveries).Error)
	assert.Zero(t, deliveries)

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameCadenceOverdue).First(&job).Error)
	assert.Nil(t, job.LockedAt, "job must not have been re-locked while rate-limited")
}

func TestProcessOverdueCadencesEmitsWebhookForOverduePolicy(t *testing.T) {
	db := setupCadenceJobTestDB(t)
	cfg := config.Config{}

	user := models.User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	overdueContact := models.Contact{UserID: user.ID, Firstname: "Neglected"}
	require.NoError(t, db.Create(&overdueContact).Error)
	policy := seedOverduePolicy(t, db, user, overdueContact)

	// Not overdue: recent interaction, so no emission for it.
	freshContact := models.Contact{UserID: user.ID, Firstname: "Fresh"}
	require.NoError(t, db.Create(&freshContact).Error)
	fresh := models.Activity{
		UserID: user.ID, Title: "Recent chat", Date: time.Now().AddDate(0, 0, -5),
		Type: models.InteractionTypeVisit, Contacts: []models.Contact{freshContact},
	}
	require.NoError(t, db.Create(&fresh).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: freshContact.VCardUID, TargetIntervalDays: 30}).Error)

	var hits int32
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wh := models.Webhook{UserID: user.ID, Name: "vikunja", URL: server.URL, Events: []string{"cadence.overdue"}, Secret: "s", IsActive: true}
	require.NoError(t, db.Create(&wh).Error)

	require.NotPanics(t, func() { ProcessOverdueCadences(db, cfg) })

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&hits) >= 1
	}, 3*time.Second, 10*time.Millisecond, "the overdue policy must trigger a cadence.overdue webhook")

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits), "exactly one emission (only the overdue policy)")

	// The delivery row is recorded after the receiver has answered, so the
	// hit counter can lead it on a slow runner.
	var deliveries []models.WebhookDelivery
	require.Eventually(t, func() bool {
		deliveries = nil
		return db.Find(&deliveries).Error == nil && len(deliveries) == 1
	}, 3*time.Second, 10*time.Millisecond, "the delivery must be recorded")
	assert.Equal(t, wh.ID, deliveries[0].WebhookID)
	assert.Equal(t, "cadence.overdue", deliveries[0].EventType)

	var envelope struct {
		Event string `json:"event"`
		Data  struct {
			CadencePolicyID string `json:"cadence_policy_id"`
			EntityID        string `json:"entity_id"`
			OverdueBy       int    `json:"overdue_by"`
			ContactID       uint   `json:"contact_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(receivedBody, &envelope))
	assert.Equal(t, "cadence.overdue", envelope.Event)
	assert.Equal(t, policy.ID, envelope.Data.CadencePolicyID)
	assert.Equal(t, overdueContact.VCardUID, envelope.Data.EntityID)
	assert.True(t, envelope.Data.OverdueBy > 0)
	assert.Equal(t, overdueContact.ID, envelope.Data.ContactID)

	// The delivered body above carries the full payload; the stored receipt
	// is the trimmed envelope (issue #622: a successful delivery never needs
	// the entity body it carried).
	var storedEnvelope struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(deliveries[0].Payload), &storedEnvelope))
	assert.Equal(t, "cadence.overdue", storedEnvelope.Event)
	assert.Nil(t, storedEnvelope.Data, "the stored receipt must not retain the entity body")

	// Lock lifecycle: acquired and released.
	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameCadenceOverdue).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock should be released after the job completes")
}

// TestProcessOverdueCadencesSkipsDeceasedContact is the regression test for
// issue #1193: a cadence policy on a contact with a recorded death
// anniversary (Card.Anniversaries[kind=death]) must not emit a
// cadence.overdue webhook, even when the policy is otherwise overdue.
func TestProcessOverdueCadencesSkipsDeceasedContact(t *testing.T) {
	db := setupCadenceJobTestDB(t)
	cfg := config.Config{}

	user := models.User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	deceasedContact := models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(&deceasedContact, &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Neglected"}}},
			Anniversaries: []contactmodel.Anniversary{
				{Kind: "death", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{
					Year: intPtr(2020), Month: intPtr(5), Day: intPtr(1),
				}}},
			},
		},
	}, "")
	require.NoError(t, db.Create(&deceasedContact).Error)
	seedOverduePolicy(t, db, user, deceasedContact)

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wh := models.Webhook{UserID: user.ID, Name: "vikunja", URL: server.URL, Events: []string{"cadence.overdue"}, Secret: "s", IsActive: true}
	require.NoError(t, db.Create(&wh).Error)

	require.NotPanics(t, func() { ProcessOverdueCadences(db, cfg) })

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&hits), "a deceased contact's overdue policy must not emit a webhook")
	var deliveries int64
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Count(&deliveries).Error)
	assert.Zero(t, deliveries)
}

// TestProcessOverdueCadences_NextDueUsesActivityUTCDateAcrossZones covers
// issue #1823 at the webhook boundary: a cadence.overdue payload for an
// activity dated D carries next_due = D + interval and the same overdue_by in
// a reminder zone west of UTC (where the old local conversion pulled D back a
// day) and east of it.
func TestProcessOverdueCadences_NextDueUsesActivityUTCDateAcrossZones(t *testing.T) {
	cases := []struct {
		name   string
		zone   string
		nowUTC time.Time
	}{
		// 09:00 CDT on 2026-10-31 == 14:00Z.
		{"west of UTC", "America/Chicago", time.Date(2026, 10, 31, 14, 0, 0, 0, time.UTC)},
		// 09:00 NZDT on 2026-10-31 == 2026-10-30 20:00Z.
		{"east of UTC", "Pacific/Auckland", time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useFakeClock(t, tc.nowUTC)
			db := setupCadenceJobTestDB(t)
			cfg := config.Config{ReminderTimezone: tc.zone}

			user := models.User{Username: "tz-tester", Password: "x", Email: "tz@example.com"}
			require.NoError(t, db.Create(&user).Error)
			contact := models.Contact{UserID: user.ID, Firstname: "Timezoned"}
			require.NoError(t, db.Create(&contact).Error)
			require.NoError(t, db.Create(&models.Activity{
				UserID: user.ID, Title: "Last call",
				Date:     time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC),
				Type:     models.InteractionTypeCall,
				Contacts: []models.Contact{contact},
			}).Error)
			require.NoError(t, db.Create(&models.CadencePolicy{
				UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 7,
			}).Error)

			received := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				received <- body
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			require.NoError(t, db.Create(&models.Webhook{
				UserID: user.ID, Name: "vikunja", URL: server.URL,
				Events: []string{"cadence.overdue"}, Secret: "s", IsActive: true,
			}).Error)

			require.NotPanics(t, func() { ProcessOverdueCadences(db, cfg) })

			var body []byte
			select {
			case body = <-received:
			case <-time.After(3 * time.Second):
				require.FailNow(t, "overdue cadence must trigger a cadence.overdue webhook")
			}

			var envelope struct {
				Event string `json:"event"`
				Data  struct {
					OverdueBy int             `json:"overdue_by"`
					NextDue   json.RawMessage `json:"next_due"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(body, &envelope))
			assert.Equal(t, "cadence.overdue", envelope.Event)
			assert.JSONEq(t, `"2026-10-29"`, string(envelope.Data.NextDue),
				"activity 2026-10-22 + 7 days, regardless of %s", tc.zone)
			assert.Equal(t, 2, envelope.Data.OverdueBy)
		})
	}
}

// TestProcessOverdueCadencesSkipsSelfContact pins issue #1793's scheduled-job
// half: even a cadence policy on the self contact must not emit a
// cadence.overdue webhook.
func TestProcessOverdueCadencesSkipsSelfContact(t *testing.T) {
	db := setupCadenceJobTestDB(t)
	cfg := config.Config{}

	user := models.User{Username: "selfcadence", Password: "x", Email: "selfcadence@example.com"}
	require.NoError(t, db.Create(&user).Error)
	self := models.Contact{UserID: user.ID, Firstname: "Me"}
	require.NoError(t, db.Create(&self).Error)
	require.NoError(t, db.Model(&user).Update("self_contact_vcard_uid", self.VCardUID).Error)
	seedOverduePolicy(t, db, user, self)

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wh := models.Webhook{UserID: user.ID, Name: "vikunja", URL: server.URL, Events: []string{"cadence.overdue"}, Secret: "s", IsActive: true}
	require.NoError(t, db.Create(&wh).Error)

	emitted, err := ProcessOverdueCadences(db, cfg)
	require.NoError(t, err)
	assert.Zero(t, emitted)

	time.Sleep(150 * time.Millisecond)
	assert.Zero(t, atomic.LoadInt32(&hits), "the self contact must not trigger a cadence.overdue webhook")
	var deliveries int64
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Count(&deliveries).Error)
	assert.Zero(t, deliveries)
}
