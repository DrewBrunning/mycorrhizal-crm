package models

const (
	LegacyTokenScopeFull    = "full"
	LegacyTokenScopeCardDAV = "carddav"
	LegacyTokenScopeCustom  = "custom"

	TokenScopeCardDAV  = "carddav"
	TokenScopeCalDAV   = "caldav"
	TokenScopeAPIRead  = "api:read"
	TokenScopeAPIWrite = "api:write"
	TokenScopeMCP      = "mcp"
)

var (
	DAVTokenScopes  = []string{TokenScopeCardDAV, TokenScopeCalDAV}
	FullTokenScopes = []string{
		TokenScopeCardDAV,
		TokenScopeCalDAV,
		TokenScopeAPIRead,
		TokenScopeAPIWrite,
		TokenScopeMCP,
	}
	AllTokenScopes = []string{
		TokenScopeCardDAV,
		TokenScopeCalDAV,
		TokenScopeAPIRead,
		TokenScopeAPIWrite,
		TokenScopeMCP,
	}
)

func IsValidTokenScope(scope string) bool {
	for _, s := range AllTokenScopes {
		if s == scope {
			return true
		}
	}
	return false
}

func NormalizeTokenScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		seen[scope] = true
	}
	out := make([]string, 0, len(seen))
	for _, scope := range AllTokenScopes {
		if seen[scope] {
			out = append(out, scope)
		}
	}
	return out
}

func ScopesForLegacyScope(scope string) []string {
	switch scope {
	case "", LegacyTokenScopeFull:
		return append([]string(nil), FullTokenScopes...)
	case LegacyTokenScopeCardDAV:
		return append([]string(nil), DAVTokenScopes...)
	default:
		return nil
	}
}

func LegacyScopeForScopes(scopes []string) string {
	has := func(want string) bool {
		for _, scope := range scopes {
			if scope == want {
				return true
			}
		}
		return false
	}
	if has(TokenScopeCardDAV) && has(TokenScopeCalDAV) &&
		!has(TokenScopeAPIRead) && !has(TokenScopeAPIWrite) && !has(TokenScopeMCP) {
		return LegacyTokenScopeCardDAV
	}
	for _, scope := range FullTokenScopes {
		if !has(scope) {
			return LegacyTokenScopeCustom
		}
	}
	return LegacyTokenScopeFull
}

func (t *ApiToken) EffectiveScopes() []string {
	if len(t.Scopes) > 0 {
		return append([]string(nil), t.Scopes...)
	}
	return ScopesForLegacyScope(t.Scope)
}

func (t *ApiToken) HasScope(scope string) bool {
	for _, s := range t.EffectiveScopes() {
		if s == scope {
			return true
		}
	}
	return false
}

func (t *ApiToken) HasAnyAPIScope() bool {
	return t.HasScope(TokenScopeAPIRead) || t.HasScope(TokenScopeAPIWrite) || t.HasScope(TokenScopeMCP)
}

// IsDAVOnlyScopes reports whether every scope is a DAV scope. Only DAV app
// passwords may be issued without an expiry (issue #2010): an API/MCP token
// with no expiry would be an unbounded automation credential.
func IsDAVOnlyScopes(scopes []string) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if scope != TokenScopeCardDAV && scope != TokenScopeCalDAV {
			return false
		}
	}
	return true
}
