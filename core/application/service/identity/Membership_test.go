// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
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

var _ repository.MembershipGrants = (*grantStore)(nil)

func grantHandler(grants *grantStore, accounts *accountStore, groups *groupStore, auth *authorizer, sink *auditSink) GrantMembership {
	return GrantMembership{
		Grants: grants, Accounts: accounts, Groups: groups, Authorizer: auth, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: membershipID},
	}
}

func invitedAccount(t *testing.T) domain.Account {
	t.Helper()
	account, err := domain.Invite(invitedID, tenant, "bert@example.org", "Bert")
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

func revokeHandler(grants *grantStore, auth *authorizer, sink *auditSink) RevokeMembership {
	return RevokeMembership{
		Grants: grants, Authorizer: auth, Audit: sink,
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
