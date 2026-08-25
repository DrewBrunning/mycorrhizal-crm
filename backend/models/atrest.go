package models

// This blank import registers the at-rest encryption GORM serializers
// ("encrypted", "encryptedjson") with gorm.io/gorm/schema before any model is
// parsed. GORM resolves a field's `serializer:` tag by name at schema-parse
// time (the first time a model is used by the DB), so the registering package
// must be linked and its init() must have run before then. models is imported
// by every caller that touches the schema — the app, all controllers, all
// tests — so importing atrest here guarantees the serializers are always
// registered. See backend/atrest/serializers.go.
import _ "mycorrhizal/atrest"
