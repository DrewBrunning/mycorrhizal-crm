// Package b provides a package-level function that shares a name with a
// gorm.DB finisher method, so a call to it through a selector expression
// (b.Save()) exercises isGormDBMethod's "no receiver at all" branch: it
// must not be mistaken for a discarded gorm result.
package b

type Thing struct{}

func Save() *Thing { return nil }
