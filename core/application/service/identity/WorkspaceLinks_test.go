// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// readOnlyOnly records the scopes it was asked for and refuses a read-write transaction.
type readOnlyOnly struct{ scopes []persistence.Scope }

func (u *readOnlyOnly) Within(context.Context, persistence.Scope, func(context.Context) error) error {
	return shared.ErrInternal.WithDetail("test.read_write_refused")
}

func (u *readOnlyOnly) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

// UC-INS-19 check 9: a link an adapter writes names the main row as it stands, read in the
// request's own workspace.
func TestALinkOutsideAMailFollowsTheMainRow(t *testing.T) {
	tenant := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000c0a10")
	hosts := &tenantHostsFake{hosts: []domain.TenantHost{{Host: "anna.hub.example", State: domain.HostActive, Canonical: true}}}
	uow := &readOnlyOnly{}
	links := WorkspaceLinks{UnitOfWork: uow, Address: WorkspaceAddress{
		Workspaces: &workspaceStore{row: domain.Workspace{Slug: "anna"}}, Hosts: hosts,
		BaseURL: "https://hub.example", Multi: true,
	}}

	if got, err := links.For(t.Context(), tenant); err != nil || got != "https://anna.hub.example" {
		t.Fatalf("before the change answered (%q, %v)", got, err)
	}
	hosts.hosts = []domain.TenantHost{{Host: "tasks.anna.example", State: domain.HostActive, Canonical: true}}
	if got, err := links.For(t.Context(), tenant); err != nil || got != "https://tasks.anna.example" {
		t.Errorf("after the change answered (%q, %v), want the new main row", got, err)
	}
	for _, scope := range uow.scopes {
		if scope.TenantID != tenant || scope.Installation || scope.System {
			t.Errorf("read under %+v, want the request's workspace", scope)
		}
	}
	if len(uow.scopes) != 2 {
		t.Errorf("%d transactions, want one per link", len(uow.scopes))
	}
}

func TestALinkWithoutAWorkspaceStaysRelative(t *testing.T) {
	uow := &readOnlyOnly{}
	multi := WorkspaceLinks{UnitOfWork: uow, Address: WorkspaceAddress{BaseURL: "https://hub.example", Multi: true}}
	if got, err := multi.For(t.Context(), shared.ID("")); err != nil || got != "" {
		t.Errorf("no workspace answered (%q, %v), want no address rather than the installation's", got, err)
	}
	single := WorkspaceLinks{Address: WorkspaceAddress{BaseURL: "https://hub.example/"}}
	if got, err := single.For(t.Context(), shared.ID("")); err != nil || got != "https://hub.example" {
		t.Errorf("single mode answered (%q, %v), want the installation's address", got, err)
	}
	if len(uow.scopes) != 0 {
		t.Errorf("opened %d transactions for no workspace", len(uow.scopes))
	}
}
