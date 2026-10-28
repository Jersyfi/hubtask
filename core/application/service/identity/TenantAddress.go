// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// requestTenant is multi-tenancy.md §3 for a request without a credential: in single mode the one
// workspace; in multi mode the workspace whose serving row the host is, then the header. A header
// naming another workspace than the host is `access.tenant_mismatch`; nothing naming one is
// `auth.tenant_unresolved`. A host no row serves - the installation's own, an old address - names
// nothing, so the header may still answer.
func requestTenant(
	ctx context.Context, uow persistence.UnitOfWork, tenants repository.TenantDirectory,
	multi bool, rawHost, header string,
) (shared.ID, error) {
	unresolved := shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
	var tenantID shared.ID
	if !multi {
		// Single mode reads no host: there is one workspace, and localhost names none.
		err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			id, err := tenants.Resolve(ctx, "")
			tenantID = id
			return err
		})
		if err != nil {
			return "", err
		}
	} else if host := domain.RequestHost(rawHost); host != "" {
		err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			id, err := tenants.ResolveHost(ctx, host)
			if errors.Is(err, shared.ErrNotFound) {
				return nil
			}
			tenantID = id
			return err
		})
		if err != nil {
			return "", err
		}
	}
	if tenantID.IsZero() {
		if header == "" {
			return "", unresolved
		}
		id, err := shared.ParseID(header)
		if err != nil {
			return "", unresolved
		}
		tenantID = id
	}
	if header != "" && header != tenantID.String() {
		return "", shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	return tenantID, nil
}

// hostNamesAnother is the authenticated half of §3: the credential binds the workspace, and a host
// whose serving row is another workspace's is `access.tenant_mismatch`. A host no row serves
// confirms nothing and refuses nothing. Read in the caller's transaction: the door answers in any
// scope, and a scope of its own would be a tenant switch (known-traps.md).
func hostNamesAnother(
	ctx context.Context, tenants repository.TenantDirectory, rawHost string, bound shared.ID,
) error {
	host := domain.RequestHost(rawHost)
	if tenants == nil || host == "" {
		return nil
	}
	named, err := tenants.ResolveHost(ctx, host)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if named != bound {
		return shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	return nil
}
