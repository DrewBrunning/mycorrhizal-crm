package services

import (
	"net/http"
	"testing"
	"time"

	apperrors "mycorrhizal/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are the user-visible messages pinned by issue #2008. Every import
// source must render the same wording and must not double the "not found"
// suffix that ErrNotFound itself appends.
const (
	wantImportSessionUnknownMessage = "Import session expired or not found"
	wantImportSessionExpiredMessage = "Import session expired"
)

type importSessionErrorFn func() *apperrors.AppError

// TestImportSessionGet_NotFoundMessages pins the exact rendered message for an
// unknown session and an expired session across every import source. Ownership
// failures remain 401 and are covered by each source's own tests.
func TestImportSessionGet_NotFoundMessages(t *testing.T) {
	expired := Now().Add(-time.Minute)
	future := Now().Add(time.Hour)

	cases := []struct {
		name  string
		setup func(t *testing.T) (getUnknown, getExpired importSessionErrorFn)
	}{
		{
			name: "CSV and vCard",
			setup: func(t *testing.T) (importSessionErrorFn, importSessionErrorFn) {
				m := NewImportSessionManager()
				id := m.CreateCSVSession(1, []string{"First"}, [][]string{{"Alice"}})
				m.mu.Lock()
				m.sessions[id].session.ExpiresAt = expired
				m.mu.Unlock()
				get := func(sessionID string, userID uint) *apperrors.AppError {
					_, appErr := m.get(sessionID, userID)
					return appErr
				}
				return func() *apperrors.AppError { return get("missing-session", 1) },
					func() *apperrors.AppError { return get(id, 1) }
			},
		},
		{
			name: "Meerkat",
			setup: func(t *testing.T) (importSessionErrorFn, importSessionErrorFn) {
				m := NewMeerkatImportManager()
				id := generateSessionID()
				m.sessions[id] = &meerkatImportSession{
					id: id, userID: 1, expiresAt: expired, hardExpiry: future,
				}
				get := func(sessionID string, userID uint) *apperrors.AppError {
					_, appErr := m.get(sessionID, userID)
					return appErr
				}
				return func() *apperrors.AppError { return get("missing-session", 1) },
					func() *apperrors.AppError { return get(id, 1) }
			},
		},
		{
			name: "Monica",
			setup: func(t *testing.T) (importSessionErrorFn, importSessionErrorFn) {
				m := NewMonicaImportManager()
				id := generateSessionID()
				m.sessions[id] = &monicaImportSession{
					id: id, userID: 1, expiresAt: expired, hardExpiry: future,
				}
				get := func(sessionID string, userID uint) *apperrors.AppError {
					_, appErr := m.get(sessionID, userID)
					return appErr
				}
				return func() *apperrors.AppError { return get("missing-session", 1) },
					func() *apperrors.AppError { return get(id, 1) }
			},
		},
		{
			name: "account bundle",
			setup: func(t *testing.T) (importSessionErrorFn, importSessionErrorFn) {
				m := NewMycorrhizalImportManager()
				id := generateSessionID()
				m.sessions[id] = &mycorrhizalImportSession{
					id: id, userID: 1, expiresAt: expired, hardExpiry: future,
				}
				get := func(sessionID string, userID uint) *apperrors.AppError {
					_, appErr := m.get(sessionID, userID)
					return appErr
				}
				return func() *apperrors.AppError { return get("missing-session", 1) },
					func() *apperrors.AppError { return get(id, 1) }
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getUnknown, getExpired := tc.setup(t)

			unknown := getUnknown()
			require.NotNil(t, unknown)
			assert.Equal(t, apperrors.ErrCodeNotFound, unknown.Code)
			assert.Equal(t, http.StatusNotFound, unknown.HTTPStatus)
			assert.Equal(t, wantImportSessionUnknownMessage, unknown.Message)

			expiredErr := getExpired()
			require.NotNil(t, expiredErr)
			assert.Equal(t, apperrors.ErrCodeNotFound, expiredErr.Code)
			assert.Equal(t, http.StatusNotFound, expiredErr.HTTPStatus)
			assert.Equal(t, wantImportSessionExpiredMessage, expiredErr.Message)
		})
	}
}

// TestImportSessionGet_ForeignSessionIsIndistinguishableFromUnknown pins the
// existence-oracle fix (#2203): a session that exists but belongs to another
// user must produce the exact error an unknown id produces (status, code,
// message and reason code), so a caller cannot probe which ids are live.
func TestImportSessionGet_ForeignSessionIsIndistinguishableFromUnknown(t *testing.T) {
	const owner, intruder = uint(1), uint(2)
	future := Now().Add(time.Hour)

	type getFn func(sessionID string, userID uint) *apperrors.AppError
	cases := []struct {
		name  string
		setup func() (id string, get getFn)
	}{
		{"CSV and vCard", func() (string, getFn) {
			m := NewImportSessionManager()
			id := m.CreateCSVSession(owner, []string{"First"}, [][]string{{"Alice"}})
			return id, func(s string, u uint) *apperrors.AppError { _, e := m.get(s, u); return e }
		}},
		{"Meerkat", func() (string, getFn) {
			m := NewMeerkatImportManager()
			id := generateSessionID()
			m.sessions[id] = &meerkatImportSession{id: id, userID: owner, expiresAt: future, hardExpiry: future}
			return id, func(s string, u uint) *apperrors.AppError { _, e := m.get(s, u); return e }
		}},
		{"Monica", func() (string, getFn) {
			m := NewMonicaImportManager()
			id := generateSessionID()
			m.sessions[id] = &monicaImportSession{id: id, userID: owner, expiresAt: future, hardExpiry: future}
			return id, func(s string, u uint) *apperrors.AppError { _, e := m.get(s, u); return e }
		}},
		{"account bundle", func() (string, getFn) {
			m := NewMycorrhizalImportManager()
			id := generateSessionID()
			m.sessions[id] = &mycorrhizalImportSession{id: id, userID: owner, expiresAt: future, hardExpiry: future}
			return id, func(s string, u uint) *apperrors.AppError { _, e := m.get(s, u); return e }
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, get := tc.setup()

			unknown := get("missing-session", intruder)
			foreign := get(id, intruder)
			require.NotNil(t, unknown)
			require.NotNil(t, foreign)

			assert.Equal(t, http.StatusNotFound, foreign.HTTPStatus)
			assert.Equal(t, unknown.HTTPStatus, foreign.HTTPStatus)
			assert.Equal(t, unknown.Code, foreign.Code)
			assert.Equal(t, unknown.Message, foreign.Message)
			assert.Equal(t, unknown.ReasonCode, foreign.ReasonCode)
			assert.Equal(t, unknown.ReasonParams, foreign.ReasonParams)
			assert.Equal(t, unknown.Details, foreign.Details)
			assert.NotContains(t, foreign.Message, "belong")
		})
	}
}
