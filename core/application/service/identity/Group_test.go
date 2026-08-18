// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// groupStore is the group table and its member links, in memory.
type groupStore struct {
	byID     map[shared.ID]domain.Group
	members  map[shared.ID][]shared.ID
	deleted  []shared.ID
	versions []int
}

func newGroups(existing ...domain.Group) *groupStore {
	store := &groupStore{byID: map[shared.ID]domain.Group{}, members: map[shared.ID][]shared.ID{}}
	for _, group := range existing {
		store.byID[group.ID] = group
	}
	return store
}

func (s *groupStore) Find(_ context.Context, id shared.ID) (domain.Group, error) {
	group, found := s.byID[id]
	if !found {
		return domain.Group{}, shared.ErrNotFound.WithDetail("groups.not_found")
	}
	return group, nil
}

func (s *groupStore) Insert(_ context.Context, group domain.Group) error {
	s.byID[group.ID] = group
	return nil
}

func (s *groupStore) Update(_ context.Context, group domain.Group, expectedVersion int) error {
	current, found := s.byID[group.ID]
	if !found {
		return shared.ErrNotFound.WithDetail("groups.not_found")
	}
	if current.Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("groups.version_conflict")
	}
	s.versions = append(s.versions, expectedVersion)
	group.Version = expectedVersion + 1
	s.byID[group.ID] = group
	return nil
}

func (s *groupStore) Delete(_ context.Context, id shared.ID) error {
	delete(s.byID, id)
	delete(s.members, id)
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *groupStore) AddMember(_ context.Context, groupID, accountID shared.ID) error {
	if !slices.Contains(s.members[groupID], accountID) {
		s.members[groupID] = append(s.members[groupID], accountID)
	}
	return nil
}

func (s *groupStore) RemoveMember(_ context.Context, groupID, accountID shared.ID) error {
	s.members[groupID] = slices.DeleteFunc(s.members[groupID],
		func(id shared.ID) bool { return id == accountID })
	return nil
}

func (s *groupStore) Members(_ context.Context, groupID shared.ID) ([]shared.ID, error) {
	return s.members[groupID], nil
}

var _ repository.Groups = (*groupStore)(nil)

func createHandler(groups *groupStore, accounts *accountStore, auth *authorizer, sink *auditSink) CreateGroup {
	return CreateGroup{
		Groups: groups, Accounts: accounts, Authorizer: auth, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: groupID},
	}
}

func TestAGroupIsCreatedWithItsMembers(t *testing.T) {
	groups, accounts, sink := newGroups(), newAccounts(invitedAccount(t)), &auditSink{}

	group, err := createHandler(groups, accounts, &authorizer{}, sink).
		Execute(t.Context(), admin(), CreateGroupCommand{Name: "Design", Members: []shared.ID{invitedID}})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	if group.Name != "Design" {
		t.Errorf("name %q", group.Name)
	}
	if members := groups.members[groupID]; len(members) != 1 || members[0] != invitedID {
		t.Errorf("members %v, want the account", members)
	}
	// The name is something a tenant chose and can identify a team or a customer, so the trail
	// carries a hash rather than the word.
	name, recorded := sink.entries[0].Changes["name"].(map[string]any)
	if !recorded || name["to"] == "Design" {
		t.Errorf("the trail records %v, want the name masked", sink.entries[0].Changes)
	}
}

// A group is useful before anybody is in it: the memberships can be granted to it first.
func TestAGroupCanBeCreatedEmpty(t *testing.T) {
	groups := newGroups()

	if _, err := createHandler(groups, newAccounts(), &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), CreateGroupCommand{Name: "Design"}); err != nil {
		t.Fatalf("creating an empty group: %v", err)
	}
	if len(groups.byID) != 1 {
		t.Error("the group was not written")
	}
}

func TestAMemberThatIsNotHereIsRefused(t *testing.T) {
	groups := newGroups()

	_, err := createHandler(groups, newAccounts(), &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), CreateGroupCommand{Name: "Design", Members: []shared.ID{invitedID}})

	if err == nil || shared.AsError(err).DetailCode != "accounts.not_found" {
		t.Fatalf("error %v, want the member refused", err)
	}
}

func updateHandler(groups *groupStore, accounts *accountStore, auth *authorizer, sink *auditSink) UpdateGroup {
	return UpdateGroup{
		Groups: groups, Accounts: accounts, Authorizer: auth, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

func settledGroup(t *testing.T) domain.Group {
	t.Helper()
	group, err := domain.NewGroup(domain.NewGroupInput{ID: groupID, TenantID: tenant, Name: "Design"})
	if err != nil {
		t.Fatalf("preparing the group: %v", err)
	}
	return group
}

// The member list is the complete membership afterwards, not an addition - otherwise removing
// somebody would be impossible to express.
func TestTheMemberListReplacesRatherThanAdds(t *testing.T) {
	second := shared.ID("01936f2a-7c1e-7000-8000-0000000000a3")
	other, err := domain.Invite(second, tenant, "cara@example.org", "Cara")
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}

	groups := newGroups(settledGroup(t))
	groups.members[groupID] = []shared.ID{invitedID}
	accounts := newAccounts(invitedAccount(t), other)

	if _, err := updateHandler(groups, accounts, &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), UpdateGroupCommand{
			GroupID: groupID, Members: []shared.ID{second}, ReplaceMembers: true,
		}); err != nil {
		t.Fatalf("updating: %v", err)
	}

	members := groups.members[groupID]
	if len(members) != 1 || members[0] != second {
		t.Errorf("members %v, want exactly the list that was sent", members)
	}
}

// An empty list empties the group, which is a legitimate thing to want; an absent one leaves the
// membership alone. The untyped channel has to keep the two apart.
func TestAnAbsentMemberListLeavesTheGroupAndAnEmptyOneEmptiesIt(t *testing.T) {
	groups := newGroups(settledGroup(t))
	groups.members[groupID] = []shared.ID{invitedID}
	handler := updateHandler(groups, newAccounts(invitedAccount(t)), &authorizer{}, &auditSink{})

	if _, err := handler.invoke(t.Context(), admin(), usecase.Input{
		"group_id": groupID.String(), "name": "Platform",
	}); err != nil {
		t.Fatalf("renaming without touching the members: %v", err)
	}
	if len(groups.members[groupID]) != 1 {
		t.Errorf("members %v, want them left alone", groups.members[groupID])
	}

	if _, err := handler.invoke(t.Context(), admin(), usecase.Input{
		"group_id": groupID.String(), "members": []any{},
	}); err != nil {
		t.Fatalf("emptying: %v", err)
	}
	if len(groups.members[groupID]) != 0 {
		t.Errorf("members %v, want the group emptied", groups.members[groupID])
	}
}

// Two administrators renaming at once: the second loses cleanly rather than overwriting.
func TestAStaleVersionIsRefused(t *testing.T) {
	groups := newGroups(settledGroup(t))
	name := "Platform"

	_, err := updateHandler(groups, newAccounts(), &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), UpdateGroupCommand{
			GroupID: groupID, Name: &name, ExpectedVersion: 7,
		})

	if err == nil || !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("error %v, want a version conflict", err)
	}
}

func deleteHandler(groups *groupStore, auth *authorizer, sink *auditSink) DeleteGroup {
	return DeleteGroup{
		Groups: groups, Authorizer: auth, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

// Deleting a group is an access change: whatever it granted, nobody holds afterwards.
func TestDeletingAGroupIsRecordedAsAnAccessChange(t *testing.T) {
	groups, sink := newGroups(settledGroup(t)), &auditSink{}

	if err := deleteHandler(groups, &authorizer{}, sink).
		Execute(t.Context(), admin(), DeleteGroupCommand{GroupID: groupID}); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	if len(groups.deleted) != 1 {
		t.Fatalf("%d deletions, want one", len(groups.deleted))
	}
	if sink.entries[0].Severity != audit.SeverityNotice {
		t.Errorf("severity %q, want NOTICE - somebody may have to explain this later",
			sink.entries[0].Severity)
	}
}

func TestDeletingAGroupThatIsNotThereIsNotFound(t *testing.T) {
	err := deleteHandler(newGroups(), &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), DeleteGroupCommand{GroupID: groupID})

	if err == nil || !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
}

func TestEveryGroupVerbNeedsThePermission(t *testing.T) {
	refusal := &authorizer{refuse: shared.ErrForbidden.WithDetail("access.not_permitted")}
	groups := newGroups(settledGroup(t))
	name := "Platform"

	if _, err := createHandler(groups, newAccounts(), refusal, &auditSink{}).
		Execute(t.Context(), admin(), CreateGroupCommand{Name: "Design"}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("create: %v", err)
	}
	if _, err := updateHandler(groups, newAccounts(), refusal, &auditSink{}).
		Execute(t.Context(), admin(), UpdateGroupCommand{GroupID: groupID, Name: &name}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("update: %v", err)
	}
	if err := deleteHandler(groups, refusal, &auditSink{}).
		Execute(t.Context(), admin(), DeleteGroupCommand{GroupID: groupID}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("delete: %v", err)
	}
	if len(groups.deleted) != 0 {
		t.Error("a refused delete removed the group")
	}
}

// Every use case of this package is registered in all three channels, and the registry is what
// checks that a descriptor is complete. Building each one here is what makes a missing audit
// declaration or an unnamed field a failing test rather than a startup error in production.
func TestEveryDescriptorIsComplete(t *testing.T) {
	descriptors := []usecase.Descriptor{
		InviteAccount{}.Descriptor(),
		UpdateAccountPreferences{}.Descriptor(),
		GrantMembership{}.Descriptor(),
		RevokeMembership{}.Descriptor(),
		CreateGroup{}.Descriptor(),
		UpdateGroup{}.Descriptor(),
		DeleteGroup{}.Descriptor(),
	}

	registry, err := usecase.NewRegistry(nil, descriptors...)
	if err != nil {
		t.Fatalf("the descriptors were refused: %v", err)
	}
	if len(registry.All()) != len(descriptors) {
		t.Errorf("%d registered, want %d", len(registry.All()), len(descriptors))
	}

	for _, descriptor := range descriptors {
		t.Run(descriptor.Name, func(t *testing.T) {
			if descriptor.TokenScope == "" {
				t.Error("no token scope - every one of these administers access")
			}
			if !descriptor.Audit.Required {
				t.Error("the audit entry is not required, and every one of these is an access change")
			}
		})
	}
}

// The channels reach the same handler as a direct call. Checked through the group verbs, which
// carry the two input shapes the others do not: a list, and an optional string.
func TestTheChannelInputReachesTheSameHandler(t *testing.T) {
	groups, accounts := newGroups(), newAccounts(invitedAccount(t))
	handler := createHandler(groups, accounts, &authorizer{}, &auditSink{})

	out, err := handler.invoke(t.Context(), admin(), usecase.Input{
		"name":    "Design",
		"members": []any{invitedID.String()},
	})
	if err != nil {
		t.Fatalf("through the channel: %v", err)
	}
	if out["name"] != "Design" || out["id"] != groupID.String() {
		t.Errorf("output %v", out)
	}
	if members := groups.members[groupID]; len(members) != 1 {
		t.Errorf("members %v, want the one the channel named", members)
	}

	// A malformed identifier in the list fails the request rather than half of it.
	if _, err := handler.invoke(t.Context(), admin(), usecase.Input{
		"name": "Platform", "members": []any{"not-an-id"},
	}); err == nil {
		t.Error("a malformed member was accepted through the channel")
	}
}

func TestTheDeleteAndRevokeChannelsReturnNothing(t *testing.T) {
	groups := newGroups(settledGroup(t))
	out, err := deleteHandler(groups, &authorizer{}, &auditSink{}).
		invoke(t.Context(), admin(), usecase.Input{"group_id": groupID.String()})
	if err != nil {
		t.Fatalf("deleting through the channel: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("output %v, want nothing - there is no group to return", out)
	}

	grants := newGrants(existingGrant(t))
	out, err = revokeHandler(grants, &authorizer{}, &auditSink{}).
		invoke(t.Context(), admin(), usecase.Input{"membership_id": membershipID.String()})
	if err != nil {
		t.Fatalf("revoking through the channel: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("output %v, want nothing", out)
	}
}

// An identifier the caller has to supply and did not is a validation error rather than a lookup of
// the zero identifier.
func TestTheVerbsThatNeedAnIdentifierSaySo(t *testing.T) {
	if err := deleteHandler(newGroups(), &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), DeleteGroupCommand{}); err == nil ||
		shared.AsError(err).DetailCode != "groups.identifier_required" {
		t.Errorf("delete: %v", err)
	}
	if _, err := updateHandler(newGroups(), newAccounts(), &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), UpdateGroupCommand{}); err == nil ||
		shared.AsError(err).DetailCode != "groups.identifier_required" {
		t.Errorf("update: %v", err)
	}
	if err := revokeHandler(newGrants(), &authorizer{}, &auditSink{}).
		Execute(t.Context(), admin(), RevokeMembershipCommand{}); err == nil ||
		shared.AsError(err).DetailCode != "memberships.identifier_required" {
		t.Errorf("revoke: %v", err)
	}
}
