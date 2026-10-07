// Package auditwire is the explicit opt-in a test harness uses to arm audit
// recording on a database it opened (issue #1493).
//
// Production wires exactly one recorder (models.NewAuditRecorder in
// embedded.Start) and a *gorm.DB without one records nothing. Tests used to be
// armed implicitly — the models layer branched on testing.Testing() and
// installed a recorder on any unarmed DB — which put a test-only code path in
// production code. Instead, internal/dbtest installs this marker on every DB it
// hands out; the models layer installs a recorder on first use only when the
// marker is present. The marker lives in this leaf package (not dbtest, not
// models) because models' own internal tests import dbtest, so dbtest cannot
// import models.
package auditwire

import "gorm.io/gorm"

// PluginName is the key the marker is stored under in gorm.Config.Plugins.
const PluginName = "mycorrhizal:audit_default"

// Default marks a database as one whose audited writes must be recorded. It
// implements gorm.Plugin only so it can live in gorm.Config.Plugins (via
// db.Use, which also hands it the root handle).
type Default struct {
	// Async selects the production-shaped recorder: events persist on their
	// own goroutine and session, outside the audited write's transaction.
	// The zero value is the synchronous recorder, which joins the audited
	// write's transaction so a test can read audit rows with no Flush.
	Async bool

	root *gorm.DB
}

// Name satisfies gorm.Plugin.
func (*Default) Name() string { return PluginName }

// Initialize satisfies gorm.Plugin; it keeps the root handle db.Use passes so
// an async recorder can be bound to it rather than to a hook's transaction.
func (d *Default) Initialize(db *gorm.DB) error {
	d.root = db
	return nil
}

// Root returns the database the marker was installed on.
func (d *Default) Root() *gorm.DB { return d.root }

// For returns the marker installed on db, or nil.
func For(db *gorm.DB) *Default {
	if db == nil || db.Config == nil {
		return nil
	}
	d, _ := db.Plugins[PluginName].(*Default)
	return d
}
