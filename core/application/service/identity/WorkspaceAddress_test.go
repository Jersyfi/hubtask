// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type failingHosts struct{}

func (failingHosts) Insert(context.Context, domain.TenantHost) error { return nil }

func (failingHosts) List(context.Context) ([]domain.TenantHost, error) {
	return nil, shared.ErrInternal.WithDetail("test.hosts_unavailable")
}

// UC-INS-19 check 9: every link to a workspace is built on its main row, whatever the short name.
func TestAWorkspacesAddressIsItsMainRow(t *testing.T) {
	anna := &workspaceStore{row: domain.Workspace{Slug: "anna"}}
	changed := tenantHostsFake{hosts: []domain.TenantHost{
		{Host: "anna.hub.example", State: domain.HostActive},
		{Host: "tasks.anna.example", State: domain.HostActive, Canonical: true},
	}}
	for _, c := range []struct {
		name    string
		address WorkspaceAddress
		want    string
	}{
		{"single mode is the installation's address",
			WorkspaceAddress{Workspaces: anna, Hosts: changed, BaseURL: "https://hub.example/"}, "https://hub.example"},
		{"the main row, under the installation's scheme and port",
			WorkspaceAddress{Workspaces: anna, Hosts: changed, BaseURL: "https://hub.example:8443", Multi: true},
			"https://tasks.anna.example:8443"},
		{"a main row that does not serve is no address",
			WorkspaceAddress{Workspaces: anna, Hosts: tenantHostsFake{hosts: []domain.TenantHost{
				{Host: "tasks.anna.example", State: domain.HostBroken, Canonical: true},
			}}, BaseURL: "https://hub.example", Multi: true}, "https://anna.hub.example"},
		{"no row yet answers the subdomain the short name makes",
			WorkspaceAddress{Workspaces: anna, Hosts: tenantHostsFake{}, BaseURL: "https://hub.example", Multi: true},
			"https://anna.hub.example"},
		{"a host beyond ASCII as a link carries it",
			WorkspaceAddress{Workspaces: anna, Domains: punycode{}, Hosts: tenantHostsFake{hosts: []domain.TenantHost{
				{Host: "müller.de", State: domain.HostActive, Canonical: true},
			}}, BaseURL: "https://hub.example", Multi: true}, "https://xn--mller-kva.de"},
		{"no installation address composes nothing",
			WorkspaceAddress{Workspaces: anna, Hosts: changed, Multi: true}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.address.Of(t.Context())
			if err != nil || got != c.want {
				t.Errorf("answered (%q, %v), want %q", got, err, c.want)
			}
		})
	}
}

func TestAnAddressThatCannotBeReadIsAnError(t *testing.T) {
	address := WorkspaceAddress{
		Workspaces: &workspaceStore{row: domain.Workspace{Slug: "anna"}}, Hosts: failingHosts{},
		BaseURL: "https://hub.example", Multi: true,
	}
	if _, err := address.Of(t.Context()); !errors.Is(err, shared.ErrInternal) {
		t.Errorf("answered %v, want the read's error rather than a link to a guessed host", err)
	}
}
