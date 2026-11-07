// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// DirectoryLinks is what a directory provisioning over SCIM needs beside the accounts and groups
// themselves (UC-ID-28): the identifier the directory knows each by, the people it switches off
// and on, and pages by position. The tenant boundary is the transaction's (ADR-0010).
type DirectoryLinks interface {
	// AccountExternalIDs answers the identifiers of those accounts that have one.
	AccountExternalIDs(ctx context.Context, ids []shared.ID) (map[shared.ID]string, error)
	// FindAccountByExternalID answers the person the directory knows by the identifier, or
	// shared.ErrNotFound.
	FindAccountByExternalID(ctx context.Context, externalID string) (shared.ID, error)
	// SetAccountExternalID writes or, empty, clears it; another person holding it is a conflict.
	SetAccountExternalID(ctx context.Context, accountID shared.ID, externalID string, at time.Time) error
	// SetAccountStatus moves a person between ACTIVE, INVITED and DISABLED and reports whether a
	// row moved: a restricted or anonymised account never does.
	SetAccountStatus(ctx context.Context, accountID shared.ID, status identity.AccountStatus, at time.Time) (bool, error)
	// PageAccounts answers the people from a position, in a stable order, and how many there are.
	PageAccounts(ctx context.Context, offset, limit int) ([]shared.ID, int, error)

	GroupExternalIDs(ctx context.Context, ids []shared.ID) (map[shared.ID]string, error)
	FindGroupByExternalID(ctx context.Context, externalID string) (shared.ID, error)
	// FindGroupByName compares as the name's uniqueness does.
	FindGroupByName(ctx context.Context, name string) (shared.ID, error)
	SetGroupExternalID(ctx context.Context, groupID shared.ID, externalID string) error
	PageGroups(ctx context.Context, offset, limit int) ([]shared.ID, int, error)
}
