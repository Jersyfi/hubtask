// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// WorkspaceAddress answers the address every link to a workspace names (ADR-0095 §2): a mail's
// link, an invitation handed over, the provider's return. In multi mode it is the workspace's main
// `tenant_host` row under the installation's scheme and port - read at the moment the link is
// written, so a link written after a change of address names the new one; in single mode the
// installation's own address.
//
// The one place a workspace's link reads BaseURL: a writer that composed the address itself would
// keep the host a change of address left behind (UC-INS-19 check 9).
type WorkspaceAddress struct {
	// Workspaces answers the short name, whose subdomain stands in while no main row serves - the
	// derived host, the one a broken own domain falls back to.
	Workspaces WorkspaceSlug
	// Hosts holds the main row. Nil answers the subdomain the short name makes.
	Hosts repository.TenantHosts
	// Domains writes a host with letters beyond ASCII in the form a link carries. Optional: nil
	// leaves the host as stored.
	Domains text.DomainEncoder
	// BaseURL and Multi are the installation's address and whether it runs several workspaces.
	BaseURL string
	Multi   bool
}

// Of answers the address of the workspace the transaction in ctx is bound to; empty where the
// installation has no address, which composes no link rather than a relative one.
func (a WorkspaceAddress) Of(ctx context.Context) (string, error) {
	if !a.Multi || a.BaseURL == "" {
		return domain.WorkspaceBase(a.BaseURL, "", false), nil
	}
	if a.Hosts != nil {
		hosts, err := a.Hosts.List(ctx)
		if err != nil {
			return "", err
		}
		if main := domain.MainHostOf(hosts); main != "" {
			if a.Domains != nil {
				if ascii, err := a.Domains.ToASCII(main); err == nil {
					main = ascii
				}
			}
			return domain.WorkspaceBaseAt(a.BaseURL, main), nil
		}
	}
	if a.Workspaces == nil {
		return domain.WorkspaceBase(a.BaseURL, "", false), nil
	}
	workspace, err := a.Workspaces.Find(ctx)
	if err != nil {
		return "", err
	}
	return domain.WorkspaceBase(a.BaseURL, workspace.Slug, true), nil
}

// BrowserAddress answers where a browser on a host that is not its workspace's main address is
// sent (UC-INS-15 check 7): the subdomain beside a main own domain, and an own domain beside the
// subdomain after switching back, send a browser on to the main address; the API answers on both.
// Every other host answers empty and is served as it is.
type BrowserAddress struct {
	Hosts      repository.BrowserHosts
	UnitOfWork persistence.UnitOfWork
	// BaseURL is the installation's address, whose scheme and port the redirect keeps;
	// InstallationHost its host, which no workspace's row is and which is never asked about.
	BaseURL, InstallationHost string
	Tenancy                   env.TenancyMode
}

// RedirectFor answers the main address's base - scheme, host and port - for a host a browser should
// leave, or empty.
func (b BrowserAddress) RedirectFor(ctx context.Context, rawHost string) (string, error) {
	host := domain.RequestHost(rawHost)
	if b.Tenancy != env.TenancyMulti || b.Hosts == nil || host == "" || host == domain.RequestHost(b.InstallationHost) {
		return "", nil
	}
	var main string
	err := b.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		found, err := b.Hosts.BrowserHost(ctx, host)
		main = found
		return err
	})
	if err != nil || main == "" {
		return "", err
	}
	return domain.WorkspaceBaseAt(b.BaseURL, main), nil
}
