package meerkatapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"mycorrhizal/meerkat"
)

// ErrTooLarge is returned when the account exceeds the caller's caps.
var ErrTooLarge = errors.New("meerkat: account is larger than the import supports")

// Me is the authenticated Meerkat user (GET /users/me).
type Me struct {
	ID       int64
	Username string
	Email    string
}

// Limits bound what one fetch may hold in memory. Zero disables a cap.
type Limits struct {
	MaxContacts int
	MaxEntities int
}

type wireUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// Me reads the authenticated user.
func (c *Client) Me(ctx context.Context) (Me, error) {
	var u wireUser
	if err := c.getJSON(ctx, "/api/v1/users/me", nil, &u); err != nil {
		return Me{}, err
	}
	return Me(u), nil
}

// CountContacts returns the number of contacts (archived included).
func (c *Client) CountContacts(ctx context.Context) (int, error) {
	var page wireContactPage
	q := url.Values{"limit": {"1"}, "include_archived": {"true"}}
	if err := c.getJSON(ctx, "/api/v1/contacts", q, &page); err != nil {
		return 0, err
	}
	return page.Total, nil
}

type wireContactPage struct {
	Contacts []wireContact `json:"contacts"`
	Total    int           `json:"total"`
}

type wireContact struct {
	ID                 int64           `json:"ID"`
	CreatedAt          *string         `json:"CreatedAt"`
	UpdatedAt          *string         `json:"UpdatedAt"`
	Firstname          *string         `json:"firstname"`
	Lastname           *string         `json:"lastname"`
	Nickname           *string         `json:"nickname"`
	Gender             *string         `json:"gender"`
	Email              *string         `json:"email"`
	Phone              *string         `json:"phone"`
	Birthday           *string         `json:"birthday"`
	Address            *string         `json:"address"`
	HowWeMet           *string         `json:"how_we_met"`
	FoodPreference     *string         `json:"food_preference"`
	WorkInformation    *string         `json:"work_information"`
	ContactInformation *string         `json:"contact_information"`
	Circles            json.RawMessage `json:"circles"`
	CustomFields       json.RawMessage `json:"custom_fields"`
	Emails             json.RawMessage `json:"emails"`
	Phones             json.RawMessage `json:"phones"`
	Addresses          json.RawMessage `json:"addresses"`
	URLs               json.RawMessage `json:"urls"`
	IMPPs              json.RawMessage `json:"impps"`
	Prefix             *string         `json:"prefix"`
	MiddleName         *string         `json:"middle_name"`
	Suffix             *string         `json:"suffix"`
	Organization       *string         `json:"organization"`
	Department         *string         `json:"department"`
	JobTitle           *string         `json:"job_title"`
	Role               *string         `json:"role"`
	Anniversary        *string         `json:"anniversary"`
	Photo              *string         `json:"photo"`
	Archived           bool            `json:"archived"`

	Relationships []wireRelationship `json:"relationships"`
	Notes         []wireNote         `json:"notes"`
	Activities    []wireActivity     `json:"activities"`
	Reminders     []wireReminder     `json:"reminders"`
}

type wireRelationship struct {
	ID               int64   `json:"ID"`
	Name             *string `json:"name"`
	Type             *string `json:"type"`
	Gender           *string `json:"gender"`
	Birthday         *string `json:"birthday"`
	ContactID        *int64  `json:"contact_id"`
	RelatedContactID *int64  `json:"related_contact_id"`
}

type wireNote struct {
	ID        int64   `json:"ID"`
	Content   *string `json:"content"`
	Date      *string `json:"date"`
	ContactID *int64  `json:"contact_id"`
}

type wireActivity struct {
	ID          int64   `json:"ID"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Location    *string `json:"location"`
	Date        *string `json:"date"`
}

type wireReminder struct {
	ID                    int64   `json:"ID"`
	Message               *string `json:"message"`
	ByMail                *bool   `json:"by_mail"`
	RemindAt              *string `json:"remind_at"`
	Recurrence            *string `json:"recurrence"`
	ReoccurFromCompletion *bool   `json:"reoccur_from_completion"`
	LastSent              *string `json:"last_sent"`
	ContactID             *int64  `json:"contact_id"`
	Completed             bool    `json:"completed"`
}

func rawStr(r json.RawMessage) *string {
	if len(r) == 0 || string(r) == "null" {
		return nil
	}
	s := string(r)
	return &s
}

func b2i(b bool) *int64 {
	v := int64(0)
	if b {
		v = 1
	}
	return &v
}

func pb2i(b *bool) *int64 {
	if b == nil {
		return nil
	}
	return b2i(*b)
}

// FetchSnapshot reads the authenticated user's whole account into a
// meerkat.Snapshot, so the file importer's mapper runs unchanged. One paged
// walk of /contacts with ?includes= preloads every relationship, note,
// activity and reminder, so the account is read in ceil(contacts/100)
// requests. progress receives (contacts fetched, contacts total).
//
// The API hides per-contact vcard_uid/etag, so UIDs are not preserved (the
// file path preserves them); re-runs still match on the stable source ids.
func (c *Client) FetchSnapshot(ctx context.Context, me Me, lim Limits, progress func(done, total int)) (*meerkat.Snapshot, error) {
	uid := me.ID
	snap := &meerkat.Snapshot{
		Users:           []meerkat.User{{ID: uid, Username: &me.Username, Email: &me.Email}},
		SourceUserID:    &uid,
		SourceUserCount: 1,
		PhotosFetchable: true,
	}
	seenActivity := map[int64]bool{}
	entities := 0

	for page := 1; page <= maxPages; page++ {
		q := url.Values{
			"limit":            {strconv.Itoa(pageSize)},
			"page":             {strconv.Itoa(page)},
			"include_archived": {"true"},
			"sort":             {"id"},
			"order":            {"asc"},
			"includes":         {"notes,activities,relationships,reminders"},
		}
		var resp wireContactPage
		if err := c.getJSON(ctx, "/api/v1/contacts", q, &resp); err != nil {
			return nil, err
		}
		if lim.MaxContacts > 0 && resp.Total > lim.MaxContacts {
			return nil, ErrTooLarge
		}
		for _, wc := range resp.Contacts {
			snap.Contacts = append(snap.Contacts, toContact(wc, uid))
			for _, r := range wc.Relationships {
				cid := r.ContactID
				if cid == nil {
					cid = &wc.ID
				}
				snap.Relationships = append(snap.Relationships, meerkat.Relationship{
					ID: r.ID, UserID: &uid, Name: r.Name, Type: r.Type, Gender: r.Gender,
					Birthday: r.Birthday, ContactID: cid, RelatedContact: r.RelatedContactID,
				})
			}
			for _, n := range wc.Notes {
				cid := n.ContactID
				if cid == nil {
					cid = &wc.ID
				}
				snap.Notes = append(snap.Notes, meerkat.Note{
					ID: n.ID, UserID: &uid, Content: n.Content, Date: n.Date, ContactID: cid,
				})
			}
			for _, a := range wc.Activities {
				if !seenActivity[a.ID] {
					seenActivity[a.ID] = true
					snap.Activities = append(snap.Activities, meerkat.Activity{
						ID: a.ID, UserID: &uid, Title: a.Title, Description: a.Description,
						Location: a.Location, Date: a.Date,
					})
				}
				snap.ActivityContacts = append(snap.ActivityContacts, meerkat.ActivityContact{ActivityID: a.ID, ContactID: wc.ID})
			}
			for _, r := range wc.Reminders {
				cid := r.ContactID
				if cid == nil {
					cid = &wc.ID
				}
				snap.Reminders = append(snap.Reminders, meerkat.Reminder{
					ID: r.ID, UserID: &uid, Message: r.Message, ByMail: pb2i(r.ByMail),
					RemindAt: r.RemindAt, Recurrence: r.Recurrence,
					ReoccurFromCompletion: pb2i(r.ReoccurFromCompletion), LastSent: r.LastSent,
					ContactID: cid, Completed: b2i(r.Completed),
				})
			}
		}
		entities = len(snap.Contacts) + len(snap.Relationships) + len(snap.Notes) +
			len(snap.Activities) + len(snap.Reminders)
		if lim.MaxEntities > 0 && entities > lim.MaxEntities {
			return nil, fmt.Errorf("%w", ErrTooLarge)
		}
		if progress != nil {
			progress(len(snap.Contacts), resp.Total)
		}
		if len(resp.Contacts) == 0 || len(snap.Contacts) >= resp.Total {
			return snap, nil
		}
	}
	return snap, nil
}

func toContact(w wireContact, uid int64) meerkat.Contact {
	var archived int64
	if w.Archived {
		archived = 1
	}
	return meerkat.Contact{
		ID: w.ID, UserID: &uid, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
		Firstname: w.Firstname, Lastname: w.Lastname, Nickname: w.Nickname, Gender: w.Gender,
		Email: w.Email, Phone: w.Phone, Birthday: w.Birthday, Photo: w.Photo, Address: w.Address,
		HowWeMet: w.HowWeMet, FoodPref: w.FoodPreference, WorkInfo: w.WorkInformation,
		ContactInfo: w.ContactInformation, CirclesJSON: rawStr(w.Circles), CustomFields: rawStr(w.CustomFields),
		Archived: &archived, EmailsJSON: rawStr(w.Emails), PhonesJSON: rawStr(w.Phones),
		AddressesJSON: rawStr(w.Addresses), URLsJSON: rawStr(w.URLs), IMPPsJSON: rawStr(w.IMPPs),
		Prefix: w.Prefix, MiddleName: w.MiddleName, Suffix: w.Suffix, Organization: w.Organization,
		Department: w.Department, JobTitle: w.JobTitle, Role: w.Role, Anniversary: w.Anniversary,
	}
}
