// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// hosts maps a workspace's host to it; the one workspace of single mode answers to the empty slug.
type hosts map[string]shared.ID

func (h hosts) ResolveHost(_ context.Context, host string) (shared.ID, error) {
	if id, ok := h[host]; ok {
		return id, nil
	}
	return "", shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
}

func (h hosts) Resolve(ctx context.Context, slug string) (shared.ID, error) {
	return h.ResolveHost(ctx, slug)
}

// storedServers is each workspace's choice, read in its own scope.
type storedServers struct {
	chosenServer
	by     map[shared.ID]domain.DocumentServerChoice
	scopes *[]persistence.Scope
}

func (s storedServers) DocumentServer(ctx context.Context) (domain.DocumentServerChoice, error) {
	scope := scopeOf(ctx)
	*s.scopes = append(*s.scopes, scope)
	return s.by[scope.TenantID], nil
}

// UC-MED-06 check 2 (ADR-0104): the page's policy opens the editor's frame for the server the
// workspace the page loads in has entered - not another workspace's, none on the installation's
// own host, and none again once the server is removed.
func TestThePolicyOpensOnlyTheWorkspacesOwnDocumentServer(t *testing.T) {
	nordlicht := shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	boreal := shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	collabora, _ := domain.NewDocumentServerChoice(domain.DocumentServerCollabora, "https://office.nordlicht.example")
	var scopes []persistence.Scope
	servers := storedServers{by: map[shared.ID]domain.DocumentServerChoice{nordlicht: collabora}, scopes: &scopes}
	framing := FramedServer{
		Tenants: hosts{"nordlicht.hub.example": nordlicht, "boreal.hub.example": boreal}, Wopi: servers,
		UnitOfWork: scopedUnitOfWork{}, Multi: true, InstallationHost: "hub.example",
	}
	for _, c := range []struct{ host, want string }{
		{"Nordlicht.hub.example:443", "https://office.nordlicht.example"},
		{"boreal.hub.example", ""},
		{"hub.example", ""},
		{"unknown.hub.example", ""},
	} {
		if got, err := framing.OriginFor(t.Context(), c.host); err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.host, got, err, c.want)
		}
	}
	if len(scopes) != 2 || scopes[0].TenantID != nordlicht || scopes[1].TenantID != boreal {
		t.Errorf("read in scopes %+v, want each workspace's own", scopes)
	}

	servers.by[nordlicht] = domain.DocumentServerChoice{Kind: domain.DocumentServerNone}
	if got, _ := framing.OriginFor(t.Context(), "nordlicht.hub.example"); got != "" {
		t.Errorf("a removed server still opens %q", got)
	}

	single := FramedServer{Tenants: hosts{"": boreal}, Wopi: storedServers{by: map[shared.ID]domain.DocumentServerChoice{boreal: collabora}, scopes: &scopes}, UnitOfWork: scopedUnitOfWork{}}
	if got, _ := single.OriginFor(t.Context(), "localhost:8080"); got != "https://office.nordlicht.example" {
		t.Errorf("single mode: %q", got)
	}
}

// scopedUnitOfWork hands the scope to the repository through the context, as the adapter's
// transaction carries its tenant.
type scopedUnitOfWork struct{}

type scopeKey struct{}

func (scopedUnitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, scopeKey{}, scope))
}

func (scopedUnitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, scopeKey{}, scope))
}

func scopeOf(ctx context.Context) persistence.Scope {
	scope, _ := ctx.Value(scopeKey{}).(persistence.Scope)
	return scope
}
