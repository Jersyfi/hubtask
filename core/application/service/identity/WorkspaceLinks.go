// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// WorkspaceLinks answers an adapter that writes a link outside a mail - a calendar feed's address,
// a CalDAV todo's, an export's - the address of the request's workspace (UC-INS-19 check 9). Its
// own read-only transaction, so the adapter reaches the main row through the application layer and
// the tenant-bound wrapper rather than through a repository (rules 2 and 3).
//
// Not an authorisation: the workspace is the caller's own, named by its credential, and its address
// is no secret - it is the host the caller already reached.
type WorkspaceLinks struct {
	Address    WorkspaceAddress
	UnitOfWork persistence.UnitOfWork
}

// For answers the address of the workspace tenantID names; empty where none can be named, which
// leaves the link relative rather than on the installation's host.
func (l WorkspaceLinks) For(ctx context.Context, tenantID shared.ID) (string, error) {
	if !l.Address.Multi {
		return l.Address.Of(ctx)
	}
	if tenantID.IsZero() || l.UnitOfWork == nil {
		return "", nil
	}
	var base string
	err := l.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		var err error
		base, err = l.Address.Of(ctx)
		return err
	})
	return base, err
}
