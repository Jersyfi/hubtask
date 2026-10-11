// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// privateRoles is heldRoles with the hubs' privacy, as storage answers it.
type privateRoles struct {
	heldRoles
	private map[shared.ID]bool
}

func (m *privateRoles) HubsOf(_ context.Context, ids []shared.ID) (map[shared.ID]identityrepository.Hub, error) {
	hubs := map[shared.ID]identityrepository.Hub{}
	for _, id := range ids {
		if _, known := m.private[id]; known {
			hubs[id] = identityrepository.Hub{ID: id, Private: m.private[id]}
		}
	}
	return hubs, nil
}

type lockedHubs struct {
	groups bool
	locked int
}

func (l *lockedHubs) LockHubOf(_ context.Context, id shared.ID) (identityrepository.Hub, bool, error) {
	l.locked++
	return identityrepository.Hub{ID: id}, true, nil
}

func (l *lockedHubs) GroupHoldsRoleUnder(context.Context, shared.ID) (bool, error) {
	return l.groups, nil
}

type privacyRevoker struct{ announced []access.Loss }

func (r *privacyRevoker) AfterHubMadePrivate(_ context.Context, hub domain.Container) (access.Loss, error) {
	return access.Loss{Accounts: []shared.ID{accountID}, Scope: identity.HubScope(hub.ID)}, nil
}

func (r *privacyRevoker) Announce(_ context.Context, _ shared.ID, losses ...access.Loss) error {
	r.announced = append(r.announced, losses...)
	return nil
}

type privacyHarness struct {
	*containerHarness
	registry *usecase.Registry
	locks    *lockedHubs
	revoker  *privacyRevoker
}

func newPrivacyHarness(private bool, held ...identity.Membership) *privacyHarness {
	h := &privacyHarness{
		containerHarness: newContainerHarness(), locks: &lockedHubs{}, revoker: &privacyRevoker{},
	}
	hub := h.withHub(hubID, "Presents")
	hub.Private = private
	h.containers.stored[hubID] = hub
	h.withCollection()

	roles := &privateRoles{heldRoles: heldRoles{held: held}, private: map[shared.ID]bool{hubID: private}}
	h.writer.Authorizer = access.Service{
		Memberships: roles, UnitOfWork: h.uow, Audit: h.audit, Clock: clock.Fixed(now),
	}
	registry, err := usecase.NewRegistry(nil, SetHubPrivacy{
		Writer: h.writer, Hubs: h.locks, Revocations: h.revoker,
	}.Descriptor())
	if err != nil {
		panic(err)
	}
	h.registry = registry
	return h
}

func (h *privacyHarness) set(id shared.ID, private bool) (usecase.Output, error) {
	return h.registry.Invoke(context.Background(), SetHubPrivacyName, actor(),
		usecase.Input{"container_id": id.String(), "private": private})
}

var (
	workspaceAdmin  = identity.Membership{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleAdmin}
	workspaceMember = identity.Membership{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleMember}
	hubOwner        = identity.Membership{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleOwner}
)

// ADR-0073 §2 read literally (D3): marking takes STRUCTURE on the workspace and OWNER on the hub
// itself; the change is announced as a policy change and every device that lost the hub is told.
func TestAnOwnerWhoMayCreateHubsMarksTheirHubPrivate(t *testing.T) {
	h := newPrivacyHarness(false, workspaceAdmin, hubOwner)

	out, err := h.set(hubID, true)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if out["private"] != true || !h.containers.stored[hubID].Private {
		t.Fatalf("the hub is not private: %v", out)
	}
	if h.locks.locked != 1 {
		t.Errorf("the hub's row was locked %d times, want once", h.locks.locked)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerPoliciesUpdated {
		t.Fatalf("announced %+v, want one policies_updated", h.events.appended)
	}
	if len(h.revoker.announced) != 1 {
		t.Errorf("the devices were told %+v, want one loss", h.revoker.announced)
	}
	if len(h.changes.recorded) != 1 || h.changes.recorded[0].Payload["private"] != true {
		t.Errorf("the change log says %+v, want private: true", h.changes.recorded)
	}
	found := false
	for _, entry := range h.audit.entries {
		if entry.Action == HubPrivacyChangedAction {
			found = true
		}
	}
	if !found {
		t.Errorf("no %s in the trail: %+v", HubPrivacyChangedAction, h.audit.entries)
	}

	// Again: nothing moves, nothing is announced.
	if _, err := h.set(hubID, true); err != nil || len(h.events.appended) != 1 {
		t.Errorf("marking twice: %v, %d events", err, len(h.events.appended))
	}
}

func TestWhoMayNotMarkAHubPrivate(t *testing.T) {
	cases := []struct {
		name    string
		private bool
		want    bool
		held    []identity.Membership
		groups  bool
		target  shared.ID
		kind    error
		detail  string
	}{
		{name: "a workspace administrator who does not own the hub", want: true,
			held: []identity.Membership{workspaceAdmin}, target: hubID, kind: shared.ErrForbidden,
			detail: "access.not_permitted"},
		{name: "the hub's owner without the right to create hubs", want: true,
			held: []identity.Membership{workspaceMember, hubOwner}, target: hubID, kind: shared.ErrForbidden,
			detail: "access.not_permitted"},
		{name: "a hub a group holds a role in", want: true, groups: true,
			held: []identity.Membership{workspaceAdmin, hubOwner}, target: hubID, kind: shared.ErrConflict,
			detail: "containers.private_hub_has_groups"},
		{name: "a collection", want: true,
			held: []identity.Membership{workspaceAdmin, hubOwner}, target: shoppingID, kind: shared.ErrValidation,
			detail: "containers.private_only_hubs"},
		{name: "a workspace administrator sharing a private hub they are not in", private: true, want: false,
			held: []identity.Membership{workspaceAdmin}, target: hubID, kind: shared.ErrNotFound,
			detail: "containers.not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newPrivacyHarness(tc.private, tc.held...)
			h.locks.groups = tc.groups

			_, err := h.set(tc.target, tc.want)
			if !errors.Is(err, tc.kind) || shared.AsError(err).DetailCode != tc.detail {
				t.Fatalf("error %v, want %s", err, tc.detail)
			}
			if len(h.containers.written) != 0 || len(h.revoker.announced) != 0 {
				t.Error("a refused change was written")
			}
		})
	}
}

// Sharing it again needs only OWNER on the hub, and tells no device anything: nobody lost it.
func TestTheOwnerSharesTheirHubAgain(t *testing.T) {
	h := newPrivacyHarness(true, workspaceMember, hubOwner)

	out, err := h.set(hubID, false)
	if err != nil || out["private"] != false {
		t.Fatalf("refused: %v (%v)", err, out)
	}
	if len(h.revoker.announced) != 0 {
		t.Errorf("sharing announced a loss: %+v", h.revoker.announced)
	}
}
