// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// locator answers for the fixture's flows at the given time, in multi-tenant operation, the
// workspace being "anna".
func locator(f *oidcFixture, at time.Time) LocateOidcFlow {
	return LocateOidcFlow{
		Flows: f.flows, UnitOfWork: f.session.work, Clock: clock.Fixed(at),
		Address: WorkspaceAddress{
			Workspaces: &workspaceStore{row: domain.Workspace{Slug: "anna"}},
			BaseURL:    "https://hub.example", Multi: true,
		},
	}
}

// UC-ID-07 check 7, the return's half: a live flow is located at the workspace's own address, read
// in the tenant its state names, and the lookup spends nothing - the return is finished afterwards.
func TestALiveFlowIsLocatedAtItsWorkspacesAddress(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	state := start(t, f)
	f.session.work.scopes = nil

	base, err := locator(f, at.Add(time.Minute)).Execute(t.Context(), state.Reveal())
	if err != nil {
		t.Fatalf("locating: %v", err)
	}
	if base != "https://anna.hub.example" {
		t.Errorf("base %q, want the workspace's own address", base)
	}
	if len(f.session.work.scopes) != 1 || f.session.work.scopes[0] != (persistence.Scope{TenantID: tenant}) {
		t.Errorf("scopes %v, want the one tenant the state names", f.session.work.scopes)
	}
	if _, err := (CompleteOidcSignIn{Writer: f.writer}).
		Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: state}); err != nil {
		t.Errorf("the located flow could not be finished: %v", err)
	}
}

// In single-tenant operation there is one address, and it is the answer.
func TestInSingleTenantOperationTheAnswerIsTheInstallationsAddress(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	state := start(t, f)
	locate := locator(f, at)
	locate.Address.Multi = false

	base, err := locate.Execute(t.Context(), state.Reveal())
	if err != nil {
		t.Fatalf("locating: %v", err)
	}
	if base != "https://hub.example" {
		t.Errorf("base %q, want the installation's address", base)
	}
}

// Unknown, expired, spent and malformed are one refusal, and none names a workspace.
func TestEveryStateButALiveOneIsTheSameNotFound(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)

	expired := start(t, f)
	spent := start(t, f)
	if _, err := (CompleteOidcSignIn{Writer: f.writer}).
		Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: spent}); err != nil {
		t.Fatalf("spending: %v", err)
	}
	unknown, _ := domain.NewOidcFlowState(tenant, make([]byte, domain.TokenSecretBytes))

	cases := map[string]struct {
		state string
		at    time.Time
	}{
		"unknown":   {unknown.Secret(), at},
		"expired":   {expired.Reveal(), at.Add(domain.OidcFlowLifetime + time.Second)},
		"spent":     {spent.Reveal(), at},
		"malformed": {"not-a-state", at},
		"empty":     {"", at},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			base, err := locator(f, c.at).Execute(t.Context(), c.state)
			if base != "" {
				t.Errorf("answered %q for a flow that is not live", base)
			}
			got := shared.AsError(err)
			if got == nil || got.Code != shared.ErrNotFound.Code || got.DetailCode != "auth.oidc_flow_not_found" {
				t.Errorf("err %v, want the one not-found", err)
			}
		})
	}
}

// A workspace gone between the start and the return is the same refusal, not a different one.
func TestAWorkspaceGoneIsTheSameNotFound(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	state := start(t, f)
	locate := locator(f, at)
	locate.Address.Workspaces = &workspaceStore{missing: true}

	_, err := locate.Execute(t.Context(), state.Reveal())
	if got := shared.AsError(err); got == nil || got.DetailCode != "auth.oidc_flow_not_found" {
		t.Errorf("err %v, want the one not-found", err)
	}
}

// UC-INS-19 check 10, the answer's half: the return goes to the workspace's main address, the
// serving canonical row, even where it is not the subdomain the short name makes (ADR-0095 §2).
func TestAFlowIsLocatedAtItsWorkspacesMainRow(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	state := start(t, f)
	locate := locator(f, at.Add(time.Minute))
	locate.Address.Hosts = tenantHostsFake{hosts: []domain.TenantHost{
		{Host: "anna.hub.example", State: domain.HostActive},
		{Host: "tasks.anna.example", State: domain.HostActive, Canonical: true},
	}}

	base, err := locate.Execute(t.Context(), state.Reveal())
	if err != nil {
		t.Fatalf("locating: %v", err)
	}
	if base != "https://tasks.anna.example" {
		t.Errorf("base %q, want the main row's host", base)
	}
}
