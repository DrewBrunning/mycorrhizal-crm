package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are CON-02 (issue #457) scenario deep-dives that complement the
// route-enumerated coverage in routes/conditional_write_matrix_test.go. Each
// maps directly to one of the ticket's "How to verify" bullets, and each runs
// against the real migrated schema (dbtest, CLAUDE.md backend trap #1).

// contactGiven pulls the given-name component out of a contact record response.
func contactGiven(t *testing.T, resp models.ContactRecordResponse) string {
	t.Helper()
	if resp.Card.Name == nil {
		return ""
	}
	for _, comp := range resp.Card.Name.Components {
		if comp.Kind == "given" {
			return comp.Value
		}
	}
	return ""
}

func contactCardBody(given, surname string) models.ContactRecordInput {
	comps := []contactmodel.NameComponent{{Kind: "given", Value: given}}
	if surname != "" {
		comps = append(comps, contactmodel.NameComponent{Kind: "surname", Value: surname})
	}
	return models.ContactRecordInput{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: comps},
	}}
}

// TestConditionalWrite_CanonicalLostUpdateSequence is the textbook lost-update
// race: client A reads, client B reads, B writes, then A writes back with its
// now-stale revision. A's write must be rejected and B's change must survive
// intact — nothing silently overwritten.
func TestConditionalWrite_CanonicalLostUpdateSequence(t *testing.T) {
	env := newCWEnv(t)
	id := strconv.Itoa(int(env.alice.ID))

	// Both clients read the contact at revision 1.
	clientA := int64(1)
	clientB := int64(1)

	// Client B writes first, with its current revision. Accepted; row -> rev 2.
	w := env.do("PUT", "/contacts/"+id, `"`+strconv.FormatInt(clientB, 10)+`"`, contactCardBody("B-change", "Beta"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var bResp models.ContactRecordResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &bResp))
	require.EqualValues(t, 2, bResp.Revision)
	require.Equal(t, "B-change", contactGiven(t, bResp))

	// Client A now writes back with the revision it read at the start (1),
	// unaware of B. This is the lost update — it must be rejected.
	w = env.do("PUT", "/contacts/"+id, `"`+strconv.FormatInt(clientA, 10)+`"`, contactCardBody("A-change", "Alpha"))
	assert412(t, w)

	// B's change survives byte-for-byte: still revision 2, still "B-change".
	var stored models.Contact
	require.NoError(t, env.db.First(&stored, env.alice.ID).Error)
	assert.EqualValues(t, 2, stored.Revision)
	assert.Equal(t, "B-change", stored.Firstname)
	assert.Equal(t, "Beta", stored.Lastname)

	// And A can recover: re-read (revision 2) and retry succeeds.
	w = env.do("PUT", "/contacts/"+id, `"2"`, contactCardBody("A-change", "Alpha"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var aResp models.ContactRecordResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &aResp))
	require.EqualValues(t, 3, aResp.Revision)
	require.Equal(t, "A-change", contactGiven(t, aResp))
}

// TestConditionalWrite_SameSecondWritesAreDistinguished is the #456/#591
// regression at the HTTP layer: two writes to the same row inside the same
// wall-clock second must still be told apart. The old ETag was
// fmt.Sprintf("e-%d-%d", id, UpdatedAt.Unix()) — same second, same token — so a
// conditional check built on it would have waved the second writer through.
// The monotonic counter has no clock input, so the stale write is rejected no
// matter how little time elapsed.
func TestConditionalWrite_SameSecondWritesAreDistinguished(t *testing.T) {
	env := newCWEnv(t)
	id := strconv.Itoa(int(env.alice.ID))

	start := time.Now()
	w := env.do("PUT", "/contacts/"+id, `"1"`, contactCardBody("first", ""))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Immediately, with no sleep: a second writer still holding revision 1.
	w = env.do("PUT", "/contacts/"+id, `"1"`, contactCardBody("second-stale", ""))
	elapsed := time.Since(start)
	assert412(t, w)

	var stored models.Contact
	require.NoError(t, env.db.First(&stored, env.alice.ID).Error)
	assert.Equal(t, "first", stored.Firstname, "the same-second stale write must not have landed")
	assert.EqualValues(t, 2, stored.Revision)

	// Informational: the two writes really did happen inside one second on
	// this run (the condition the old token could not survive). Not asserted
	// hard — a run that straddles a second boundary is still a valid test of
	// the rejection above.
	if elapsed >= time.Second {
		t.Logf("note: the two writes spanned %s (>1s); the rejection is still the real invariant", elapsed)
	}
}

// TestConditionalWrite_PartialUpdateStillRejected covers the ticket's bullet 4:
// a stale write that only means to touch one field is still a lost update for
// that field. The reminder handler is a good probe because it copies fields
// individually onto the loaded row.
func TestConditionalWrite_PartialUpdateStillRejected(t *testing.T) {
	env := newCWEnv(t)
	byMail := false
	reoccur := true
	rem := models.Reminder{
		UserID:                env.user.ID,
		Message:               "call about the roof",
		ByMail:                &byMail,
		RemindAt:              time.Now().Add(72 * time.Hour),
		Recurrence:            "once",
		ReoccurFromCompletion: &reoccur,
		ContactID:             &env.alice.ID,
	}
	require.NoError(t, env.db.Create(&rem).Error)
	id := strconv.Itoa(int(rem.ID))

	// Writer B changes the recurrence (rev 1 -> 2).
	updB := rem
	updB.Recurrence = "monthly"
	w := env.do("PUT", "/reminders/"+id, `"1"`, updB)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Writer A, still on revision 1, only intends to tweak the message — a
	// "small" partial edit. It is still rejected, and B's recurrence change
	// is not clobbered.
	updA := rem
	updA.Message = "call about the roof AND the gutter"
	w = env.do("PUT", "/reminders/"+id, `"1"`, updA)
	assert412(t, w)

	var stored models.Reminder
	require.NoError(t, env.db.First(&stored, rem.ID).Error)
	assert.Equal(t, "call about the roof", stored.Message, "A's partial message edit must not have landed")
	assert.Equal(t, "monthly", stored.Recurrence, "B's recurrence change must survive")
	assert.EqualValues(t, 2, stored.Revision)
}

// TestConditionalWrite_RejectionLeavesRowByteIdentical is the ticket's bullet
// 6 / "verified by comparing the full record": a rejected conditional write
// must not partially apply. Snapshot the whole row, attempt a stale write that
// would have changed several fields, and require the row is unchanged.
func TestConditionalWrite_RejectionLeavesRowByteIdentical(t *testing.T) {
	env := newCWEnv(t)
	id := strconv.Itoa(int(env.alice.ID))

	// Move the row to revision 2 so "1" is unambiguously stale.
	w := env.do("PUT", "/contacts/"+id, `"1"`, contactCardBody("Canonical", "Value"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var before models.Contact
	require.NoError(t, env.db.First(&before, env.alice.ID).Error)
	beforeJSON, err := json.Marshal(before)
	require.NoError(t, err)

	// A stale write touching name components, nickname, and org at once.
	staleBody := models.ContactRecordInput{Card: contactmodel.Card{
		Name:      &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "WIPED"}, {Kind: "surname", Value: "GONE"}}},
		Nicknames: []contactmodel.Nickname{{Name: "should-not-persist"}},
	}}
	w = env.do("PUT", "/contacts/"+id, `"1"`, staleBody)
	assert412(t, w)

	var after models.Contact
	require.NoError(t, env.db.First(&after, env.alice.ID).Error)
	afterJSON, err := json.Marshal(after)
	require.NoError(t, err)

	assert.JSONEq(t, string(beforeJSON), string(afterJSON),
		"a rejected conditional write must leave the stored contact byte-identical")
}
