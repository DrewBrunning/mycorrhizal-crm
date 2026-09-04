//go:build linux

package fsguard

import (
	"fmt"
	"syscall"
)

// statfsType is the real implementation of typeProbe: a syscall.Statfs, the
// same primitive internal/diskspace and services/alerting_conditions.go
// already use for free-space geometry. Statfs_t.Type is Linux-specific (it is
// the numeric f_type filesystem magic from statfs(2)); other platforms report
// filesystem type differently or not at all, which is why this file is
// //go:build linux and fsguard_other.go covers everything else.
func statfsType(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs %q: %w", path, err)
	}
	return st.Type, nil
}
