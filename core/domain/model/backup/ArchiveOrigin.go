// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

// ArchiveOrigin is whose an archive is, seen from the workspace that asks to restore it - read
// off the archive's manifest and this installation's own workspaces.
type ArchiveOrigin int

const (
	// ArchiveOfOwnWorkspace is the asking workspace's own archive.
	ArchiveOfOwnWorkspace ArchiveOrigin = iota + 1
	// ArchiveOfLocalWorkspace is an archive of another workspace this installation holds - the
	// archive BK-10 protects.
	ArchiveOfLocalWorkspace
	// ArchiveOfUnknownWorkspace is an archive of a workspace this installation does not hold: an
	// export another installation wrote, or a workspace deleted here for good.
	ArchiveOfUnknownWorkspace
)

// Accepts reports whether the mode may read an archive of that origin (backup-restore.md §8.2,
// BK-10).
//
// Every mode takes the workspace's own archive. Only NEW_TENANT takes one of a workspace this
// installation does not hold, because it writes into a workspace minted a moment ago and so
// overwrites nobody's data; it is the provider-migration path (tenant-export.md §10). No mode takes
// an archive of another workspace that exists here: a restore never becomes the way one workspace
// reads another. INSTANCE is refused before this question is asked, for its own reason.
func (m RestoreMode) Accepts(origin ArchiveOrigin) bool {
	switch origin {
	case ArchiveOfOwnWorkspace:
		return true
	case ArchiveOfUnknownWorkspace:
		return m == RestoreNewTenant
	default:
		return false
	}
}
