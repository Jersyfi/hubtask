// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// otherAdmin is a second administrator of the workspace, who must not reach a private hub.
var otherAdmin = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")

// grantedRoles is the membership store as the real authoriser reads it, with what the use case
// grants in its transaction counted at once - as PostgreSQL does inside the one transaction - and
// each hub's privacy read from the row the use case wrote.
type grantedRoles struct {
	held    []identity.Membership
	grants  *grantsMade
	written *containers
}

func (m *grantedRoles) of(account shared.ID) []identity.Membership {
	var out []identity.Membership
	for _, held := range m.held {
		if held.AccountID == account {
			out = append(out, held)
		}
	}
	for _, grant := range m.grants.granted {
		if grant.AccountID == account {
			out = append(out, identity.Membership{AccountID: account, Scope: grant.Scope, Role: grant.Role})
		}
	}
	return out
}

func (m *grantedRoles) Along(_ context.Context, account shared.ID, _ []identity.Scope) ([]identity.Membership, error) {
	return m.of(account), nil
}

func (m *grantedRoles) SharedItemsIn(context.Context, shared.ID, shared.ID) ([]shared.ID, error) {
	return nil, nil
}

func (m *grantedRoles) Administrators(context.Context, []identity.Scope) ([]shared.ID, error) {
	return nil, nil
}

func (m *grantedRoles) HubsOpenedFor(context.Context, shared.ID, []shared.ID, time.Time) ([]shared.ID, error) {
	return nil, nil
}

func (m *grantedRoles) HubsOf(_ context.Context, ids []shared.ID) (map[shared.ID]identityrepository.Hub, error) {
	out := map[shared.ID]identityrepository.Hub{}
	for _, row := range m.written.inserted {
		for _, id := range ids {
			if row.ID == id {
				hub := row.ID
				if row.Type == domain.ContainerCollection {
					hub = row.ParentID
				}
				out[id] = identityrepository.Hub{ID: hub, Private: m.private(hub)}
			}
		}
	}
	return out, nil
}

func (m *grantedRoles) private(hub shared.ID) bool {
	for _, row := range m.written.inserted {
		if row.ID == hub {
			return row.Private
		}
	}
	return false
}

func (m *grantedRoles) HoldsAny(_ context.Context, account shared.ID) (bool, error) {
	return len(m.of(account)) > 0, nil
}

// askingTemplates takes a template as the configuration does, as far as authorisation goes: inside
// the transaction it asks the authoriser what taking a hub part asks - READ on the hub to find it,
// STRUCTURE along the workspace and the hub - and records what it found at that moment.
type askingTemplates struct {
	authoriser access.Service
	written    *containers
	refuseHub  bool

	steps []string
	hub   shared.ID
	// takerReached and otherReached are the two answers at the hub part: the creator's, and a
	// second administrator's, while the transaction runs.
	takerReached, otherReached error
	privateAtFirstRow          bool
}

func (a *askingTemplates) TakeWorkspacePartOnce(context.Context, appshared.ActorContext, StarterTemplate) error {
	a.steps = append(a.steps, "workspace")
	return nil
}

func (a *askingTemplates) TakeForHub(ctx context.Context, actor appshared.ActorContext, _ StarterTemplate, hub shared.ID) error {
	a.steps = append(a.steps, "hub")
	a.hub = hub
	a.privateAtFirstRow = len(a.written.inserted) > 0 && a.written.inserted[0].Private
	path := []identity.Scope{identity.TenantScope(), identity.HubScope(hub)}
	reach := func(who appshared.ActorContext) error {
		for _, permission := range []service.Permission{service.PermissionRead, service.PermissionStructure} {
			if err := a.authoriser.Authorize(ctx, who, access.Request{
				Permission: permission, Path: path, Action: "config.applied", TokenScope: containersWrite,
				TargetType: "config", TargetID: hub,
			}); err != nil {
				return err
			}
		}
		return nil
	}
	a.takerReached = reach(actor)
	other := actor
	other.AccountID = otherAdmin
	a.otherReached = reach(other)
	if a.refuseHub {
		return shared.ErrValidation.WithDetail("config.refused")
	}
	return a.takerReached
}

// storing finds what was inserted, as the transaction does: the first collection names the hub
// written a moment before as its parent.
type storing struct{ *containers }

func (s storing) Insert(ctx context.Context, container domain.Container) (domain.Container, error) {
	inserted, err := s.containers.Insert(ctx, container)
	if err == nil {
		s.stored[inserted.ID] = inserted
	}
	return inserted, err
}

// hubTemplateHarness is CreateHubFromTemplate behind the real authoriser and the registry, with a
// second administrator of the workspace beside whoever calls.
func hubTemplateHarness(held ...identity.Membership) (*usecase.Registry, *harness, *grantsMade, *askingTemplates) {
	h := newHarness()
	grants := &grantsMade{}
	roles := &grantedRoles{
		held:   append(held, identity.Membership{AccountID: otherAdmin, Scope: identity.TenantScope(), Role: identity.RoleAdmin}),
		grants: grants, written: h.containers,
	}
	authoriser := access.Service{Memberships: roles, UnitOfWork: h.uow, Audit: h.audit, Clock: clock.Fixed(now)}
	h.handler.Authorizer = authoriser
	h.handler.OwnHubs = authoriser
	h.handler.Grants = grants
	h.handler.PrivateHubs = privateHubsSwitch{}
	h.handler.Containers = storing{h.containers}
	templates := &askingTemplates{authoriser: authoriser, written: h.containers}
	registry, err := usecase.NewRegistry(nil, CreateHubFromTemplate{Containers: h.handler, Templates: templates}.Descriptor())
	if err != nil {
		panic(err)
	}
	return registry, h, grants, templates
}

func templateActor() appshared.ActorContext {
	a := actor()
	a.Scopes = []string{"containers:write", "workspace:manage"}
	return a
}

func fromTemplate(private bool) usecase.Input {
	return usecase.Input{"name": "Support", "template": "itil_service_desk", "private": private}
}

func shapes(role identity.Role) identity.Membership {
	return identity.Membership{AccountID: accountID, Scope: identity.TenantScope(), Role: role}
}

// mayCreateSharedHubs is a custom role on the workspace carrying the right to create hubs and to
// read, nothing of an administrator's else.
var mayCreateSharedHubs = identity.Membership{
	AccountID: accountID, Scope: identity.TenantScope(), Custom: "hub_maker",
	Permissions: []string{string(service.PermissionRead), string(service.PermissionStructure)},
}

// UC-ID-16 check 9: whoever may shape the workspace makes a private hub from a template, and it is
// private from its first row - its creator its owner, another administrator of the workspace not
// reaching it even inside the transaction that makes it - with its first collection and the
// template's hub part taken into it.
func TestAPrivateHubFromATemplateIsPrivateFromItsFirstRow(t *testing.T) {
	for name, held := range map[string]identity.Membership{
		"the workspace's owner":       shapes(identity.RoleOwner),
		"an administrator":            shapes(identity.RoleAdmin),
		"a role that may create hubs": mayCreateSharedHubs,
	} {
		t.Run(name, func(t *testing.T) {
			registry, h, grants, templates := hubTemplateHarness(held)

			out, err := registry.Invoke(context.Background(), CreateHubFromTemplateName, templateActor(), fromTemplate(true))
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if len(h.containers.inserted) != 2 {
				t.Fatalf("%d containers written, want the hub and its first collection", len(h.containers.inserted))
			}
			hub, first := h.containers.inserted[0], h.containers.inserted[1]
			if out["private"] != true || !hub.Private || hub.Type != domain.ContainerHub {
				t.Fatalf("the hub was not written private: %v", out)
			}
			if first.Type != domain.ContainerCollection || first.ParentID != hub.ID || first.Name != hub.Name {
				t.Errorf("the first collection is %+v", first)
			}
			if !templates.privateAtFirstRow {
				t.Error("the hub's first row was not private")
			}
			if payload := h.events.appended[0].Payload; payload["private"] != true {
				t.Errorf("the hub's event says private = %v", payload["private"])
			}
			if len(grants.granted) != 1 || grants.granted[0].AccountID != accountID ||
				grants.granted[0].Role != identity.RoleOwner || grants.granted[0].Scope != identity.HubScope(hub.ID) {
				t.Fatalf("granted %+v, want OWNER on the new hub to its creator", grants.granted)
			}
			if len(templates.steps) != 2 || templates.steps[0] != "workspace" || templates.steps[1] != "hub" || templates.hub != hub.ID {
				t.Errorf("the template was taken as %v into %s", templates.steps, templates.hub)
			}
			if templates.takerReached != nil {
				t.Errorf("the creator did not reach the hub its template is taken into: %v", templates.takerReached)
			}
			// Not found, as a hub that is not there (T-04).
			if !errors.Is(templates.otherReached, shared.ErrNotFound) {
				t.Errorf("another administrator reached the private hub while it was made: %v", templates.otherReached)
			}
			if !h.uow.committed {
				t.Error("the transaction did not commit")
			}
		})
	}
}

// The same callers make a shared hub from a template as before: nobody is made its owner, and the
// workspace's other administrator reaches it.
func TestASharedHubFromATemplateIsUnchanged(t *testing.T) {
	registry, h, grants, templates := hubTemplateHarness(shapes(identity.RoleAdmin))
	if _, err := registry.Invoke(context.Background(), CreateHubFromTemplateName, templateActor(), fromTemplate(false)); err != nil {
		t.Fatal(err)
	}
	if h.containers.inserted[0].Private || len(grants.granted) != 0 {
		t.Errorf("a shared hub: private %v, granted %v", h.containers.inserted[0].Private, grants.granted)
	}
	if templates.otherReached != nil {
		t.Errorf("another administrator does not reach a shared hub: %v", templates.otherReached)
	}

	// An explicit null, as a door hands it on, is nothing chosen: a shared hub.
	registry, h, grants, _ = hubTemplateHarness(shapes(identity.RoleAdmin))
	in := fromTemplate(false)
	in["private"] = nil
	if _, err := registry.Invoke(context.Background(), CreateHubFromTemplateName, templateActor(), in); err != nil {
		t.Fatalf("private: null answered %v", err)
	}
	if h.containers.inserted[0].Private || len(grants.granted) != 0 {
		t.Error("private: null made a private hub")
	}
}

// UC-ID-16 check 9: `private` never lowers what a template asks. A person who
// may create only private hubs, a requester, a hub's own administrator and a machine are refused,
// shared or private, before anything is taken; and where private hubs are not allowed a private
// one is refused for whoever asks, while a shared one is still made.
func TestAHubFromATemplateIsRefusedWhereItMustBe(t *testing.T) {
	machine := templateActor()
	machine.Kind = appshared.ActorServiceAccount
	requester := identity.Membership{
		AccountID: accountID, Scope: identity.TenantScope(), Custom: "requester",
		Permissions: []string{string(service.PermissionRequest)},
	}
	hubAdmin := identity.Membership{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleAdmin}

	cases := []struct {
		name    string
		held    []identity.Membership
		actor   appshared.ActorContext
		private bool
		off     bool
		detail  string
		kind    error
	}{
		{"a member, who may create only private hubs, asking a private one", []identity.Membership{shapes(identity.RoleMember)},
			templateActor(), true, false, "access.not_permitted", shared.ErrForbidden},
		{"a member asking a shared one", []identity.Membership{shapes(identity.RoleMember)},
			templateActor(), false, false, "access.not_permitted", shared.ErrForbidden},
		{"a viewer asking a private one", []identity.Membership{shapes(identity.RoleViewer)},
			templateActor(), true, false, "access.not_permitted", shared.ErrForbidden},
		{"a requester asking a private one", []identity.Membership{requester},
			templateActor(), true, false, "access.not_permitted", shared.ErrForbidden},
		{"a requester asking a shared one", []identity.Membership{requester},
			templateActor(), false, false, "access.not_permitted", shared.ErrForbidden},
		{"a hub's administrator asking a private one", []identity.Membership{hubAdmin},
			templateActor(), true, false, "access.not_permitted", shared.ErrForbidden},
		{"a service account that may shape the workspace asking a private one", []identity.Membership{shapes(identity.RoleAdmin)},
			machine, true, false, "access.not_permitted", shared.ErrForbidden},
		{"an administrator where private hubs are off", []identity.Membership{shapes(identity.RoleAdmin)},
			templateActor(), true, true, "containers.private_hubs_off", shared.ErrForbidden},
		{"a member where private hubs are off", []identity.Membership{shapes(identity.RoleMember)},
			templateActor(), true, true, "access.not_permitted", shared.ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, h, grants, templates := hubTemplateHarness(tc.held...)
			h.handler.PrivateHubs = privateHubsSwitch{off: tc.off}
			registry, err := usecase.NewRegistry(nil, CreateHubFromTemplate{Containers: h.handler, Templates: templates}.Descriptor())
			if err != nil {
				t.Fatal(err)
			}

			_, err = registry.Invoke(context.Background(), CreateHubFromTemplateName, tc.actor, fromTemplate(tc.private))
			if !errors.Is(err, tc.kind) {
				t.Fatalf("error %v, want %v", err, tc.kind)
			}
			if got := shared.AsError(err).DetailCode; got != tc.detail {
				t.Errorf("detail %q, want %q", got, tc.detail)
			}
			if len(h.containers.inserted) != 0 || len(grants.granted) != 0 || len(templates.steps) != 0 {
				t.Errorf("a refused hub left %d rows, %d grants, template steps %v",
					len(h.containers.inserted), len(grants.granted), templates.steps)
			}
			// Refused before the hub's transaction: nothing was rolled back, and a refusal for a
			// permission is in the trail.
			if h.uow.rolledBack {
				t.Error("the refusal came from inside the hub's transaction")
			}
			if tc.detail == "access.not_permitted" && !deniedIn(h.audit) {
				t.Errorf("the refusal is not in the trail: %+v", h.audit.entries)
			}
		})
	}

	t.Run("a shared one where private hubs are off", func(t *testing.T) {
		registry, h, _, _ := hubTemplateHarness(shapes(identity.RoleAdmin))
		h.handler.PrivateHubs = privateHubsSwitch{off: true}
		if _, err := registry.Invoke(context.Background(), CreateHubFromTemplateName, templateActor(), fromTemplate(false)); err != nil {
			t.Errorf("a shared hub from a template where private hubs are off: %v", err)
		}
	})
}

func deniedIn(trail *sink) bool {
	for _, entry := range trail.entries {
		if entry.Outcome == audit.OutcomeDenied {
			return true
		}
	}
	return false
}

// UC-ID-16 check 9: a refused hub part refuses the private hub as a whole - the transaction that
// wrote the hub and its owner rolls back.
func TestAPrivateHubFromATemplateWhosePartIsRefusedIsNotMade(t *testing.T) {
	registry, h, _, templates := hubTemplateHarness(shapes(identity.RoleAdmin))
	templates.refuseHub = true

	_, err := registry.Invoke(context.Background(), CreateHubFromTemplateName, templateActor(), fromTemplate(true))
	if shared.AsError(err).DetailCode != "config.refused" {
		t.Fatalf("answered %v, want the part's refusal", err)
	}
	// The fake has nothing to undo; that the rows are gone with the rollback is proven against
	// PostgreSQL (test/integration).
	if !h.uow.rolledBack {
		t.Error("the hub's transaction was not rolled back")
	}
}
