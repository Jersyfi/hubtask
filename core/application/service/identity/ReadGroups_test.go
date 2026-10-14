// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

func namedGroup(t *testing.T, id shared.ID, name string) domain.Group {
	t.Helper()
	group, err := domain.NewGroup(domain.NewGroupInput{ID: id, TenantID: tenant, Name: name})
	if err != nil {
		t.Fatalf("preparing the group: %v", err)
	}
	return group
}

// Any member reads the groups: no permission question is asked, only the credential's scope.
func TestTheGroupsAreListedForAnyMemberWithTheScope(t *testing.T) {
	groups := newGroups(
		namedGroup(t, groupID, "Leads"),
		namedGroup(t, "01936f2a-7c1e-7000-8000-0000000000c2", "Backoffice"),
	)

	page, err := ListGroups{Groups: groups, UnitOfWork: &unitOfWork{}}.
		Execute(t.Context(), reader(), ListGroupsQuery{Size: 500})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	if len(page.Groups) != 2 {
		t.Errorf("listed %d groups, want two", len(page.Groups))
	}
	if groups.listedPage.Size != maxPageSize {
		t.Errorf("page size %d, want the clamp", groups.listedPage.Size)
	}
}

func TestACredentialWithoutTheScopeListsNoGroups(t *testing.T) {
	groups := newGroups(namedGroup(t, groupID, "Leads"))

	_, err := ListGroups{Groups: groups, UnitOfWork: &unitOfWork{}}.
		Execute(t.Context(), admin(), ListGroupsQuery{})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("got %v, want the scope refusal", err)
	}
	if groups.listedPage.Size != 0 {
		t.Error("the list was read for a credential that may not read it")
	}
}

func TestAGroupIsReadWithItsMembers(t *testing.T) {
	groups := newGroups(namedGroup(t, groupID, "Leads"))
	groups.members[groupID] = []shared.ID{invitedID, adminID}

	detail, err := GetGroup{Groups: groups, UnitOfWork: &unitOfWork{}}.Execute(t.Context(), reader(), groupID)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	if detail.Group.Name != "Leads" || len(detail.Members) != 2 {
		t.Errorf("read %+v", detail)
	}
}

func TestAGroupThatIsNotThereIsNotFoundByIdentifier(t *testing.T) {
	_, err := GetGroup{Groups: newGroups(), UnitOfWork: &unitOfWork{}}.Execute(t.Context(), reader(), groupID)

	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("got %v, want not found", err)
	}
	if shared.AsError(err).Params["group_id"] != groupID.String() {
		t.Errorf("the answer does not name the group: %v", err)
	}
}

func TestTheGroupReadsAreDeclaredAsReads(t *testing.T) {
	for _, descriptor := range []usecase.Descriptor{ListGroups{}.Descriptor(), GetGroup{}.Descriptor()} {
		if !descriptor.ReadOnly || descriptor.TokenScope != membersRead {
			t.Errorf("%s is declared %+v", descriptor.Name, descriptor)
		}
	}
	if err := (ListGroups{}).Descriptor().ValidateInput(usecase.Input{"cursor": "abc", "size": 10}); err != nil {
		t.Errorf("the controller's input is refused: %v", err)
	}
	if err := (GetGroup{}).Descriptor().ValidateInput(usecase.Input{"group_id": groupID.String()}); err != nil {
		t.Errorf("the controller's input is refused: %v", err)
	}
}

// The catalogue's list shape carries the members as strings, which is what every channel
// serialises: an MCP tool and an automation action read the same document.
func TestTheGroupOutputCarriesTheMembersAsIdentifiers(t *testing.T) {
	groups := newGroups(namedGroup(t, groupID, "Leads"))
	groups.members[groupID] = []shared.ID{invitedID}

	out, err := GetGroup{Groups: groups, UnitOfWork: &unitOfWork{}}.
		invoke(t.Context(), reader(), usecase.Input{"group_id": groupID.String()})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	members, _ := out["members"].([]string)
	if len(members) != 1 || members[0] != invitedID.String() {
		t.Errorf("members %v", out["members"])
	}
}

// byDepth answers grantable roles by how far down the path runs: the workspace, or a hub below it.
type byDepth struct {
	atWorkspace, atHub []domain.Role
	requests           []access.Request
}

func (b *byDepth) GrantableRoles(_ context.Context, _ appshared.ActorContext, request access.Request) ([]domain.Role, error) {
	b.requests = append(b.requests, request)
	if len(request.Path) > 1 {
		return b.atHub, nil
	}
	return b.atWorkspace, nil
}

// The group tells the reader whether they may change who is in it, by the condition UpdateGroup and
// DeleteGroup refuse on (domain-model.md §3.2, P-05): the right to manage members on the workspace,
// and every role the group holds within the reader's own where it applies.
func TestTheGroupSaysWhetherTheReaderMayChangeItsMembers(t *testing.T) {
	everyRole := domain.Roles()
	belowOwner := everyRole[1:]
	held := func(role domain.Role, scope domain.Scope) []domain.Grant {
		grant, err := domain.NewGrant(membershipID, tenant, "", groupID, scope, role)
		if err != nil {
			t.Fatalf("preparing: %v", err)
		}
		return []domain.Grant{grant}
	}

	for _, row := range []struct {
		name   string
		reader *byDepth
		held   []domain.Grant
		want   bool
	}{
		{"an owner, the group holds OWNER", &byDepth{atWorkspace: everyRole, atHub: everyRole}, held(domain.RoleOwner, domain.TenantScope()), true},
		{"an administrator, the group holds OWNER", &byDepth{atWorkspace: belowOwner, atHub: belowOwner}, held(domain.RoleOwner, domain.TenantScope()), false},
		{"an administrator, the group holds ADMIN on a hub", &byDepth{atWorkspace: belowOwner, atHub: belowOwner}, held(domain.RoleAdmin, domain.HubScope(hubID)), true},
		{"an administrator who owns the hub the group holds OWNER on", &byDepth{atWorkspace: belowOwner, atHub: everyRole}, held(domain.RoleOwner, domain.HubScope(hubID)), true},
		{"an administrator, the group holds nothing", &byDepth{atWorkspace: belowOwner}, nil, true},
		{"a member, the group holds nothing", &byDepth{}, nil, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			groups := newGroups(namedGroup(t, groupID, "Leads"))
			grants := newGrants(row.held...)
			out, err := GetGroup{Groups: groups, Grants: grants, Grantable: row.reader, UnitOfWork: &unitOfWork{}}.
				invoke(t.Context(), reader(), usecase.Input{"group_id": groupID.String()})
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			if out["members_changeable"] != row.want {
				t.Errorf("members_changeable %v, want %v", out["members_changeable"], row.want)
			}
			for _, request := range row.reader.requests {
				if request.TokenScope != membersWrite || request.Permission != service.PermissionManageMembers {
					t.Errorf("asked %+v, want the update's own request", request)
				}
			}
		})
	}

	// Wired without the answer, nothing is offered.
	out, err := GetGroup{Groups: newGroups(namedGroup(t, groupID, "Leads")), UnitOfWork: &unitOfWork{}}.
		invoke(t.Context(), reader(), usecase.Input{"group_id": groupID.String()})
	if err != nil || out["members_changeable"] != false {
		t.Errorf("unwired: %v (%v), want false", out["members_changeable"], err)
	}
}
