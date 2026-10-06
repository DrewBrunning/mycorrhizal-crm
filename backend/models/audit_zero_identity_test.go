package models

import "testing"

func TestSkipZeroIdentityAudit(t *testing.T) {
	cases := []struct {
		name     string
		entityID string
		userID   uint
		skip     bool
	}{
		{"real row", "7", 1, false},
		{"uuid row", "7f0c3c1e-0000-4000-8000-000000000000", 1, false},
		{"empty id", "", 1, true},
		{"zero id", "0", 1, true},
		{"zero user", "7", 0, true},
		{"all zero", "", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := skipZeroIdentityAudit("note", tc.entityID, tc.userID, AuditOpDelete); got != tc.skip {
				t.Fatalf("skip = %v, want %v", got, tc.skip)
			}
		})
	}
}
