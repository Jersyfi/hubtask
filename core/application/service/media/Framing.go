// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// WorkspaceHosts answers which workspace a request's host serves: by its host in multi mode, the
// one workspace by the empty slug in single mode.
type WorkspaceHosts interface {
	ResolveHost(ctx context.Context, host string) (shared.ID, error)
	Resolve(ctx context.Context, slug string) (shared.ID, error)
}

// FramedServer answers, for the page a browser loads, the origin of the document server its
// workspace entered, which the page's policy opens for the editor's frame and its form (ADR-0104,
// security.md §9). Nobody's right is asked: the answer is the policy header every visitor of the
// page receives anyway, and a host that names no workspace answers none.
type FramedServer struct {
	Tenants    WorkspaceHosts
	Wopi       repository.Wopi
	UnitOfWork persistence.UnitOfWork
	// Multi reads the host; single mode has one workspace and reads none.
	Multi bool
	// InstallationHost is the installation's own host, which serves no workspace's page.
	InstallationHost string
}

// OriginFor answers the entered server's origin for a page loaded on this host, or empty.
func (f FramedServer) OriginFor(ctx context.Context, rawHost string) (string, error) {
	var tenantID shared.ID
	err := f.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var err error
		if !f.Multi {
			tenantID, err = f.Tenants.Resolve(ctx, "")
			return err
		}
		host := identity.RequestHost(rawHost)
		if host == "" || host == identity.RequestHost(f.InstallationHost) {
			return nil
		}
		tenantID, err = f.Tenants.ResolveHost(ctx, host)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && tenantID.IsZero()) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var origin string
	err = f.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		choice, err := f.Wopi.DocumentServer(ctx)
		if err == nil && choice.IsOn() {
			origin = choice.URL
		}
		return err
	})
	return origin, err
}
