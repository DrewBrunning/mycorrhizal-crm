package soak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mycorrhizal/database"

	"github.com/go-co-op/gocron"

	"gorm.io/gorm"
)

// schedulerCheck asserts the scheduler is alive and not stuck after the run:
// every registered job has a next run in the future (a job whose NextRun is
// zero or in the past was dropped or wedged — the missed-tick failure the
// per-function catch-up tests cannot see end to end), and none reports a
// scheduler-level error. Job *outcomes* are judged separately from the
// job_runs_total{result="failure"} counter.
func schedulerCheck(jobs []*gocron.Job, now time.Time) Check {
	c := Check{Name: "scheduler"}
	if len(jobs) == 0 {
		c.Detail = "no scheduled jobs registered"
		return c
	}
	var bad []string
	for _, j := range jobs {
		name := strings.Join(j.Tags(), ",")
		if name == "" {
			name = "(untagged)"
		}
		if err := j.Error(); err != nil {
			bad = append(bad, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		// A small grace: a job due this very second may have just fired and
		// not yet rescheduled.
		if next := j.NextRun(); next.IsZero() || next.Before(now.Add(-2*time.Second)) {
			bad = append(bad, fmt.Sprintf("%s: next run %v is not in the future", name, next))
		}
	}
	if len(bad) > 0 {
		c.Detail = strings.Join(bad, "; ")
		return c
	}
	c.OK = true
	c.Detail = fmt.Sprintf("%d jobs registered, all with a future next run and no error", len(jobs))
	return c
}

// ftsTables pairs each derived FTS index with the base table it must mirror
// one-to-one over live (not soft-deleted) rows.
var ftsTables = []struct{ fts, base string }{
	{"contacts_fts", "contacts"},
	{"notes_fts", "notes"},
	{"activities_fts", "activities"},
}

// ftsCheck asserts each FTS index holds exactly one row per live base row
// after the churn: the issue's "FTS row count != contacts count at end".
func ftsCheck(ctx context.Context, db *gorm.DB) Check {
	c := Check{Name: "FTS index vs base tables"}
	var drift []string
	var summary []string
	for _, p := range ftsTables {
		var ftsN, baseN int64
		// #nosec G201 -- table names are constants from ftsTables above.
		if err := db.WithContext(ctx).Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", p.fts)).Scan(&ftsN).Error; err != nil {
			c.Detail = fmt.Sprintf("count %s: %v", p.fts, err)
			return c
		}
		// #nosec G201 -- table names are constants from ftsTables above.
		if err := db.WithContext(ctx).Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL", p.base)).Scan(&baseN).Error; err != nil {
			c.Detail = fmt.Sprintf("count %s: %v", p.base, err)
			return c
		}
		summary = append(summary, fmt.Sprintf("%s=%d/%d", p.fts, ftsN, baseN))
		if ftsN != baseN {
			drift = append(drift, fmt.Sprintf("%s has %d rows for %d live %s", p.fts, ftsN, baseN, p.base))
		}
	}
	if len(drift) > 0 {
		c.Detail = strings.Join(drift, "; ")
		return c
	}
	c.OK = true
	c.Detail = strings.Join(summary, " ") + " (fts/live)"
	return c
}

// maxProbes bounds how many live / gone names the search probe checks per
// side.
const maxProbes = 5

// searchProbe is the content half of the FTS-drift check: after the churn, a
// contact that was created and never touched must still be findable by name,
// and one that was deleted must not be. Counts can match while the indexed
// *content* has drifted; this catches that.
func (w *Workload) searchProbe(ctx context.Context) Check {
	c := Check{Name: "search freshness probe"}
	var missing, stale []string
	var liveN, goneN int
	for _, u := range w.users {
		u.idsMu.Lock()
		var live []string
		for _, n := range u.pristine {
			if len(live) < maxProbes {
				live = append(live, n)
			}
		}
		gone := append([]string(nil), u.gone...)
		u.idsMu.Unlock()
		if len(gone) > maxProbes {
			gone = gone[len(gone)-maxProbes:]
		}
		for _, name := range live {
			liveN++
			if !w.searchHas(ctx, u, name) {
				missing = append(missing, name)
			}
		}
		for _, name := range gone {
			goneN++
			if w.searchHas(ctx, u, name) {
				stale = append(stale, name)
			}
		}
	}
	if len(missing) > 0 || len(stale) > 0 {
		c.Detail = fmt.Sprintf("live contacts not found by search: %v; deleted contacts still found: %v", missing, stale)
		return c
	}
	c.OK = true
	c.Detail = fmt.Sprintf("%d live names found, %d deleted names absent", liveN, goneN)
	return c
}

func (w *Workload) searchHas(ctx context.Context, u *user, name string) bool {
	b, st, err := w.do(ctx, u.client, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(name)+"&limit=50", nil)
	if err != nil || st != http.StatusOK {
		return false
	}
	return searchResultHasFirstname(b, name)
}

// searchResultHasFirstname reports whether a /search response lists a contact
// whose firstname is name. It parses the body rather than substring-matching
// it: the response echoes the query, so a substring test is true for every
// name, found or not.
func searchResultHasFirstname(body []byte, name string) bool {
	var res struct {
		Contacts []struct {
			Firstname string `json:"firstname"`
		} `json:"contacts"`
	}
	if json.Unmarshal(body, &res) != nil {
		return false
	}
	for _, c := range res.Contacts {
		if strings.EqualFold(c.Firstname, name) {
			return true
		}
	}
	return false
}

// openExternalDB opens an already-migrated database file for the end-of-run
// checks of an external target.
func openExternalDB(path string) (*gorm.DB, error) {
	db, err := database.OpenMigratedFile(path)
	if err != nil {
		return nil, fmt.Errorf("open %s for the end-of-run checks: %w", path, err)
	}
	return db, nil
}

func closeGorm(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
