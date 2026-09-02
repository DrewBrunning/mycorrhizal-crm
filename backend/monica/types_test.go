package monica

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSnapshot_ParsesFixtureShape(t *testing.T) {
	t.Parallel()
	raw := `{
	  "contacts": [
	    { "id": 1, "first_name": "Ada", "gender_type": "F", "information": {
	      "dates": { "birthdate": { "date": "1815-12-10T00:00:00Z", "is_year_unknown": false } },
	      "career": { "job": "Programmer", "company": "Co" },
	      "avatar": { "url": "https://x/a.jpg", "source": "photo" },
	      "how_you_met": { "general_information": "met" } },
	      "tags": [{ "name": "Family" }],
	      "contactFields": [ { "content": "ada@x.com", "contact_field_type": { "name": "Email", "protocol": "mailto:", "type": "email" } } ]
	    }
	  ],
	  "relationships": {
	    "1": [ { "relationship_type": { "name": "Spouse" }, "contact_is": { "id": 1, "complete_name": "Ada" }, "of_contact": { "id": 2, "complete_name": "Ben" } } ]
	  },
	  "notes": [ { "id": 3, "body": "hi", "created_at": "2023-01-01T00:00:00Z", "contact": { "id": 1 } } ],
	  "gifts": [ { "id": 4, "name": "Book", "status": "idea", "contact": { "id": 1 } } ]
	}`
	snap, err := LoadSnapshot(strings.NewReader(raw))
	require.NoError(t, err)
	require.Len(t, snap.Contacts, 1)
	c := snap.Contacts[0]
	assert.Equal(t, "Ada", c.FirstName)
	assert.Equal(t, "F", c.GenderType)
	assert.Equal(t, "Programmer", *c.Information.Career.Job)
	assert.Equal(t, "https://x/a.jpg", *c.Information.Avatar.URL)
	assert.Equal(t, "photo", *c.Information.Avatar.Source)
	require.Len(t, c.Tags, 1)
	require.Len(t, c.ContactFields, 1)

	require.Len(t, snap.Relationships[1], 1)
	require.Len(t, snap.Notes, 1)
	require.Len(t, snap.Gifts, 1)
}

func TestLoadSnapshot_InitializesRelationshipMapForEmptySnapshot(t *testing.T) {
	t.Parallel()
	// A snapshot whose JSON explicitly nulls relationships (or omits it)
	// must come back with a usable empty map.
	snap, err := LoadSnapshot(strings.NewReader(`{"contacts": [], "relationships": null}`))
	require.NoError(t, err)
	require.NotNil(t, snap.Relationships)
	assert.Empty(t, snap.Contacts)

	snap, err = LoadSnapshot(strings.NewReader(`{"contacts": []}`))
	require.NoError(t, err)
	require.NotNil(t, snap.Relationships)
}

func TestLoadSnapshot_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()
	_, err := LoadSnapshot(strings.NewReader(`{not json`))
	assert.Error(t, err)
}
