// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup_test

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
)

// backup-restore.md §8.2 and BK-10: an archive is restored only from its own workspace, except
// that NEW_TENANT takes one of a workspace this installation does not hold. Another workspace of
// this installation stays out of reach in every mode.
func TestWhoseArchiveEachModeAccepts(t *testing.T) {
	modes := []domain.RestoreMode{
		domain.RestoreInspect, domain.RestoreSelective, domain.RestoreMerge,
		domain.RestoreReplaceTenant, domain.RestoreNewTenant, domain.RestoreInstance,
	}
	for _, origin := range []struct {
		name     string
		origin   domain.ArchiveOrigin
		accepted func(domain.RestoreMode) bool
	}{
		{"own workspace", domain.ArchiveOfOwnWorkspace, func(domain.RestoreMode) bool { return true }},
		{"other local workspace", domain.ArchiveOfLocalWorkspace, func(domain.RestoreMode) bool { return false }},
		{"unknown workspace", domain.ArchiveOfUnknownWorkspace, func(m domain.RestoreMode) bool {
			return m == domain.RestoreNewTenant
		}},
		{"no origin", domain.ArchiveOrigin(0), func(domain.RestoreMode) bool { return false }},
	} {
		for _, mode := range modes {
			t.Run(origin.name+"/"+string(mode), func(t *testing.T) {
				if got, want := mode.Accepts(origin.origin), origin.accepted(mode); got != want {
					t.Errorf("accepts is %v, want %v", got, want)
				}
			})
		}
	}
}
