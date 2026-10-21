package carddav

// Issue #1733: the ambient flag is CRM-only (no vCard property), so a CardDAV
// PUT -- a full overwrite of the resource by design -- must neither clear it
// nor serve it. The vCard import yields a nil CRMEnvelope.Ambient, which
// ApplyRecordToContact reads as "keep the stored value".

import (
	"context"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCardDAV_PutKeepsAmbientFlagAndGetDoesNotServeIt(t *testing.T) {
	backend, db := newTestBackend(t)
	contact := models.Contact{UserID: 1, Firstname: "Ada", Ambient: true}
	require.NoError(t, db.Create(&contact).Error)
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	urlPath := "/carddav/addressbooks/tester/contacts/" + contact.VCardUID + ".vcf"

	obj, err := backend.GetAddressObject(ctx, urlPath, nil)
	require.NoError(t, err)
	assert.NotContains(t, encodeCard(t, obj.Card), "AMBIENT", "the flag has no vCard home and must not be served")

	_, err = backend.PutAddressObject(ctx, urlPath, clientCard(contact.VCardUID), nil)
	require.NoError(t, err)

	var stored models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", uint(1), contact.VCardUID).First(&stored).Error)
	assert.Equal(t, "Ada Edited", stored.Card.Name.Full, "control: the PUT really overwrote the card")
	assert.True(t, stored.Ambient, "a CardDAV PUT must not clear the CRM-only ambient flag")
	require.NotNil(t, stored.CRM.Ambient)
	assert.True(t, *stored.CRM.Ambient)
}
