// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"log/slog"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// requestTenant is multi-tenancy.md §3 for a request without a credential: in single mode the one
// workspace; in multi mode the workspace whose serving row the host is. The header is a second
// source only at the installation's own host; a host that is neither that nor served names nothing
// and is `auth.tenant_unresolved`, header or not (T-28). A header naming another workspace than the
// host is `access.tenant_mismatch`.
func requestTenant(
	ctx context.Context, uow persistence.UnitOfWork, tenants repository.TenantDirectory,
	multi bool, installationHost, rawHost, header string,
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
	} else if host := domain.RequestHost(rawHost); host != "" && host != domain.RequestHost(installationHost) {
		err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			id, err := tenants.ResolveHost(ctx, host)
			tenantID = id
			return err
		})
		if errors.Is(err, shared.ErrNotFound) {
			if !isInstallationHost(host, installationHost) {
				return "", unresolved
			}
			err = nil
		}
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

// isInstallationHost is whether a host no row serves is the installation's own, where the header
// speaks. An installation that did not state its address (`config.base_url_missing`) has no host to
// tell apart: every host no row serves is its own, as before any host was read.
func isInstallationHost(host, installationHost string) bool {
	own := domain.RequestHost(installationHost)
	return host == "" || own == "" || host == own
}

// hostNamesAnother is the authenticated half of §3: the credential binds the workspace, and only
// the installation's own host or the workspace's serving row confirms it. Another workspace's host,
// and a host no row serves, is `access.tenant_mismatch` (T-28). Read in the caller's transaction:
// the door answers in any scope, and a scope of its own would be a tenant switch (known-traps.md).
func hostNamesAnother(
	ctx context.Context, tenants repository.TenantDirectory, installationHost, rawHost string, bound shared.ID,
) error {
	host := domain.RequestHost(rawHost)
	if tenants == nil || host == "" || host == domain.RequestHost(installationHost) {
		return nil
	}
	named, err := tenants.ResolveHost(ctx, host)
	if errors.Is(err, shared.ErrNotFound) {
		if isInstallationHost(host, installationHost) {
			return nil
		}
		return shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	if err != nil {
		return err
	}
	if named != bound {
		return shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	return nil
}

// EnsureMainHosts gives every live workspace without a main address its row under the
// installation's host, at the start of a multi-mode server and before its first request
// (migration 0207, ADR-0095 §1). Authorisation is the configuration itself, as for the operators
// the environment names: nothing here is on a route.
type EnsureMainHosts struct {
	Hosts      repository.MainHosts
	UnitOfWork persistence.UnitOfWork
	// BaseHost is the installation's own host; every workspace's subdomain is a label under it.
	BaseHost string
}

// Apply writes the missing rows and answers how many it wrote.
func (e EnsureMainHosts) Apply(ctx context.Context) (int, error) {
	if e.Hosts == nil || domain.RequestHost(e.BaseHost) == "" {
		return 0, nil
	}
	var written int
	err := e.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		count, err := e.Hosts.EnsureMain(ctx, domain.RequestHost(e.BaseHost))
		written = count
		return err
	})
	if err != nil {
		return 0, err
	}
	if written > 0 {
		slog.InfoContext(ctx, "workspaces given their main address", slog.Int("count", written))
	}
	return written, nil
}
