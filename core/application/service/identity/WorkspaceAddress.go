// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
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
