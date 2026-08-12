package main

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/routes"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// loadOpenAPIDoc loads backend/openapi.yaml and validates it, failing the test
// on any structural problem. Shared by the spec test and the drift test so
// the drift test never operates on a broken document.
func loadOpenAPIDoc(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false

	doc, err := loader.LoadFromFile("openapi.yaml")
	if err != nil {
		t.Fatalf("failed to load openapi.yaml: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi.yaml failed validation: %v", err)
	}
	return doc
}

// TestOpenAPISpecValidates loads and validates backend/openapi.yaml (WP-71
// item 6, docs/fork-plan/50-integration-and-rebrand.md): the spec must be
// well-formed OpenAPI 3.0 and internally consistent (every $ref resolves,
// every schema/path is structurally valid). This is a Go-based check
// (github.com/getkin/kin-openapi, added as a test-only dependency) rather
// than a Node/swagger-cli install, per this WP's own tooling note (no local
// Node toolchain is assumed available; Go/Docker is this repo's confirmed
// toolchain — docs/fork-plan/70-environment.md).
func TestOpenAPISpecValidates(t *testing.T) {
	doc := loadOpenAPIDoc(t)

	// Spot-check the schemas this spec documents actually exist, so a future
	// edit that silently deletes a schema (rather than just drifting its
	// fields) is caught here too. The full route-vs-spec correspondence is
	// covered by TestOpenAPIRouteCoverage (drift), which enumerates the live
	// router — this list guards the schema half of the contract.
	wantSchemas := []string{
		// WP-71 nested-Card contact API.
		"ContactSummary", "ContactSummaryWithRelations", "ContactRecordInput",
		"ContactRecordResponse", "Card", "CRMEnvelope", "Passthrough",
		"JCardProp", "Timestamp", "PartialDate", "AnniversaryDate",
		"Anniversary", "NameComponent", "Name", "Nickname", "OrgUnit",
		"Organization", "Title", "Email", "Phone", "OnlineService",
		"AddressComponent", "Address", "GrammaticalGender", "Pronouns",
		"SpeakToAs", "PersonalInfo", "Author", "CardNote", "Resource",
		"LanguagePref", "Relation",
		// WP-73b import + contact-subscription surface.
		"ColumnMapping", "ImportUploadResponse", "ImportPreviewRequest",
		"DuplicateMatch", "ImportRowPreview", "ImportPreviewResponse",
		"RowImportAction", "ImportConfirmRequest", "ImportResult",
		"ContactSubscriptionInput", "ContactSubscriptionResponse",
		// T8: everything the drift test now covers.
		"Error", "MessageResponse", "LogoutResponse", "HealthResponse",
		"UserRegistrationInput", "LoginInput", "PasswordResetRequestInput",
		"PasswordResetConfirmInput", "ChangePasswordInput",
		"UpdateLanguageInput", "UpdateDateFormatInput",
		"EnabledContactFieldsInput", "PasswordStrength", "OIDCConfigResponse",
		"AdminUserResponse", "CurrentUserResponse", "AdminUsersListResponse",
		"AdminUserUpdateInput", "AdminUserCreateInput",
		"ContactFlat", "ContactResponse", "ContactEmail", "ContactPhone",
		"ContactURL", "ContactIMPP", "ContactAddress", "Birthday",
		"ContactMergeRequest", "ContactMergePreviewResponse",
		"ContactMergeResolution", "ContactMergeFieldConflict",
		"ContactMergeAssociationCounts",
		"Note", "NoteInput", "Activity", "ActivityInput",
		"Reminder", "ReminderCompleteResponse", "ReminderCompletion",
		"Circle", "CircleInput", "CircleMember", "CircleMemberInput",
		"Household", "HouseholdInput", "HouseholdMember",
		"HouseholdMemberInput",
		"LinkFieldType", "LinkFieldTypeInput", "LinkFieldTypeReorderInput",
		"Tag", "TagInput", "ContactTag", "ContactTagInput",
		"RelationshipEdge", "RelationshipEdgeInput", "ThinContactInput",
		"LifeEvent", "LifeEventInput",
		"Preference", "PreferenceInput",
		"ConversationAgenda", "ConversationAgendaInput", "ConversationAgendaDiscussInput",
		"Gift", "GiftInput",
		"FieldDefinition", "FieldDefinitionInput", "FieldConstraints",
		"FieldValue", "FieldValueInput", "ContactFieldValuesInput",
		"CalendarSubscriptionInput", "CalendarSubscriptionResponse",
		"ApiTokenInput", "ApiTokenResponse", "ApiTokenCreateResponse",
		"WebhookInput", "WebhookResponse", "WebhookCreateResponse",
		"WebhookDeliveryResponse",
		"GraphResponse", "GraphNode", "GraphEdge",
		// M3 dashboard composite.
		"DashboardResponse", "DashboardReminder",
		// M4 contact-detail composite.
		"ContactDetailResponse", "ContactDetailUser", "ContactDetailLifeEvent",
		"ContactDetailImmich",
	}
	for _, name := range wantSchemas {
		if _, ok := doc.Components.Schemas[name]; !ok {
			t.Errorf("openapi.yaml is missing expected schema %q", name)
		}
	}

	// The two OIDC conditional routes are documented but not registered by
	// the drift test (OIDC is off in its config), so assert their presence
	// explicitly here rather than relying on route coverage.
	oidcPaths := map[string]bool{
		"/auth/oidc/login":    true,
		"/auth/oidc/callback": true,
	}
	for p := range oidcPaths {
		if doc.Paths.Find(p) == nil {
			t.Errorf("openapi.yaml is missing expected conditional path %q", p)
		}
	}

	// GET /contacts must document the cursor-pagination + change-feed query
	// mechanics T17 requires (cursor/since/limit/order plus the surviving
	// search/archive/circle/includes filters), the sort= control T73 adds,
	// and must NOT document the removed page/fields params (Gap 3: removed,
	// not just undocumented-but-still-there).
	contactsGet := doc.Paths.Find("/contacts").Get
	wantParams := []string{"cursor", "since", "limit", "order", "sort", "search", "include_archived", "archived", "circle", "includes"}
	gotParams := map[string]bool{}
	for _, p := range contactsGet.Parameters {
		gotParams[p.Value.Name] = true
	}
	for _, name := range wantParams {
		if !gotParams[name] {
			t.Errorf("GET /contacts is missing documented query param %q", name)
		}
	}
	for _, gone := range []string{"fields", "page"} {
		if gotParams[gone] {
			t.Errorf("GET /contacts still documents %q, which T17 removed", gone)
		}
	}
	// T73: sort= is documented with exactly the two accepted tokens.
	if sortParam := contactsGet.Parameters.GetByInAndName("query", "sort"); sortParam == nil {
		t.Error("GET /contacts must document the T73 sort query param")
	} else {
		if enum := sortParam.Schema.Value.Enum; len(enum) != 2 ||
			enum[0] != "updated_at" || enum[1] != "name" {
			t.Errorf("sort param enum = %v, want [updated_at name]", enum)
		}
	}

	// POST/PUT /contacts must reference ContactRecordInput as their request
	// body (the nested shape), not a flat DTO.
	postBody := doc.Paths.Find("/contacts").Post.RequestBody.Value.Content.Get("application/json").Schema
	if postBody.Ref == "" || postBody.Ref != "#/components/schemas/ContactRecordInput" {
		t.Errorf("POST /contacts request body ref = %q, want #/components/schemas/ContactRecordInput", postBody.Ref)
	}
}

// ginToOpenAPIPath converts a Gin route pattern to the OpenAPI template form
// (":id" -> "{id}"). Gin's wildcard routes ("*path", used only by the
// CardDAV group) have no OpenAPI analog and are never expected here.
func ginToOpenAPIPath(ginPath string) string {
	re := regexp.MustCompile(`:([a-zA-Z0-9_]+)`)
	return re.ReplaceAllString(ginPath, "{$1}")
}

// specPathFromRegistered maps a registered full router path to its spec path
// key. The spec's single server is "/api/v1", so registered "/api/v1/contacts"
// corresponds to spec path "/contacts"; the unversioned /health has no prefix.
func specPathFromRegistered(fullPath string) string {
	const v1Prefix = "/api/v1"
	if len(fullPath) >= len(v1Prefix) && fullPath[:len(v1Prefix)] == v1Prefix {
		return fullPath[len(v1Prefix):]
	}
	return fullPath
}

// openAPITestConfig builds a config with OIDC and CardDAV disabled — the
// smallest configuration whose registered route surface is exactly the
// /api/v1 REST API plus /health. OIDC login/callback and all CardDAV routes
// are conditional on server config and are deliberately absent here.
func openAPITestConfig() *config.Config {
	return &config.Config{
		DBPath:           ":memory:",
		JWTSecretKey:     "test-secret-key-that-is-long-enough-32",
		ProfilePhotoDir:  "/tmp/test-photos",
		FrontendURL:      "http://localhost:5173",
		Port:             "7300",
		ReminderTime:     "12:00",
		ReminderTimezone: "UTC",
		JWTExpiryHours:   96,
		ReadTimeout:      15,
		WriteTimeout:     15,
		IdleTimeout:      60,
	}
}

// operationForMethod returns the PathItem's operation for the given HTTP
// method, or nil if the path item has none.
func operationForMethod(pi *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case "GET":
		return pi.Get
	case "POST":
		return pi.Post
	case "PUT":
		return pi.Put
	case "PATCH":
		return pi.Patch
	case "DELETE":
		return pi.Delete
	case "HEAD":
		return pi.Head
	case "OPTIONS":
		return pi.Options
	}
	return nil
}

// isConditionallyRegisteredPath reports whether a documented path is only
// present in the router under a server configuration the drift test does not
// exercise (OIDC enabled). The spec documents these with an explicit note;
// the drift test must not flag them as dead documentation.
func isConditionallyRegisteredPath(path string) bool {
	switch path {
	case "/auth/oidc/login", "/auth/oidc/callback":
		return true
	}
	return false
}

// TestOpenAPIRouteCoverage (T8's drift test) enumerates the live Gin router
// after RegisterRoutes and asserts a one-to-one correspondence with the spec:
// every registered route has a spec entry (forward), and every documented
// route is actually registered (reverse, minus the OIDC-conditional pair).
// Routes are enumerated from router.Routes() rather than parsed from
// routes/routes.go source so the test cannot go stale — adding a route
// without documenting it (or documenting one without registering it) fails
// here.
func TestOpenAPIRouteCoverage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	routes.RegisterRoutes(router, openAPITestConfig(), db, nil)

	doc := loadOpenAPIDoc(t)

	// Enumerate the registered surface. CardDAV is off in this config, so the
	// only deliberate no-spec routes (.well-known/carddav, /carddav/*) never
	// appear. The Gin router returns one RouteInfo per (method, path).
	registered := map[string]bool{}
	for _, r := range router.Routes() {
		if isCardDAVRoute(r.Path) {
			continue
		}
		registered[r.Method+" "+ginToOpenAPIPath(specPathFromRegistered(r.Path))] = true
	}

	// Forward: a registered route with no spec entry is the failure this test
	// exists to prevent.
	for key := range registered {
		parts := splitRouteKey(key)
		pi := doc.Paths.Find(parts.path)
		if pi == nil || operationForMethod(pi, parts.method) == nil {
			t.Errorf("route %s %s is registered in routes/routes.go but has no entry in openapi.yaml", parts.method, parts.path)
		}
	}

	// Reverse: a documented route that is no longer registered is dead
	// documentation (T8's "ideally vice versa").
	for path, pi := range doc.Paths.Map() {
		if isConditionallyRegisteredPath(path) {
			continue
		}
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			if operationForMethod(pi, method) == nil {
				continue
			}
			if !registered[method+" "+path] {
				t.Errorf("openapi.yaml documents %s %s but the route is not registered in routes/routes.go", method, path)
			}
		}
	}

	// Sanity: the router surface is non-trivial, so a broken enumeration
	// (e.g. an empty Routes()) fails loudly instead of vacuously passing.
	require.GreaterOrEqual(t, len(registered), 90,
		"unexpectedly small registered route surface — the drift test may be broken")
}

// isCardDAVRoute reports whether a route belongs to the CardDAV WebDAV/XML
// surface, which this JSON spec deliberately does not document (it is a
// separate protocol, not REST, and is out of T8's scope — see openapi.yaml's
// info description). CardDAV is also disabled in the drift test's config, so
// these routes never register here anyway; the guard is belt-and-suspenders
// in case the config changes.
func isCardDAVRoute(path string) bool {
	return path == "/.well-known/carddav" || path == "/carddav/*path"
}

type routeKeyParts struct {
	method string
	path   string
}

func splitRouteKey(key string) routeKeyParts {
	method, path, _ := strings.Cut(key, " ")
	return routeKeyParts{method: method, path: path}
}
