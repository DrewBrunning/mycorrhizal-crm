package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mycorrhizal/buildinfo"
	"mycorrhizal/config"
	"mycorrhizal/internal/clock"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

// MCP server (issue #176, ADR 0032): a streamable-HTTP adapter over the
// existing REST read surface. Every tool is a thin wrapper that calls the
// same service / builder function its REST endpoint calls, as the user the
// AuthMiddleware already resolved — so ownership scoping and sensitivity
// filtering cannot drift between the two surfaces.

// MCP tool limits (ADR 0032 §6). Over-max values are clamped, never rejected.
const (
	mcpSearchDefaultLimit   = 20
	mcpSearchMaxLimit       = 50
	mcpTimelineDefaultLimit = 25
	mcpTimelineMaxLimit     = 100
	mcpCadenceDefaultLimit  = 25
	mcpCadenceMaxLimit      = 100
	// mcpMaxOffset bounds offset pagination: the timeline composer is
	// cursor-based, so an offset is served by composing offset+limit rows
	// and slicing.
	mcpMaxOffset = 1000
	// mcpMaxTimelineSpan is the widest since..until window list_timeline
	// serves; a wider (or absent) window is clamped to it.
	mcpMaxTimelineSpan = 366 * 24 * time.Hour
)

// mcpClampLimit applies a tool's default and maximum to a requested limit.
func mcpClampLimit(requested, def, max int) int {
	if requested < 1 {
		return def
	}
	if requested > max {
		return max
	}
	return requested
}

// mcpClampOffset clamps a requested offset into [0, mcpMaxOffset].
func mcpClampOffset(requested int) int {
	if requested < 0 {
		return 0
	}
	if requested > mcpMaxOffset {
		return mcpMaxOffset
	}
	return requested
}

// Tool inputs. The struct tags are the tool's published JSON schema.

type mcpSearchContactsInput struct {
	Query            string `json:"query" jsonschema:"search term (required, at least 2 characters)"`
	Limit            int    `json:"limit,omitempty" jsonschema:"results per section; default 20, max 50 (larger values are clamped)"`
	Offset           int    `json:"offset,omitempty" jsonschema:"results to skip per section; default 0"`
	IncludeSensitive bool   `json:"include_sensitive,omitempty" jsonschema:"include private/secret data; default false. When false, a contact is not returned if its only match is a private/secret address"`
}

type mcpGetContactInput struct {
	ID               int  `json:"id" jsonschema:"numeric contact id (required)"`
	IncludeSensitive bool `json:"include_sensitive,omitempty" jsonschema:"include private/secret data; default false"`
}

type mcpListTimelineInput struct {
	ContactID        int    `json:"contact_id" jsonschema:"numeric contact id (required)"`
	Limit            int    `json:"limit,omitempty" jsonschema:"items per page; default 25, max 100 (larger values are clamped)"`
	Offset           int    `json:"offset,omitempty" jsonschema:"items to skip; default 0"`
	Since            string `json:"since,omitempty" jsonschema:"inclusive ISO-8601 start date, e.g. 2026-01-31"`
	Until            string `json:"until,omitempty" jsonschema:"inclusive ISO-8601 end date; the since..until span is capped at 366 days (a wider or absent window is clamped to the 366 days ending at until, default today)"`
	IncludeSensitive bool   `json:"include_sensitive,omitempty" jsonschema:"include private/secret data; default false"`
}

type mcpRunCadenceReportInput struct {
	Limit            int  `json:"limit,omitempty" jsonschema:"results per page; default 25, max 100 (larger values are clamped)"`
	Offset           int  `json:"offset,omitempty" jsonschema:"results to skip; default 0"`
	IncludeSensitive bool `json:"include_sensitive,omitempty" jsonschema:"accepted for interface uniformity; this report carries no sensitivity-tiered fields"`
}

// mcpToolError is a tool-level failure: reported to the client as an
// IsError tool result (so the assistant can read and react to it) rather than
// a protocol error.
func mcpToolError(format string, args ...any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}, nil, nil
}

// mcpJSONResult marshals v — the same value the matching REST handler hands
// to c.JSON — as both text content and structured content.
func mcpJSONResult(v any) (*mcp.CallToolResult, any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err // # pragma: no cover — REST response types always marshal
	}
	var structured map[string]any
	if err := json.Unmarshal(raw, &structured); err != nil {
		return nil, nil, err // # pragma: no cover — every wrapped REST body is a JSON object
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(raw)}},
		StructuredContent: structured,
	}, nil, nil
}

// mcpParseDate parses an ISO-8601 date (YYYY-MM-DD) or full RFC 3339
// timestamp in loc.
func mcpParseDate(raw string, loc *time.Location) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", raw, loc); err == nil {
		return t, true, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, fmt.Errorf("%q is not an ISO-8601 date (expected YYYY-MM-DD)", raw)
}

// mcpTimelineWindow resolves list_timeline's since/until into the composer's
// [cutoff, notAfter] bounds, clamping the span to mcpMaxTimelineSpan.
func mcpTimelineWindow(since, until string, now time.Time) (cutoff, notAfter time.Time, err error) {
	loc := now.Location()
	s, hasSince, err := mcpParseDate(since, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("since: %w", err)
	}
	u, hasUntil, err := mcpParseDate(until, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("until: %w", err)
	}

	switch {
	case hasUntil:
		// A bare date is inclusive of that whole day.
		if len(strings.TrimSpace(until)) == len("2006-01-02") {
			u = u.AddDate(0, 0, 1).Add(-time.Nanosecond)
		}
		notAfter = u
	case hasSince:
		notAfter = s.Add(mcpMaxTimelineSpan)
	default:
		notAfter = now
	}
	if notAfter.Before(s) && hasSince {
		return time.Time{}, time.Time{}, errors.New("until must not be before since")
	}

	earliest := notAfter.Add(-mcpMaxTimelineSpan)
	cutoff = earliest
	if hasSince && s.After(earliest) {
		cutoff = s
	}
	return cutoff, notAfter, nil
}

// newMCPServer builds the tool set for one request, bound to the already
// authenticated user. A fresh server per request (stateless transport) keeps
// the user identity out of any shared state.
func newMCPServer(db *gorm.DB, userID uint, cfg config.Config, clk clock.Clock) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "mycorrhizal-crm", Version: buildinfo.Get().Version}, nil)
	now := func() time.Time { return clk.Now().In(cfg.GetReminderLocation()) }

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_contacts",
		Description: "Full-text search across the user's contacts, notes and interactions (same as GET /search). Results are grouped into contacts, notes and activities.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in mcpSearchContactsInput) (*mcp.CallToolResult, any, error) {
		term := strings.TrimSpace(in.Query)
		if term == "" {
			return mcpToolError("query (a search term) is required")
		}
		if len([]rune(term)) > services.MaxSearchTermLen {
			return mcpToolError("query must be at most %d characters", services.MaxSearchTermLen)
		}
		limit := mcpClampLimit(in.Limit, mcpSearchDefaultLimit, mcpSearchMaxLimit)
		result, err := services.SearchPageScoped(db, userID, term, limit, mcpClampOffset(in.Offset), nil, in.IncludeSensitive)
		if err != nil {
			return mcpToolError("search failed")
		}
		return mcpJSONResult(result)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_contact",
		Description: "Everything known about one contact in a single call (same as GET /contacts/:id/detail): the record plus notes, activities, reminders, relationships, life events, gifts and more.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in mcpGetContactInput) (*mcp.CallToolResult, any, error) {
		contact, ok := mcpOwnedContact(db, userID, in.ID)
		if !ok {
			return mcpToolError("contact %d not found", in.ID)
		}
		excluded := []string{models.RelationshipSensitivityPrivate, models.RelationshipSensitivitySecret}
		if in.IncludeSensitive {
			excluded = nil
		}
		detail, err := buildContactDetailScoped(db, userID, contact, cfg, in.IncludeSensitive, excluded)
		if err != nil {
			return mcpToolError("failed to compose contact detail")
		}
		// buildContactDetailScoped builds the owner-facing record, which keeps
		// postal addresses (the REST detail needs them so a full-overwrite
		// edit cannot drop them). MCP is a copy out to the caller's model, so
		// the default include_sensitive=false must withhold an above-normal
		// address and its geo: coordinate just as the export/sync surfaces do,
		// and opt in to them with the same flag (issue #1433).
		if !in.IncludeSensitive {
			detail.Contact.Card.Addresses = models.FilterSensitiveAddresses(detail.Contact.Card.Addresses)
		}
		return mcpJSONResult(detail)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_timeline",
		Description: "A page of one contact's merged timeline — notes, activities, completions, external activities, gifts and life events — newest first (same as GET /contacts/:id/timeline).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in mcpListTimelineInput) (*mcp.CallToolResult, any, error) {
		contact, ok := mcpOwnedContact(db, userID, in.ContactID)
		if !ok {
			return mcpToolError("contact %d not found", in.ContactID)
		}
		current := now()
		cutoff, notAfter, err := mcpTimelineWindow(in.Since, in.Until, current)
		if err != nil {
			return mcpToolError("%v", err)
		}
		limit := mcpClampLimit(in.Limit, mcpTimelineDefaultLimit, mcpTimelineMaxLimit)
		offset := mcpClampOffset(in.Offset)

		contactID := contact.ID
		items, _, err := services.ComposeTimeline(db, userID, services.TimelineQuery{
			ContactID:         &contactID,
			Limit:             offset + limit,
			Desc:              true,
			Types:             models.TimelineTypes,
			Cutoff:            &cutoff,
			NotAfter:          &notAfter,
			FilterSensitivity: !in.IncludeSensitive,
			Now:               current,
		})
		if err != nil {
			return mcpToolError("failed to retrieve contact timeline")
		}
		if offset >= len(items) {
			items = []models.TimelineItem{}
		} else {
			items = items[offset:]
		}
		return mcpJSONResult(gin.H{"items": items, "limit": limit, "offset": offset})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_cadence_report",
		Description: "Contacts the user is overdue to reach out to, most overdue first (same as GET /cadence-policies/overdue). \"Overdue\" is evaluated as of now.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in mcpRunCadenceReportInput) (*mcp.CallToolResult, any, error) {
		overdue, err := services.ListOverdueCadences(db, userID, now())
		if err != nil {
			return mcpToolError("failed to compute overdue cadences")
		}
		limit := mcpClampLimit(in.Limit, mcpCadenceDefaultLimit, mcpCadenceMaxLimit)
		offset := mcpClampOffset(in.Offset)
		page := []services.OverdueCadence{}
		if offset < len(overdue) {
			end := offset + limit
			if end > len(overdue) {
				end = len(overdue)
			}
			page = overdue[offset:end]
		}
		return mcpJSONResult(gin.H{"overdue": page, "total": len(overdue), "limit": limit, "offset": offset})
	})

	return server
}

// mcpOwnedContact loads a contact only if it belongs to userID; an
// unowned, deleted or non-positive id is indistinguishable from "not found".
func mcpOwnedContact(db *gorm.DB, userID uint, id int) (*models.Contact, bool) {
	if id < 1 {
		return nil, false
	}
	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		return nil, false
	}
	return &contact, true
}

// MCPHandler serves the MCP streamable-HTTP endpoint (POST /mcp). It must be
// mounted behind AuthMiddleware: the tools run as the "userID" it sets, with
// the "db" and "cfg" context values every other handler reads.
func MCPHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		db := c.MustGet("db").(*gorm.DB)
		userID, ok := currentUserID(c)
		if !ok {
			return
		}
		cfg := currentConfig(c)
		clk := clock.FromContext(c)
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return newMCPServer(db, userID, cfg, clk)
		}, &mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			// Disable the go-sdk's DNS-rebinding guard (streamable.go: the
			// 403 when the connection's local address is loopback but Host is
			// not localhost). It protects a localhost-only MCP server from a
			// malicious page reaching it via a rebinding hostname; this server
			// is reached through the shipped nginx, which forwards the
			// client's public Host to the backend on 127.0.0.1, so the guard
			// would reject every remote assistant (issue #1441). The endpoint
			// is not localhost-only and is not left open: it sits behind
			// AuthMiddleware (session or mycorrhizal_ API token) plus the same
			// rate limit and CORS policy as /api/v1, so an unauthenticated
			// rebinding page still cannot run a tool.
			DisableLocalhostProtection: true,
		})
		handler.ServeHTTP(c.Writer, c.Request)
	}
}
