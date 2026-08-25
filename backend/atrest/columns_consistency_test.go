package atrest_test

import (
	"reflect"
	"strings"
	"testing"

	"mycorrhizal/atrest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
)

// TestEncryptedColumns_MatchesSerializerTags pins the two halves of the
// at-rest feature to each other: every model field tagged
// `serializer:encrypted`/`serializer:encryptedjson` must appear in
// atrest.EncryptedColumns (table.column), and vice versa. A column added to
// one list without the other silently leaves pre-existing rows unencrypted
// forever (or backfills a column the model never encrypts) — this test makes
// that drift a build failure.
func TestEncryptedColumns_MatchesSerializerTags(t *testing.T) {
	got := map[string]bool{}
	collectEncrypted(t, models.Contact{}, got)
	collectEncrypted(t, models.LifeEvent{}, got)
	collectEncrypted(t, models.Reminder{}, got)
	collectEncrypted(t, models.ReminderCompletion{}, got)
	collectEncrypted(t, models.Gift{}, got)
	collectEncrypted(t, models.Preference{}, got)
	collectEncrypted(t, models.ConversationAgenda{}, got)
	collectEncrypted(t, models.AuditEvent{}, got)
	collectEncrypted(t, models.ContactSyncConflict{}, got)
	collectEncrypted(t, models.ContactSyncLink{}, got)

	want := map[string]bool{}
	for _, spec := range atrest.EncryptedColumns {
		want[spec] = true
	}

	for spec := range got {
		require.True(t, want[spec], "field %s is serializer-encrypted but missing from atrest.EncryptedColumns", spec)
	}
	for spec := range want {
		require.True(t, got[spec], "atrest.EncryptedColumns lists %s but no model field carries its serializer tag", spec)
	}
}

// collectEncrypted walks a model struct and records every field whose gorm
// tag carries serializer:encrypted or serializer:encryptedjson, keyed as
// "table.column" using the field's explicit gorm column tag (or the GORM
// default snake_case of the field name).
func collectEncrypted(t *testing.T, v interface{}, out map[string]bool) {
	t.Helper()
	rt := reflect.TypeOf(v)
	tableName := tableNameFor(rt)

	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		gormTag := f.Tag.Get("gorm")
		if !strings.Contains(gormTag, "serializer:encrypted") {
			continue
		}
		column := columnNameFor(f)
		out[tableName+"."+column] = true
	}
}

func tableNameFor(rt reflect.Type) string {
	if tn, ok := reflect.New(rt).Interface().(interface{ TableName() string }); ok {
		return tn.TableName()
	}
	// Fall back to GORM's pluralized snake_case for the handful of models
	// that don't define TableName (none of the encrypted ones should hit this).
	name := rt.Name()
	var b strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	s := strings.ToLower(b.String())
	if strings.HasSuffix(s, "y") {
		s = s[:len(s)-1] + "ies"
	} else if !strings.HasSuffix(s, "s") {
		s += "s"
	}
	return s
}

func columnNameFor(f reflect.StructField) string {
	tag := f.Tag.Get("gorm")
	// gorm tag format: "column:how_we_met;serializer:encrypted"
	for _, part := range strings.Split(tag, ";") {
		if strings.HasPrefix(part, "column:") {
			return strings.TrimPrefix(part, "column:")
		}
	}
	return toSnake(f.Name)
}

func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}
