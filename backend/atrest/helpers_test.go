package atrest

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// Pure-function unit tests for backfill.go/serializers.go's small helpers.
// These are exercised indirectly by the integration/backfill tests above via
// real tables, but every real table in this schema happens to use only a
// string or uint PK, so the other idAsString/dbString branches (int64,
// []byte, unsupported types) and the isNoSuchTable/splitSpec edge cases never
// fire through that path. Testing them directly is cheap and pins the
// contract without needing a fabricated DB driver quirk.

func TestIdAsString(t *testing.T) {
	tests := []struct {
		name    string
		in      interface{}
		want    string
		wantErr bool
	}{
		{"string", "vcard-uuid-123", "vcard-uuid-123", false},
		{"int64", int64(42), "42", false},
		{"uint64", uint64(42), "42", false},
		{"int", 42, "42", false},
		{"bytes", []byte("blob-id"), "blob-id", false},
		{"unsupported", 3.14, "", true},
		{"nil", nil, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := idAsString(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIsNoSuchTable(t *testing.T) {
	require.True(t, isNoSuchTable(errors.New("no such table: widgets")))
	require.False(t, isNoSuchTable(errors.New("no such column: foo")))
	require.False(t, isNoSuchTable(nil))
}

func TestSplitSpec(t *testing.T) {
	tests := []struct {
		spec       string
		wantTable  string
		wantColumn string
		wantOK     bool
	}{
		{"contacts.card", "contacts", "card", true},
		{"life_events.description", "life_events", "description", true},
		{"nodothere", "", "", false},
		{".column", "", "", false},
		{"table.", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			table, column, ok := splitSpec(tc.spec)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, tc.wantTable, table)
				require.Equal(t, tc.wantColumn, column)
			}
		})
	}
}

func TestDbString(t *testing.T) {
	require.Equal(t, "", dbString(nil))
	require.Equal(t, "hello", dbString("hello"))
	require.Equal(t, "hello", dbString([]byte("hello")))
	require.Equal(t, "", dbString(42), "unrecognized driver value types default to empty, not a panic")
}
