package semanticequal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"
)

// byConcept maps every correspondence concept_id to its extractor: a
// function that reads the concept's value(s) off a neutral Record and
// returns them as normalized, comparable string keys. The extractor for each
// concept follows the table's own neutral_path and transform columns — the
// granularity is the table's, not a parallel notion of "the same"
// (docs/adrs/0002-correspondence-table-locked-oracle.md).
//
// Rows whose transform declares the WHOLE element as the unit (org_units,
// onlineservice, adr_components, related, personalinfo) extract the element
// with its jointly-carried sibling fields, per the row's own notes;
// everything else extracts exactly what the neutral_path names.
var byConcept map[string]func(*contactmodel.Record) []string

func init() {
	byConcept = map[string]func(*contactmodel.Record) []string{
		// --- identity / meta ---
		"uid":      scalar(func(r *contactmodel.Record) string { return r.Card.UID }),
		"kind":     scalar(func(r *contactmodel.Record) string { return r.Card.Kind }),
		"prodid":   scalar(func(r *contactmodel.Record) string { return r.Card.ProdID }),
		"updated":  timestampOf(func(r *contactmodel.Record) *contactmodel.Timestamp { return r.Card.Updated }),
		"created":  timestampOf(func(r *contactmodel.Record) *contactmodel.Timestamp { return r.Card.Created }),
		"language": scalar(func(r *contactmodel.Record) string { return r.Card.Language }),

		// --- name ---
		"name.full": scalar(func(r *contactmodel.Record) string {
			if r.Card.Name == nil {
				return ""
			}
			return r.Card.Name.Full
		}),
		"name.surname":    nameComponent("surname"),
		"name.given":      nameComponent("given"),
		"name.given2":     nameComponent("given2"),
		"name.title":      nameComponent("title"),
		"name.credential": nameComponent("credential"),
		"name.surname2":   nameComponent("surname2"),
		"name.generation": nameComponent("generation"),
		"name.phonetic":   namePhonetic,
		"nickname":        sliceOf(func(r *contactmodel.Record) []contactmodel.Nickname { return r.Card.Nicknames }, func(n contactmodel.Nickname) string { return n.Name }),

		// --- organizations / titles ---
		"org": sliceOf(func(r *contactmodel.Record) []contactmodel.Organization { return r.Card.Organizations }, orgKey),
		"org.unit": func(r *contactmodel.Record) []string {
			var out []string
			for _, o := range r.Card.Organizations {
				for _, u := range o.Units {
					if u.Name != "" {
						out = append(out, u.Name)
					}
				}
			}
			return out
		},
		"title": titleKind("title"),
		"role":  titleKind("role"),

		// --- contact methods ---
		"email":  sliceOf(func(r *contactmodel.Record) []contactmodel.Email { return r.Card.Emails }, func(e contactmodel.Email) string { return e.Address }),
		"phone":  sliceOf(func(r *contactmodel.Record) []contactmodel.Phone { return r.Card.Phones }, func(p contactmodel.Phone) string { return p.Number }),
		"impp":   sliceOf(func(r *contactmodel.Record) []contactmodel.OnlineService { return r.Card.ImppAddresses }, onlineServiceKey),
		"social": sliceOf(func(r *contactmodel.Record) []contactmodel.OnlineService { return r.Card.SocialProfiles }, onlineServiceKey),

		// --- addresses ---
		"adr":     sliceOf(func(r *contactmodel.Record) []contactmodel.Address { return r.Card.Addresses }, addressKey),
		"adr.geo": addressField(func(a contactmodel.Address) string { return a.Coordinates }),
		"adr.tz":  addressField(func(a contactmodel.Address) string { return a.TimeZone }),

		// --- anniversaries ---
		"anniversary.birth":       anniversaryKind("birth"),
		"anniversary.wedding":     anniversaryKind("wedding"),
		"anniversary.death":       anniversaryKind("death"),
		"anniversary.place.birth": anniversaryPlace("birth"),
		"anniversary.place.death": anniversaryPlace("death"),

		// --- speakToAs ---
		"gramgender": func(r *contactmodel.Record) []string {
			if r.Card.SpeakToAs == nil {
				return nil
			}
			var out []string
			for _, g := range r.Card.SpeakToAs.GrammaticalGenders {
				if v := strings.ToLower(g.Value); v != "" {
					out = append(out, v)
				}
			}
			return out
		},
		"pronouns": func(r *contactmodel.Record) []string {
			if r.Card.SpeakToAs == nil {
				return nil
			}
			var out []string
			for _, p := range r.Card.SpeakToAs.Pronouns {
				if p.Pronouns != "" {
					out = append(out, p.Pronouns)
				}
			}
			return out
		},

		// --- personal info ---
		"expertise": personalInfoKind("expertise"),
		"hobby":     personalInfoKind("hobby"),
		"interest":  personalInfoKind("interest"),

		// --- notes / keywords ---
		"note": sliceOf(func(r *contactmodel.Record) []contactmodel.Note { return r.Card.Notes }, noteKey),
		"keywords": func(r *contactmodel.Record) []string {
			var out []string
			for _, k := range r.Card.Keywords {
				if k != "" {
					out = append(out, k)
				}
			}
			return out
		},

		// --- resources ---
		"photo":      mediaKind("photo"),
		"logo":       mediaKind("logo"),
		"sound":      mediaKind("sound"),
		"calendar":   sliceOf(func(r *contactmodel.Record) []contactmodel.Resource { return r.Card.Calendars }, resourceURI),
		"freebusy":   sliceOf(func(r *contactmodel.Record) []contactmodel.Resource { return r.Card.FreeBusyURLs }, resourceURI),
		"caladruri":  sliceOf(func(r *contactmodel.Record) []contactmodel.Resource { return r.Card.SchedulingAddresses }, resourceURI),
		"key":        sliceOf(func(r *contactmodel.Record) []contactmodel.Resource { return r.Card.CryptoKeys }, resourceURI),
		"directory":  func(r *contactmodel.Record) []string { return directoriesOfKind(r, "directory") },
		"source":     func(r *contactmodel.Record) []string { return directoriesOfKind(r, "entry") },
		"link":       sliceOf(func(r *contactmodel.Record) []contactmodel.Resource { return r.Card.Links }, resourceURI),
		"contacturi": sliceOf(func(r *contactmodel.Record) []contactmodel.Resource { return r.Card.ContactURIs }, resourceURI),
		"lang":       sliceOf(func(r *contactmodel.Record) []contactmodel.LanguagePref { return r.Card.PreferredLanguages }, func(l contactmodel.LanguagePref) string { return l.Language }),

		// --- relations / members ---
		"related": sliceOf(func(r *contactmodel.Record) []contactmodel.Relation { return r.Card.RelatedTo }, relationKey),
		"member": func(r *contactmodel.Record) []string {
			var out []string
			for _, m := range r.Card.Members {
				if m != "" {
					out = append(out, m)
				}
			}
			return out
		},

		// --- passthrough ---
		"pt.vcard": func(r *contactmodel.Record) []string {
			var out []string
			for _, p := range r.Passthrough.VCard {
				out = append(out, jCardPropKey(p))
			}
			return out
		},
		"pt.jscontact": func(r *contactmodel.Record) []string {
			if len(r.Passthrough.JSContact) == 0 {
				return nil
			}
			var out []string
			for ptr, raw := range r.Passthrough.JSContact {
				out = append(out, ptr+"="+string(raw))
			}
			return out
		},
	}

	// Lock the comparer to the oracle: every row the table declares must have
	// an extractor here (a row we cannot compare is the ADR's "escalate, never
	// invent" signal), and every extractor must be backed by a row (no
	// invented concepts). This runs at package load, so any drift fails the
	// test binary at startup rather than silently skipping a concept.
	rows := correspondence.Load()
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		seen[row.ConceptID] = true
		if _, ok := byConcept[row.ConceptID]; !ok { // # pragma: no cover — drift is caught at dev time the moment the table changes; a healthy binary never reaches this
			panic(fmt.Sprintf("semanticequal: correspondence concept %q has no registered extractor — add one (ADR-0002: escalate, never invent)", row.ConceptID))
		}
	}
	for id := range byConcept {
		if !seen[id] { // # pragma: no cover — same drift guard, other direction
			panic(fmt.Sprintf("semanticequal: extractor registered for concept %q which has no correspondence-table row — remove it or extend the oracle deliberately", id))
		}
	}
}

// --- extractor combinators -------------------------------------------------

// scalar wraps a single-value reader into a value list (one entry, or none
// when the value is empty).
func scalar(read func(*contactmodel.Record) string) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		if v := read(r); v != "" {
			return []string{v}
		}
		return nil
	}
}

// timestampOf reads a *Timestamp and normalizes it under the ts_rfc3339
// transform (RFC3339 -> UTC instant).
func timestampOf(read func(*contactmodel.Record) *contactmodel.Timestamp) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		if ts := read(r); ts != nil && ts.UTC != "" {
			return []string{canonicalTimestamp(ts.UTC)}
		}
		return nil
	}
}

// nameComponent extracts the values of Name.Components entries of one kind
// (the [kind=X] predicate on the table's name.* rows).
func nameComponent(kind string) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		if r.Card.Name == nil {
			return nil
		}
		var out []string
		for _, c := range r.Card.Name.Components {
			if c.Kind == kind && c.Value != "" {
				out = append(out, c.Value)
			}
		}
		return out
	}
}

// namePhonetic implements the name.phonetic row: the neutral path names
// Name.PhoneticScript, and the row's v4_params (PHONETIC;SCRIPT) declare the
// PHONETIC parameter too, which the adapters read/write as the paired
// Name.PhoneticSystem field (the same "params populate sibling fields"
// convention the note row uses). Both are compared as one key.
func namePhonetic(r *contactmodel.Record) []string {
	if r.Card.Name == nil {
		return nil
	}
	n := r.Card.Name
	if n.PhoneticScript == "" && n.PhoneticSystem == "" {
		return nil
	}
	return []string{n.PhoneticScript + "|" + n.PhoneticSystem}
}

// sliceOf maps a neutral slice through a per-element key builder.
func sliceOf[T any](slice func(*contactmodel.Record) []T, key func(T) string) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		var out []string
		for _, e := range slice(r) {
			if k := key(e); k != "" {
				out = append(out, k)
			}
		}
		return out
	}
}

// titleKind extracts Titles entries of one Kind.
func titleKind(kind string) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		var out []string
		for _, t := range r.Card.Titles {
			if t.Kind == kind && t.Name != "" {
				out = append(out, t.Name)
			}
		}
		return out
	}
}

// addressField extracts one Address field across the whole Addresses slice.
func addressField(field func(contactmodel.Address) string) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		var out []string
		for _, a := range r.Card.Addresses {
			if v := field(a); v != "" {
				out = append(out, v)
			}
		}
		return out
	}
}

// anniversaryKind extracts the AnniversaryDate of one anniversary Kind,
// normalized under the date_partial transform.
func anniversaryKind(kind string) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		var out []string
		for _, a := range r.Card.Anniversaries {
			if a.Kind == kind {
				if k := anniversaryDateKey(a.Date); k != "" {
					out = append(out, k)
				}
			}
		}
		return out
	}
}

// anniversaryPlace extracts the Place (*Address) of one anniversary Kind,
// normalized like the place_text transform's neutral side.
func anniversaryPlace(kind string) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		var out []string
		for _, a := range r.Card.Anniversaries {
			if a.Kind == kind && a.Place != nil {
				out = append(out, addressKey(*a.Place))
			}
		}
		return out
	}
}

// personalInfoKind extracts PersonalInfo entries of one Kind, comparing the
// full element (value + the LEVEL/INDEX params the table declares).
func personalInfoKind(kind string) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		var out []string
		for _, p := range r.Card.PersonalInfo {
			if p.Kind == kind {
				if k := personalInfoKey(p); k != "" {
					out = append(out, k)
				}
			}
		}
		return out
	}
}

// mediaKind extracts Media entries of one Kind by URI.
func mediaKind(kind string) func(*contactmodel.Record) []string {
	return func(r *contactmodel.Record) []string {
		var out []string
		for _, m := range r.Card.Media {
			if m.Kind == kind && m.URI != "" {
				out = append(out, m.URI)
			}
		}
		return out
	}
}

// directoriesOfKind extracts Directories entries of one Kind by URI.
func directoriesOfKind(r *contactmodel.Record, kind string) []string {
	var out []string
	for _, d := range r.Card.Directories {
		if d.Kind == kind && d.URI != "" {
			out = append(out, d.URI)
		}
	}
	return out
}

// --- element key builders -------------------------------------------------

// orgKey is the org row's unit of comparison: the whole Organization element
// (name + units), per the org_units transform's "anchor + sibling Units[]"
// convention stated in the table.
func orgKey(o contactmodel.Organization) string {
	units := make([]string, 0, len(o.Units))
	for _, u := range o.Units {
		if u.Name != "" {
			units = append(units, u.Name)
		}
	}
	sort.Strings(units)
	return joinKeys(o.Name, strings.Join(units, ","))
}

// onlineServiceKey is the impp/social rows' unit of comparison: the whole
// OnlineService element. The impp row's v4_params and the social row's
// transform both declare SERVICE-TYPE/USERNAME (Service/User) as riding on
// the element, so they are part of the key alongside the URI/address the
// neutral path names.
func onlineServiceKey(o contactmodel.OnlineService) string {
	return joinKeys(o.Service, o.User, o.URI)
}

// addressKey is the adr row's unit of comparison: the whole Address element
// (all structural sibling fields the table's anchor-row convention covers —
// see the jscontact adapter's "whole-element copy" note).
func addressKey(a contactmodel.Address) string {
	var comps []string
	for _, c := range a.Components {
		if c.Value != "" {
			comps = append(comps, c.Kind+"="+c.Value)
		}
	}
	sort.Strings(comps)
	var contexts []string
	for _, c := range a.Contexts {
		contexts = append(contexts, c)
	}
	sort.Strings(contexts)
	return joinKeys(
		strings.Join(comps, ","),
		a.CountryCode,
		a.Coordinates,
		a.TimeZone,
		a.Full,
		strings.Join(contexts, ","),
		strconv.Itoa(intPtrVal(a.Pref)),
		strconv.FormatBool(boolPtrVal(a.IsOrdered)),
		a.DefaultSeparator,
		a.PhoneticSystem,
		a.PhoneticScript,
	)
}

// relationKey is the related row's unit of comparison: the whole Relation
// element (target + the relation tags, per the related transform).
func relationKey(rl contactmodel.Relation) string {
	rels := append([]string(nil), rl.Relations...)
	sort.Strings(rels)
	return joinKeys(rl.Target, strings.Join(rels, ","))
}

// personalInfoKey is the personalinfo transform's unit of comparison: the
// whole PersonalInfo element (value + LEVEL/INDEX params the table declares).
func personalInfoKey(p contactmodel.PersonalInfo) string {
	return joinKeys(p.Value, p.Level, strconv.Itoa(intPtrVal(p.ListAs)))
}

// noteKey is the note row's unit of comparison: the note text plus the
// AUTHOR/AUTHOR-NAME/CREATED params the row declares (which vCard4 reads/writes
// into Note.Author/Created).
func noteKey(n contactmodel.Note) string {
	var authorName, authorURI, created string
	if n.Author != nil {
		authorName, authorURI = n.Author.Name, n.Author.URI
	}
	if n.Created != nil {
		created = canonicalTimestamp(n.Created.UTC)
	}
	return joinKeys(n.Note, authorName, authorURI, created)
}

func resourceURI(r contactmodel.Resource) string { return r.URI }

// --- transform normalizers -------------------------------------------------

// canonicalTimestamp normalizes an RFC3339 instant to a canonical UTC form
// (the ts_rfc3339 transform). Unparseable strings pass through unchanged.
func canonicalTimestamp(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Format(time.RFC3339)
}

// partialDateKey renders a PartialDate canonically (the date_partial
// transform's neutral side).
func partialDateKey(p *contactmodel.PartialDate) string {
	if p == nil { // # pragma: no cover — anniversaryDateKey guards non-nil before calling; kept defensive
		return ""
	}
	return fmt.Sprintf("y=%d;m=%d;d=%d;cs=%s", intPtrVal(p.Year), intPtrVal(p.Month), intPtrVal(p.Day), p.CalendarScale)
}

// anniversaryDateKey normalizes an AnniversaryDate: a full RFC3339 timestamp
// and a partial date are distinct representational cases, each rendered
// canonically.
func anniversaryDateKey(d contactmodel.AnniversaryDate) string {
	if d.Partial != nil {
		if k := partialDateKey(d.Partial); k != "" {
			return "partial|" + k
		}
	}
	if d.Timestamp != nil && *d.Timestamp != "" {
		return "timestamp|" + canonicalTimestamp(*d.Timestamp)
	}
	return ""
}

// jCardPropKey canonicalizes a Passthrough.VCard entry (RFC 9555 vCardProps
// shape) so parameter-value spelling differences (single value vs a repeated
// single-element value, case) don't fail a round trip while real changes do.
func jCardPropKey(p contactmodel.JCardProp) string {
	params := make([]string, 0, len(p.Params))
	for k, v := range p.Params {
		params = append(params, strings.ToLower(k)+"="+paramValueKey(v))
	}
	sort.Strings(params)
	return joinKeys(strings.ToLower(p.Name), strings.ToLower(p.Type), strings.Join(params, ","), string(p.Value))
}

func paramValueKey(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	case []string:
		sort.Strings(vv)
		return strings.Join(vv, ",")
	case []any:
		var parts []string
		for _, item := range vv {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		sort.Strings(parts)
		return strings.Join(parts, ",")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v) // # pragma: no cover — json.Marshal of a param value cannot fail
		}
		return string(b)
	}
}
