package models

import (
	"testing"

	"mycorrhizal/contactmodel"
)

// TestApplyName covers every arm of applyName (issue #2913): the FN-only
// split, structured components winning over FN, a partial component set,
// whitespace around FN, and the DeriveProjection fallback for a still-empty
// slot. The import-path regression lives in services/import_vcard_fn_only_test.go;
// this table drives the function directly so each branch is pinned next to it.
func TestApplyName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		card       contactmodel.Card
		proj       contactmodel.Projection
		wantFirst  string
		wantLast   string
		wantMiddle string
		wantPrefix string
		wantSuffix string
	}{
		{
			name:      "multi-word FN splits at the first space",
			card:      contactmodel.Card{Name: &contactmodel.Name{Full: "Fn Only"}},
			wantFirst: "Fn",
			wantLast:  "Only",
		},
		{
			name:      "single-word FN sets only the first name",
			card:      contactmodel.Card{Name: &contactmodel.Name{Full: "Prince"}},
			wantFirst: "Prince",
		},
		{
			name:      "FN is trimmed before splitting",
			card:      contactmodel.Card{Name: &contactmodel.Name{Full: "  Ada Lovelace \t"}},
			wantFirst: "Ada",
			wantLast:  "Lovelace",
		},
		{
			name:      "whitespace-only FN falls back to the projection",
			card:      contactmodel.Card{Name: &contactmodel.Name{Full: "   "}},
			proj:      contactmodel.Projection{Firstname: "Proj", Lastname: "Ection"},
			wantFirst: "Proj",
			wantLast:  "Ection",
		},
		{
			name: "given and surname components win over FN",
			card: contactmodel.Card{Name: &contactmodel.Name{
				Full: "Ignored Name",
				Components: []contactmodel.NameComponent{
					{Kind: "given", Value: "Given"},
					{Kind: "surname", Value: "Surname"},
				},
			}},
			proj:      contactmodel.Projection{Firstname: "Proj", Lastname: "Ection"},
			wantFirst: "Given",
			wantLast:  "Surname",
		},
		{
			name: "only a given component: FN is not split, surname falls back to the projection",
			card: contactmodel.Card{Name: &contactmodel.Name{
				Full:       "Given Only",
				Components: []contactmodel.NameComponent{{Kind: "given", Value: "Given"}},
			}},
			proj:      contactmodel.Projection{Firstname: "Proj", Lastname: "Ection"},
			wantFirst: "Given",
			wantLast:  "Ection",
		},
		{
			name: "only a surname component: FN is not split, given name falls back to the projection",
			card: contactmodel.Card{Name: &contactmodel.Name{
				Full:       "Surname Only",
				Components: []contactmodel.NameComponent{{Kind: "surname", Value: "Surname"}},
			}},
			proj:      contactmodel.Projection{Firstname: "Proj", Lastname: "Ection"},
			wantFirst: "Proj",
			wantLast:  "Surname",
		},
		{
			name: "empty Full with no components uses the projection for both names",
			card: contactmodel.Card{Name: &contactmodel.Name{}},
			proj: contactmodel.Projection{Firstname: "Proj", Lastname: "Ection"},
			// The remaining flat name fields still clear.
			wantFirst: "Proj",
			wantLast:  "Ection",
		},
		{
			name: "no Name at all uses the projection",
			card: contactmodel.Card{},
			proj: contactmodel.Projection{Firstname: "Proj", Lastname: "Ection"},
			// applyName must not dereference a nil Name.
			wantFirst: "Proj",
			wantLast:  "Ection",
		},
		{
			name: "every structured component maps to its flat field",
			card: contactmodel.Card{Name: &contactmodel.Name{
				Components: []contactmodel.NameComponent{
					{Kind: "given", Value: "Ada"},
					{Kind: "given2", Value: "Middle"},
					{Kind: "title", Value: "Dr"},
					{Kind: "credential", Value: "PhD"},
					{Kind: "surname", Value: "Lovelace"},
				},
			}},
			wantFirst:  "Ada",
			wantMiddle: "Middle",
			wantPrefix: "Dr",
			wantSuffix: "PhD",
			wantLast:   "Lovelace",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Pre-fill the flat fields so a missing reset would leak through.
			c := &Contact{
				Firstname: "stale", Lastname: "stale", MiddleName: "stale",
				Prefix: "stale", Suffix: "stale",
			}
			applyName(c, tc.card, tc.proj)
			if c.Firstname != tc.wantFirst {
				t.Errorf("Firstname = %q, want %q", c.Firstname, tc.wantFirst)
			}
			if c.Lastname != tc.wantLast {
				t.Errorf("Lastname = %q, want %q", c.Lastname, tc.wantLast)
			}
			if c.MiddleName != tc.wantMiddle {
				t.Errorf("MiddleName = %q, want %q", c.MiddleName, tc.wantMiddle)
			}
			if c.Prefix != tc.wantPrefix {
				t.Errorf("Prefix = %q, want %q", c.Prefix, tc.wantPrefix)
			}
			if c.Suffix != tc.wantSuffix {
				t.Errorf("Suffix = %q, want %q", c.Suffix, tc.wantSuffix)
			}
		})
	}
}
