// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// LdapDirectories is a workspace's LDAP directory (UC-ID-31) and whom it holds. The bind password
// is stored sealed and answered only to the run that opens it. The tenant boundary is the
// transaction's (ADR-0010).
type LdapDirectories interface {
	// Find answers the directory and its sealed password, or shared.ErrNotFound.
	Find(ctx context.Context) (identity.LdapDirectory, crypto.Sealed, error)
	// Save writes the configuration whole; the last run's record stays.
	Save(ctx context.Context, directory identity.LdapDirectory, sealed crypto.Sealed, by shared.ID) error
	// RecordRun writes a run's outcome and the next run; a directory removed meanwhile is
	// shared.ErrNotFound.
	RecordRun(ctx context.Context, directory identity.LdapDirectory) error
	// Delete removes the directory and its sealed password; none is shared.ErrNotFound.
	Delete(ctx context.Context) error
	// Held answers the people whose external identifier carries identity.LdapExternalPrefix.
	Held(ctx context.Context) ([]identity.LdapHeld, error)
	// HeldGroups answers the groups whose external identifier carries it.
	HeldGroups(ctx context.Context) ([]identity.LdapHeldGroup, error)
}
