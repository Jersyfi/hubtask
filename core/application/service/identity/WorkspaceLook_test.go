// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var guestsHub = shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")

// placedMemberships holds roles at the places a test names: a guest of one hub holds nothing at the
// workspace, so READ there is refused while a role somewhere still makes them a member. holds is
// what the store answers for "holds a role anywhere"; a requester's REQUEST is no such role.
type placedMemberships struct {
	membershipsFake
	held  []domain.Membership
	holds bool
}

func (m placedMemberships) Along(context.Context, shared.ID, []domain.Scope) ([]domain.Membership, error) {
	return m.held, nil
}

func (m placedMemberships) HoldsAny(context.Context, shared.ID) (bool, error) { return m.holds, nil }

// lookRegistry is the look's read beside the workspace's, over the workspace fixture, under the
// authoriser the test hands it.
func lookRegistry(t *testing.T, b *branding) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil,
		ReadWorkspace{Writer: b.policy.writer}.Descriptor(),
		ReadWorkspaceLook{Writer: b.policy.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return registry
}

// UC-INS-22 checks 5 and 9 name every member: the look is read by whoever holds a role anywhere in
// the workspace - a guest of one hub among them, who may not read the workspace's set-up - and by
// nobody who holds none, through the registry as every door asks (rule 2).
func TestEveryMemberReadsTheWorkspacesLookAndNobodyElse(t *testing.T) {
	atWorkspace := func(role domain.Role) []domain.Membership {
		return []domain.Membership{{Role: role, Scope: domain.TenantScope()}}
	}
	cases := []struct {
		name     string
		held     []domain.Membership
		holds    bool
		kind     shared.ActorKind
		scopes   []string
		allowed  bool
		readsAll bool
	}{
		{"an owner", atWorkspace(domain.RoleOwner), true, shared.ActorUser, nil, true, true},
		{"a member", atWorkspace(domain.RoleMember), true, shared.ActorUser, nil, true, true},
		{"a viewer", atWorkspace(domain.RoleViewer), true, shared.ActorUser, nil, true, true},
		{"an auditor", atWorkspace(domain.RoleAuditor), true, shared.ActorUser, nil, true, true},
		{"a service account as member", atWorkspace(domain.RoleMember), true, shared.ActorServiceAccount, nil, true, true},
		{"a guest of one hub", []domain.Membership{{Role: domain.RoleGuest, Scope: domain.HubScope(guestsHub)}}, true, shared.ActorUser, nil, true, false},
		{"a member of one hub", []domain.Membership{{Role: domain.RoleMember, Scope: domain.HubScope(guestsHub)}}, true, shared.ActorUser, nil, true, false},
		{"an account holding nothing", nil, false, shared.ActorUser, nil, false, false},
		{"a requester", nil, false, shared.ActorUser, nil, false, false},
		{"a member's token without workspace:read", atWorkspace(domain.RoleMember), true, shared.ActorUser, []string{"items:read"}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &auditSink{}
			authoriser := access.Service{
				Memberships: placedMemberships{held: tc.held, holds: tc.holds},
				UnitOfWork:  &unitOfWork{}, Audit: sink, Clock: clock.Fixed(now),
			}
			b := newBranding(t, authoriser)
			b.policy.store.row.Branding = domain.Branding{Header: domain.HeaderWorkspaceName, Symbol: squareMark}
			registry := lookRegistry(t, b)
			actor := workspaceActor()
			actor.Kind = tc.kind
			if tc.scopes != nil {
				actor.Scopes = tc.scopes
			}

			look, err := registry.Invoke(t.Context(), ReadWorkspaceLookName, actor, usecase.Input{})
			if tc.allowed {
				if err != nil {
					t.Fatalf("the look was refused: %v", err)
				}
				if look["header"] != "WORKSPACE_NAME" || look["symbol_media_id"] != squareMark.String() ||
					look["display_name"] != b.policy.store.row.DisplayName {
					t.Errorf("the look answered %v", look)
				}
			} else if !errors.Is(err, shared.ErrForbidden) {
				t.Errorf("the look answered %v, want forbidden", err)
			}

			_, setUp := registry.Invoke(t.Context(), ReadWorkspaceName, actor, usecase.Input{})
			if tc.readsAll != (setUp == nil) {
				t.Errorf("the workspace's set-up answered %v; this caller reads it: %v", setUp, tc.readsAll)
			}
		})
	}
}

// Nothing beyond the name, the choice and the picture identifiers leaves the look's read; where the
// workspace may not show its own look, it is the product's and names no picture (ADR-0112).
func TestTheLookCarriesTheNameTheChoiceAndThePicturesOnly(t *testing.T) {
	b := newBranding(t, nil)
	b.policy.store.row.Branding = domain.Branding{
		Header: domain.HeaderLogo, LogoLight: lightLogo, LogoDark: darkLogo, Symbol: squareMark,
	}
	registry := lookRegistry(t, b)

	look, err := registry.Invoke(t.Context(), ReadWorkspaceLookName, workspaceActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	keys := make([]string, 0, len(look))
	for key := range look {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"display_name", "header", "logo_dark_media_id", "logo_light_media_id", "symbol_media_id"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("the look carries %v, want %v", keys, want)
	}
	if look["header"] != "LOGO" || look["logo_light_media_id"] != lightLogo.String() ||
		look["logo_dark_media_id"] != darkLogo.String() || look["symbol_media_id"] != squareMark.String() {
		t.Errorf("the look answered %v", look)
	}

	b.policy.instance.level.Policy.Patch = domain.PolicyPatch{OwnBranding: boolOf(false)}
	off, err := registry.Invoke(t.Context(), ReadWorkspaceLookName, workspaceActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading switched off: %v", err)
	}
	if off["header"] != "PRODUCT" || off["logo_light_media_id"] != nil || off["logo_dark_media_id"] != nil ||
		off["symbol_media_id"] != nil || off["display_name"] != b.policy.store.row.DisplayName {
		t.Errorf("switched off above, the look answered %v, want the product's and no picture", off)
	}
}
