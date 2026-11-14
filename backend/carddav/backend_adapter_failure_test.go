package carddav

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubImporter and stubExporter stand in for an adapter that fails on input the
// real adapters accept, so the defensive branches around them can be exercised.
type stubImporter struct{ err error }

func (s stubImporter) Import([]byte) (*contactmodel.Record, []contactmodel.Diagnostic, error) {
	return nil, nil, s.err
}

type stubExporter struct {
	data []byte
	err  error
}

func (s stubExporter) Export(*contactmodel.Record) ([]byte, []contactmodel.Diagnostic, error) {
	return s.data, nil, s.err
}

func swapExporter(t *testing.T, e contactmodel.Exporter) {
	t.Helper()
	old := exporterFor
	exporterFor = func(string) contactmodel.Exporter { return e }
	t.Cleanup(func() { exporterFor = old })
}

func TestBackend_PutRejectsACardTheImporterCannotParse(t *testing.T) {
	b, db := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "u", db, b.photoDir, "")
	old := importerFor
	importerFor = func(vcard.Card) contactmodel.Importer { return stubImporter{err: errors.New("boom")} }
	t.Cleanup(func() { importerFor = old })

	_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/x.vcf", simpleCard("x-uid", "X"), nil)

	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, httpStatusOf(t, err))
	var n int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", 1).Count(&n).Error)
	assert.Zero(t, n, "a rejected PUT must not create a contact")
}

func TestBackend_ContactToAddressObjectServesAnEmptyCardWhenExportFails(t *testing.T) {
	b, db := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "u", db, b.photoDir, "")
	var buf bytes.Buffer
	captureLogger(t, &buf)
	swapExporter(t, stubExporter{err: errors.New("export boom")})

	obj := b.contactToAddressObject(ctx, &models.Contact{UserID: 1, Firstname: "A", VCardUID: "a-uid"})

	require.NotNil(t, obj)
	assert.Empty(t, obj.Card)
	assert.Contains(t, buf.String(), "failed to export contact as vCard")
}

func TestBackend_ContactToAddressObjectServesAnEmptyCardWhenTheExportDoesNotDecode(t *testing.T) {
	b, db := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "u", db, b.photoDir, "")
	var buf bytes.Buffer
	captureLogger(t, &buf)
	swapExporter(t, stubExporter{data: []byte("this is not a vCard")})

	obj := b.contactToAddressObject(ctx, &models.Contact{UserID: 1, Firstname: "A", VCardUID: "a-uid"})

	require.NotNil(t, obj)
	assert.Empty(t, obj.Card)
	assert.Contains(t, buf.String(), "failed to decode exported vCard")
}
