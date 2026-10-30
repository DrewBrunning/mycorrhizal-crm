package controllers

// Issue #2255: the same contact write over REST and over a CardDAV PUT must
// leave identical derived state -- the webhook event, the married LifeEvent
// mirrored from the wedding anniversary, the flat name columns and the audit
// trail -- because both go through services.SaveContact.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"mycorrhizal/carddav"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// derivedState is everything a contact write derives that the two protocols
// must agree on.
type derivedState struct {
	WebhookEvents []string
	Married       []string // formatted dates of the married life events
	Firstname     string
	FN            string
	AuditOps      []string
}

type parityCase struct {
	name     string
	existing *contactmodel.PartialDate // wedding on the stored contact; nil = no stored contact
	incoming *contactmodel.PartialDate // wedding on the written card; nil = none
}

func ymd(y, m, d int) *contactmodel.PartialDate {
	return &contactmodel.PartialDate{Year: &y, Month: &m, Day: &d}
}

func ymdString(p *contactmodel.PartialDate) string {
	if p == nil || p.Year == nil || p.Month == nil || p.Day == nil {
		return "?"
	}
	b, _ := json.Marshal([]int{*p.Year, *p.Month, *p.Day})
	return string(b)
}

// webhookSink registers an active webhook for the contact events and records
// each delivered event name.
func webhookSink(t *testing.T, db *gorm.DB, userID uint) func() []string {
	t.Helper()
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Get("X-Mycorrhizal-Event"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, db.Create(&models.Webhook{UserID: userID, Name: "h", URL: srv.URL,
		Events: []string{"contact.created", "contact.updated"}, IsActive: true}).Error)
	return func() []string {
		services.WaitForWebhookGoroutines()
		mu.Lock()
		defer mu.Unlock()
		out := append([]string(nil), got...)
		sort.Strings(out)
		return out
	}
}

func readDerivedState(t *testing.T, db *gorm.DB, userID uint, uid string, events []string) derivedState {
	t.Helper()
	var c models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", userID, uid).First(&c).Error)
	st := derivedState{WebhookEvents: events, Firstname: c.Firstname, FN: c.FN}
	var evs []models.LifeEvent
	require.NoError(t, db.Where("user_id = ? AND entity_id = ? AND type = ?", userID, uid, models.LifeEventTypeMarried).Find(&evs).Error)
	for _, e := range evs {
		st.Married = append(st.Married, ymdString(e.Date))
	}
	var audits []models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ?", models.AuditEntityContact, uid).Order("id").Find(&audits).Error)
	for _, a := range audits {
		st.AuditOps = append(st.AuditOps, a.Operation)
	}
	return st
}

// seedStored creates the pre-existing contact (with its mirrored married
// event) directly, identically for both sides.
func seedStored(t *testing.T, db *gorm.DB, userID uint, wedding *contactmodel.PartialDate) models.Contact {
	t.Helper()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Alice"}}},
		Anniversaries: []contactmodel.Anniversary{
			{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: wedding}},
		},
	}}
	c := models.Contact{UserID: userID}
	models.ApplyRecordToContact(&c, rec, "")
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, services.SyncWeddingFromCard(db, userID, c.VCardUID, wedding))
	return c
}

func restSide(t *testing.T, tc parityCase) derivedState {
	t.Helper()
	db, router := setupRouter(t)
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)
	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	events := webhookSink(t, db, user.ID)

	body, _ := json.Marshal(weddingContactPayload(tc.incoming))
	var req *http.Request
	var uid string
	if tc.existing != nil {
		c := seedStored(t, db, user.ID, tc.existing)
		uid = c.VCardUID
		req, _ = http.NewRequest(http.MethodPut, fmt.Sprintf("/contacts/%d", c.ID), bytes.NewBuffer(body))
	} else {
		req, _ = http.NewRequest(http.MethodPost, "/contacts", bytes.NewBuffer(body))
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Less(t, w.Code, 300, w.Body.String())
	if uid == "" {
		var latest models.Contact
		require.NoError(t, db.Where("user_id = ?", user.ID).Order("id desc").First(&latest).Error)
		uid = latest.VCardUID
	}
	return readDerivedState(t, db, user.ID, uid, events())
}

func davSide(t *testing.T, tc parityCase) derivedState {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "tester", Password: "password123", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	events := webhookSink(t, db, user.ID)
	backend := carddav.NewBackend(db, t.TempDir())
	ctx := carddav.ContextWithUser(context.Background(), user.ID, user.Username, db, t.TempDir(), "")

	uid := "dav-parity-uid"
	if tc.existing != nil {
		uid = seedStored(t, db, user.ID, tc.existing).VCardUID
	}
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "4.0")
	card.SetValue(vcard.FieldUID, uid)
	card.SetValue(vcard.FieldFormattedName, "Alice")
	card.SetValue(vcard.FieldName, ";Alice;;;")
	if tc.incoming != nil {
		card.SetValue("ANNIVERSARY", ymdCompact(tc.incoming))
	}
	_, err := backend.PutAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+uid+".vcf", card, nil)
	require.NoError(t, err)
	return readDerivedState(t, db, user.ID, uid, events())
}

func ymdCompact(p *contactmodel.PartialDate) string {
	return fmt.Sprintf("%04d%02d%02d", *p.Year, *p.Month, *p.Day)
}

func TestCardDAVPutAndRESTWriteLeaveIdenticalDerivedState(t *testing.T) {
	cases := []parityCase{
		{name: "create with wedding", incoming: ymd(2009, 8, 16)},
		{name: "create without wedding"},
		{name: "update moves the wedding", existing: ymd(2009, 8, 16), incoming: ymd(2011, 5, 20)},
		{name: "update removes the wedding", existing: ymd(2009, 8, 16)},
		{name: "update keeps the wedding", existing: ymd(2009, 8, 16), incoming: ymd(2009, 8, 16)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rest := restSide(t, tc)
			dav := davSide(t, tc)
			assert.Equal(t, rest, dav)
			require.NotEmpty(t, rest.WebhookEvents, "control: the REST side really fired a webhook")
		})
	}
}
