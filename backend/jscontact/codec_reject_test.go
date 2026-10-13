package jscontact

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectTypes lists every JSContact object type with its own UnmarshalJSON,
// paired with a constructor for a fresh zero value and the @type it demands.
// (AnniversaryDate, PartialDate and Timestamp are polymorphic/value shapes and
// are covered separately below.)
func rejectTypes() map[string]struct {
	fresh func() json.Unmarshaler
	typ   string
} {
	type entry = struct {
		fresh func() json.Unmarshaler
		typ   string
	}
	return map[string]entry{
		"SpeakToAs":         {func() json.Unmarshaler { return &SpeakToAs{} }, "SpeakToAs"},
		"Anniversary":       {func() json.Unmarshaler { return &Anniversary{} }, "Anniversary"},
		"Name":              {func() json.Unmarshaler { return &Name{} }, "Name"},
		"NameComponent":     {func() json.Unmarshaler { return &NameComponent{} }, "NameComponent"},
		"AddressComponent":  {func() json.Unmarshaler { return &AddressComponent{} }, "AddressComponent"},
		"Organization":      {func() json.Unmarshaler { return &Organization{} }, "Organization"},
		"OrgUnit":           {func() json.Unmarshaler { return &OrgUnit{} }, "OrgUnit"},
		"Title":             {func() json.Unmarshaler { return &Title{} }, "Title"},
		"PersonalInfo":      {func() json.Unmarshaler { return &PersonalInfo{} }, "PersonalInfo"},
		"Note":              {func() json.Unmarshaler { return &Note{} }, "Note"},
		"Author":            {func() json.Unmarshaler { return &Author{} }, "Author"},
		"Address":           {func() json.Unmarshaler { return &Address{} }, "Address"},
		"Phone":             {func() json.Unmarshaler { return &Phone{} }, "Phone"},
		"EmailAddress":      {func() json.Unmarshaler { return &EmailAddress{} }, "EmailAddress"},
		"OnlineService":     {func() json.Unmarshaler { return &OnlineService{} }, "OnlineService"},
		"Calendar":          {func() json.Unmarshaler { return &Calendar{} }, "Calendar"},
		"SchedulingAddress": {func() json.Unmarshaler { return &SchedulingAddress{} }, "SchedulingAddress"},
		"CryptoKey":         {func() json.Unmarshaler { return &CryptoKey{} }, "CryptoKey"},
		"Directory":         {func() json.Unmarshaler { return &Directory{} }, "Directory"},
		"Link":              {func() json.Unmarshaler { return &Link{} }, "Link"},
		"Media":             {func() json.Unmarshaler { return &Media{} }, "Media"},
		"Nickname":          {func() json.Unmarshaler { return &Nickname{} }, "Nickname"},
		"LanguagePref":      {func() json.Unmarshaler { return &LanguagePref{} }, "LanguagePref"},
		"Pronouns":          {func() json.Unmarshaler { return &Pronouns{} }, "Pronouns"},
		"Relation":          {func() json.Unmarshaler { return &Relation{} }, "Relation"},
	}
}

// TestUnmarshalJSON_RejectsNonObject pins that every typed object refuses a
// JSON value that is not an object, with the decoder's own type error rather
// than silently producing a zero value.
func TestUnmarshalJSON_RejectsNonObject(t *testing.T) {
	t.Parallel()
	for name, e := range rejectTypes() {
		t.Run(name, func(t *testing.T) {
			err := e.fresh().UnmarshalJSON([]byte(`[1,2]`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot unmarshal array")
		})
	}
}

// TestUnmarshalJSON_RejectsForeignType pins that every typed object refuses a
// payload whose @type names a different JSContact type.
func TestUnmarshalJSON_RejectsForeignType(t *testing.T) {
	t.Parallel()
	for name, e := range rejectTypes() {
		t.Run(name, func(t *testing.T) {
			err := e.fresh().UnmarshalJSON([]byte(`{"@type":"NotThisType"}`))
			require.Error(t, err)
			assert.Equal(t, `jscontact: expected @type "`+e.typ+`", got "NotThisType"`, err.Error())
		})
	}
}

func TestAnniversaryDate_UnmarshalJSON_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, in, wantContains string }{
		{"non-object", `[1]`, "cannot unmarshal array"},
		{"unknown @type", `{"@type":"Bogus"}`, `jscontact: Anniversary.date has unknown @type "Bogus"`},
		{"timestamp with wrong utc type", `{"@type":"Timestamp","utc":5}`, "cannot unmarshal number"},
		{"partial date with wrong year type", `{"@type":"PartialDate","year":"x"}`, "cannot unmarshal string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d AnniversaryDate
			err := d.UnmarshalJSON([]byte(tc.in))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantContains)
			assert.Nil(t, d.Timestamp)
			assert.Nil(t, d.PartialDate)
		})
	}
}
