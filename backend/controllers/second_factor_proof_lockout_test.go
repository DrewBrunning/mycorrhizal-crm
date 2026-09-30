package controllers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1352: every session-authenticated second-factor proof shares ONE
// account-keyed attempt limiter (requireSecondFactorProof, bucket
// secondFactorProofLockKey).

// proofRoute is one call site of the helper.
type proofRoute struct {
	name string
	// do sends the request with the given code and returns the status.
	do func(e *waEnv, tok, passkeyID, code string) int
	// effect reports whether the route's side effect happened.
	effect func(e *waEnv) bool
}

func proofRoutes() []proofRoute {
	return []proofRoute{
		{
			name: "disable",
			do: func(e *waEnv, tok, _, code string) int {
				w, _ := e.do("POST", "/users/2fa/disable", map[string]string{"code": code}, tok)
				return w.Code
			},
			effect: func(e *waEnv) bool { return !e.reload().TOTPEnabled },
		},
		{
			name: "regenerate",
			do: func(e *waEnv, tok, _, code string) int {
				w, _ := e.do("POST", "/users/2fa/recovery-codes/regenerate", map[string]string{"code": code}, tok)
				return w.Code
			},
			// the old set is replaced, so the code we would have used is gone
			effect: func(e *waEnv) bool { return !e.hasRecoveryCode(e.codes[0]) },
		},
		{
			name: "delete passkey",
			do: func(e *waEnv, tok, id, code string) int {
				w, _ := e.do("DELETE", "/webauthn/credentials/"+id, map[string]string{"code": code}, tok)
				return w.Code
			},
			effect: func(e *waEnv) bool { return e.credentialCount() == 0 },
		},
		{
			name: "delete account",
			do: func(e *waEnv, tok, _, code string) int {
				w, _ := e.do("DELETE", "/account", map[string]string{"current_password": strongPassword, "totp_code": code}, tok)
				return w.Code
			},
			effect: func(e *waEnv) bool {
				var n int64
				require.NoError(e.t, e.db.Unscoped().Model(&models.User{}).Where("id = ?", e.user.ID).Count(&n).Error)
				return n == 0
			},
		},
	}
}

func (e *waEnv) hasRecoveryCode(code string) bool {
	var n int64
	require.NoError(e.t, e.db.Model(&models.RecoveryCode{}).
		Where("user_id = ? AND code_hash = ?", e.user.ID, services.HashRecoveryCode(code)).Count(&n).Error)
	return n == 1
}

// proofEnv builds a TOTP + passkey account (so every route is reachable) and
// returns the env, a live session token and the passkey id. Proofs in these
// tests are recovery codes (e.codes).
func proofEnv(t *testing.T) (*waEnv, string, string) {
	t.Helper()
	e := newWAEnv(t)
	t.Cleanup(func() { middleware.GetAccountRateLimiter().RecordSuccessfulLogin(secondFactorProofLockKey(e.user.ID)) })
	_, tok := e.enableTOTP(e.session())
	id, _, tok := e.enroll(e.auth(), "Key", tok)
	e.lastAuth = nil
	return e, tok, id
}

const wrongProof = "ZZZZZ-ZZZZZ-ZZZZZ"

func TestSecondFactorProof_LocksAfterMaxAttemptsOnEveryRoute(t *testing.T) {
	for _, r := range proofRoutes() {
		t.Run(r.name, func(t *testing.T) {
			e, tok, id := proofEnv(t)
			good := e.codes[0]

			for i := 0; i < middleware.MaxLoginAttempts-1; i++ {
				assert.Equal(t, http.StatusBadRequest, r.do(e, tok, id, wrongProof), "miss %d", i+1)
			}
			assert.Equal(t, http.StatusTooManyRequests, r.do(e, tok, id, wrongProof), "the attempt that trips the limit")
			// Locked: a genuine proof is refused and nothing happens.
			assert.Equal(t, http.StatusTooManyRequests, r.do(e, tok, id, good), "correct code while locked")
			assert.False(t, r.effect(e), "no side effect while locked")
			assert.True(t, e.hasRecoveryCode(good), "the recovery code was not consumed")

			// The owner's sign-in is a different bucket.
			pending, _ := e.passwordStep()
			assert.NotNil(t, pending)
		})
	}
}

func TestSecondFactorProof_SuccessResetsCounter(t *testing.T) {
	for _, r := range proofRoutes() {
		t.Run(r.name, func(t *testing.T) {
			e, tok, id := proofEnv(t)
			for i := 0; i < middleware.MaxLoginAttempts-1; i++ {
				require.Equal(t, http.StatusBadRequest, r.do(e, tok, id, wrongProof))
			}
			require.Equal(t, http.StatusOK, r.do(e, tok, id, e.codes[0]), "a genuine proof before the lock succeeds")
			assert.Equal(t, 0, middleware.GetAccountRateLimiter().GetFailedAttempts(secondFactorProofLockKey(e.user.ID)))
		})
	}
}

// One bucket for all routes: guesses cannot be spread across endpoints.
func TestSecondFactorProof_BucketIsSharedAcrossRoutes(t *testing.T) {
	e, tok, id := proofEnv(t)
	routes := proofRoutes()
	var last int
	for i := 0; i < middleware.MaxLoginAttempts; i++ {
		last = routes[i%len(routes)].do(e, tok, id, wrongProof)
	}
	assert.Equal(t, http.StatusTooManyRequests, last)
	for _, r := range routes {
		assert.Equal(t, http.StatusTooManyRequests, r.do(e, tok, id, e.codes[0]), r.name)
	}
}

// TestProofSitesGoThroughHelper is the completeness guard: valid2FAProof and
// verifyProofAssertion may only be called by requireSecondFactorProof (the
// session-authenticated gate) and Complete2FALogin (pre-session, login buckets).
func TestProofSitesGoThroughHelper(t *testing.T) {
	allowed := map[string]bool{"requireSecondFactorProof": true, "Complete2FALogin": true}
	watched := map[string]bool{"valid2FAProof": true, "verifyProofAssertion": true}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	found := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		file, err := parser.ParseFile(token.NewFileSet(), f, src, 0)
		require.NoError(t, err)
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && watched[id.Name] {
					found++
					assert.True(t, allowed[fn.Name.Name], "%s: %s calls %s directly; use requireSecondFactorProof (issue #1352)", f, fn.Name.Name, id.Name)
				}
				return true
			})
		}
	}
	assert.GreaterOrEqual(t, found, 3, "the guard must actually see the known call sites")
}
