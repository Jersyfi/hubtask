// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	membershipID = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	groupID      = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")
	hubID        = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
)

// grantStore is the write half of the membership table, in memory.
type grantStore struct {
	byID    map[shared.ID]domain.Grant
	granted []domain.Grant
	revoked []shared.ID

	listedAt   domain.Scope
	listedPage repository.Page
}

func newGrants(existing ...domain.Grant) *grantStore {
	store := &grantStore{byID: map[shared.ID]domain.Grant{}}
	for _, grant := range existing {
		store.byID[grant.ID] = grant
	}
	return store
}

func (s *grantStore) Grant(_ context.Context, grant domain.Grant) error {
	s.byID[grant.ID] = grant
	s.granted = append(s.granted, grant)
	return nil
}

func (s *grantStore) Revoke(_ context.Context, id shared.ID) (bool, error) {
	if _, found := s.byID[id]; !found {
		return false, nil
	}
	delete(s.byID, id)
	s.revoked = append(s.revoked, id)
	return true, nil
}

func (s *grantStore) Find(_ context.Context, id shared.ID) (domain.Grant, error) {
	grant, found := s.byID[id]
	if !found {
		return domain.Grant{}, shared.ErrNotFound.WithDetail("memberships.not_found")
	}
	return grant, nil
}

// ListAt answers what is granted at exactly the scope asked, newest first by identifier, and
// remembers what it was asked so that a test can see the scope and the page.
func (s *grantStore) ListAt(_ context.Context, scope domain.Scope, page repository.Page) (repository.GrantPage, error) {
	s.listedAt, s.listedPage = scope, page
	var grants []domain.Grant
	for _, grant := range s.byID {
		if grant.Scope == scope {
			grants = append(grants, grant)
		}
	}
	slices.SortFunc(grants, func(a, b domain.Grant) int { return strings.Compare(string(b.ID), string(a.ID)) })
	return repository.GrantPage{Grants: grants}, nil
}

func (s *grantStore) OfGroup(_ context.Context, groupID shared.ID) ([]domain.Grant, error) {
	var grants []domain.Grant
	for _, grant := range s.byID {
		if grant.GroupID == groupID {
			grants = append(grants, grant)
		}
	}
	slices.SortFunc(grants, func(a, b domain.Grant) int { return strings.Compare(string(a.ID), string(b.ID)) })
	return grants, nil
}

var _ repository.MembershipGrants = (*grantStore)(nil)

func grantHandler(grants *grantStore, accounts *accountStore, groups *groupStore, auth *authorizer, sink *auditSink) GrantMembership {
	return GrantMembership{
		Grants: grants, Accounts: accounts, Groups: groups, Hubs: &hubLocks{}, Authorizer: auth, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: membershipID},
	}
}

// hubLocks answers which hubs are private, and records which it locked.
type hubLocks struct {
	private map[shared.ID]bool
	locked  []shared.ID
}

func (h *hubLocks) LockHubOf(_ context.Context, id shared.ID) (repository.Hub, bool, error) {
	h.locked = append(h.locked, id)
	return repository.Hub{ID: id, Private: h.private[id]}, true, nil
}

func (h *hubLocks) GroupHoldsRoleUnder(context.Context, shared.ID) (bool, error) { return false, nil }

// A group inside a private hub would be the workspace's administrators' way in (identity.md §22):
// refused, under the hub's lock. A person may be given a role there, and a group anywhere else.
func TestAGroupIsRefusedARoleInAPrivateHub(t *testing.T) {
	group := domain.Group{ID: groupID, TenantID: tenant, Name: "Parents"}
	cases := []struct {
		name    string
		cmd     GrantMembershipCommand
		private bool
		refused bool
	}{
		{"a group on a private hub", GrantMembershipCommand{GroupID: groupID, Scope: domain.HubScope(hubID), Role: domain.RoleViewer}, true, true},
		{"a group on a collection of a private hub", GrantMembershipCommand{GroupID: groupID, Scope: domain.CollectionScope(hubID), Role: domain.RoleViewer}, true, true},
		{"a group on a shared hub", GrantMembershipCommand{GroupID: groupID, Scope: domain.HubScope(hubID), Role: domain.RoleViewer}, false, false},
		{"a person on a private hub", GrantMembershipCommand{AccountID: invitedID, Scope: domain.HubScope(hubID), Role: domain.RoleViewer}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grants := newGrants()
			handler := grantHandler(grants, newAccounts(invitedAccount(t)), newGroups(group), &authorizer{}, &auditSink{})
			locks := &hubLocks{private: map[shared.ID]bool{hubID: tc.private}}
			handler.Hubs = locks

			_, err := handler.Execute(t.Context(), admin(), tc.cmd)
			if !tc.refused {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || shared.AsError(err).DetailCode != "memberships.group_in_private_hub" {
				t.Fatalf("error %v, want memberships.group_in_private_hub", err)
			}
			if !errors.Is(err, shared.ErrValidation) {
				t.Errorf("error %v, want a validation error (422)", err)
			}
			if len(grants.granted) != 0 || len(locks.locked) != 1 {
				t.Errorf("granted %v, locked %v; want nothing granted after one lock", grants.granted, locks.locked)
			}
		})
	}
}

func invitedAccount(t *testing.T) domain.Account {
	t.Helper()
	account, err := domain.Invite(invitedID, tenant, "bert@example.org", "Bert", nil, nil)
	if err != nil {
		t.Fatalf("preparing the account: %v", err)
	}
	return account
}

func TestARoleIsGrantedToAnAccountAndRecorded(t *testing.T) {
	grants, accounts, sink := newGrants(), newAccounts(invitedAccount(t)), &auditSink{}

	grant, err := grantHandler(grants, accounts, newGroups(), &authorizer{}, sink).
		Execute(t.Context(), admin(), GrantMembershipCommand{
			AccountID: invitedID, Scope: domain.HubScope(hubID), Role: domain.RoleMember,
		})
	if err != nil {
		t.Fatalf("granting: %v", err)
	}

	if grant.Role != domain.RoleMember || grant.Scope.ID != hubID {
		t.Errorf("granted %+v", grant)
	}
	if len(grants.granted) != 1 {
		t.Fatalf("%d grants written, want one", len(grants.granted))
	}
	if len(sink.entries) != 1 || sink.entries[0].Severity != audit.SeverityNotice {
		t.Errorf("audit entries %v, want one notice - this is what an access review reads", sink.entries)
	}
}

// The permission is decided at the scope being granted, not at the tenant: an administrator of one
// hub may hand out roles inside it and nowhere else.
func TestTheGrantIsAuthorisedAtItsOwnScope(t *testing.T) {
	auth := &authorizer{}

	if _, err := grantHandler(newGrants(), newAccounts(invitedAccount(t)), newGroups(), auth, &auditSink{}).
		Execute(t.Context(), admin(), GrantMembershipCommand{
			AccountID: invitedID, Scope: domain.HubScope(hubID), Role: domain.RoleAdmin,
		}); err != nil {
		t.Fatalf("granting: %v", err)
	}

	if len(auth.requests) != 1 {
		t.Fatalf("%d authorisation requests", len(auth.requests))
	}
	path := auth.requests[0].Path
	if len(path) != 2 || path[0].Type != domain.ScopeTenant || path[1].ID != hubID {
		t.Errorf("the path walked was %v, want the tenant and then the hub", path)
	}
}

// Granting to somebody who is not here says so, rather than surfacing as a foreign key violation
// from three layers down.
func TestGrantingToAnAccountThatIsNotHereIsNotFound(t *testing.T) {
	grants := newGrants()

	_, err := grantHandler(grants, newAccounts(), newGroups(), &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), GrantMembershipCommand{
			AccountID: invitedID, Scope: domain.TenantScope(), Role: domain.RoleMember,
		})

	if err == nil || shared.AsError(err).DetailCode != "accounts.not_found" {
		t.Fatalf("error %v, want the account not found", err)
	}
	if len(grants.granted) != 0 {
		t.Error("a grant was written for an account that does not exist")
	}
}

// The domain's rule, reached through the use case: exactly one subject.
func TestAGrantWithoutExactlyOneSubjectIsRefusedBeforeAnyPermissionIsChecked(t *testing.T) {
	auth := &authorizer{}

	_, err := grantHandler(newGrants(), newAccounts(), newGroups(), auth, &auditSink{}).
		Execute(t.Context(), admin(), GrantMembershipCommand{
			AccountID: invitedID, GroupID: groupID, Scope: domain.TenantScope(), Role: domain.RoleMember,
		})

	if err == nil || shared.AsError(err).DetailCode != "memberships.subject_ambiguous" {
		t.Fatalf("error %v, want the grant refused", err)
	}
	if len(auth.requests) != 0 {
		t.Error("a malformed grant reached the authorisation service")
	}
}

// revoker notes what the use cases say was lost, and what they had announced; the deciding is
// access.Revocations' and is tested there.
type revoker struct {
	grants    []domain.Grant
	groupLoss map[shared.ID][]shared.ID
	announced []access.Loss
}

func (r *revoker) AfterGrantRevoked(_ context.Context, grant domain.Grant) (access.Loss, error) {
	r.grants = append(r.grants, grant)
	return access.Loss{Accounts: []shared.ID{grant.AccountID}, Scope: grant.Scope}, nil
}

func (r *revoker) AfterGroupLoss(_ context.Context, groupID shared.ID, accounts []shared.ID) ([]access.Loss, error) {
	if r.groupLoss == nil {
		r.groupLoss = map[shared.ID][]shared.ID{}
	}
	r.groupLoss[groupID] = append(r.groupLoss[groupID], accounts...)
	if len(accounts) == 0 {
		return nil, nil
	}
	return []access.Loss{{Accounts: accounts, Scope: domain.HubScope(hubID)}}, nil
}

func (r *revoker) Announce(_ context.Context, _ shared.ID, losses ...access.Loss) error {
	r.announced = append(r.announced, losses...)
	return nil
}

func revokeHandler(grants *grantStore, auth *authorizer, sink *auditSink) RevokeMembership {
	return RevokeMembership{
		Grants: grants, Authorizer: auth, Revocations: &revoker{}, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

func existingGrant(t *testing.T) domain.Grant {
	t.Helper()
	grant, err := domain.NewGrant(membershipID, tenant, invitedID, "", domain.HubScope(hubID), domain.RoleMember)
	if err != nil {
		t.Fatalf("preparing the grant: %v", err)
	}
	return grant
}

func TestARevocationRemovesTheMembershipAndRecordsIt(t *testing.T) {
	grants, sink := newGrants(existingGrant(t)), &auditSink{}

	if err := revokeHandler(grants, &authorizer{}, sink).
		Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: membershipID}); err != nil {
		t.Fatalf("revoking: %v", err)
	}

	if len(grants.revoked) != 1 {
		t.Fatalf("%d revocations, want one", len(grants.revoked))
	}
	entry := sink.entries[0]
	if entry.Action != MembershipRevokedAction {
		t.Errorf("action %q", entry.Action)
	}
	// The same fields as the grant, so that the pair of one access review reads as a pair.
	if entry.Changes["role"] == nil || entry.Changes["scope_type"] == nil {
		t.Errorf("the entry records %v, want what was taken away", entry.Changes)
	}
}

// The devices of whoever held the role are told what they lost, once the row is gone.
func TestARevocationIsAnnouncedToWhoeverHeldTheRole(t *testing.T) {
	grants, told := newGrants(existingGrant(t)), &revoker{}
	handler := revokeHandler(grants, &authorizer{}, &auditSink{})
	handler.Revocations = told

	if err := handler.Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: membershipID}); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if len(told.grants) != 1 || told.grants[0].ID != membershipID {
		t.Errorf("the loss described %v, want the grant revoked", told.grants)
	}
	if len(told.announced) != 1 || told.announced[0].Accounts[0] != invitedID || told.announced[0].Scope != domain.HubScope(hubID) {
		t.Errorf("announced %v, want the account at the hub", told.announced)
	}
}

type lastMember struct{ named [][]shared.ID }

func (l *lastMember) AfterMemberLeft(_ context.Context, _ shared.ID, named []shared.ID) (int, error) {
	l.named = append(l.named, named)
	return 0, nil
}

// UC-ID-16 check 6: a person's grant revoked asks whether the hub it was on has a member left; a
// workspace grant cannot leave a private hub empty, and is not asked about.
func TestARevocationAsksWhetherTheHubHasAMemberLeft(t *testing.T) {
	last := &lastMember{}
	handler := revokeHandler(newGrants(existingGrant(t)), &authorizer{}, &auditSink{})
	handler.LastMember = last
	if err := handler.Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: membershipID}); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if len(last.named) != 1 || len(last.named[0]) != 1 || last.named[0][0] != hubID {
		t.Errorf("asked about %v, want the hub the grant was on", last.named)
	}

	workspace, err := domain.NewGrant(membershipID, tenant, invitedID, "", domain.TenantScope(), domain.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	last = &lastMember{}
	handler = revokeHandler(newGrants(workspace), &authorizer{}, &auditSink{})
	handler.LastMember = last
	if err := handler.Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: membershipID}); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if len(last.named) != 0 {
		t.Errorf("a workspace grant asked about %v", last.named)
	}
}

// Revoking what is not there answers not found rather than reporting a removal that did not
// happen.
func TestRevokingSomethingThatIsNotThereIsNotFound(t *testing.T) {
	err := revokeHandler(newGrants(), &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: membershipID})

	if err == nil || !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
}

// The membership is read before the permission is decided, because the scope it is at is what the
// permission is decided against - and a refusal must not remove anything.
func TestARefusedRevocationRemovesNothing(t *testing.T) {
	grants := newGrants(existingGrant(t))
	auth := &authorizer{refuse: shared.ErrForbidden.WithDetail("access.not_permitted")}

	err := revokeHandler(grants, auth, &auditSink{}).
		Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: membershipID})

	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want the refusal", err)
	}
	if len(grants.revoked) != 0 {
		t.Error("a refused revocation removed the membership")
	}
	if len(auth.requests) != 1 || auth.requests[0].Path[1].ID != hubID {
		t.Errorf("authorised against %v, want the scope the membership is at", auth.requests)
	}
}
