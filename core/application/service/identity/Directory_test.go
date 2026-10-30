// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// directoryLinks is migration 0256's two columns, in memory, over the account store.
type directoryLinks struct {
	accounts *accountStore
	people   map[shared.ID]string
	groups   map[shared.ID]string
	names    map[string]shared.ID
}

func newLinks(accounts *accountStore) *directoryLinks {
	return &directoryLinks{accounts: accounts, people: map[shared.ID]string{}, groups: map[shared.ID]string{}, names: map[string]shared.ID{}}
}

func pick(held map[shared.ID]string, ids []shared.ID) map[shared.ID]string {
	out := map[shared.ID]string{}
	for _, id := range ids {
		if v, ok := held[id]; ok {
			out[id] = v
		}
	}
	return out
}

func find(held map[shared.ID]string, value string, code string) (shared.ID, error) {
	for id, v := range held {
		if v == value {
			return id, nil
		}
	}
	return "", shared.ErrNotFound.WithDetail(code)
}

func (l *directoryLinks) AccountExternalIDs(_ context.Context, ids []shared.ID) (map[shared.ID]string, error) {
	return pick(l.people, ids), nil
}

func (l *directoryLinks) FindAccountByExternalID(_ context.Context, value string) (shared.ID, error) {
	return find(l.people, value, "accounts.not_found")
}

func (l *directoryLinks) SetAccountExternalID(_ context.Context, id shared.ID, value string, _ time.Time) error {
	if holder, err := find(l.people, value, ""); err == nil && holder != id && value != "" {
		return shared.ErrConflict.WithDetail("directory.external_id_taken")
	}
	if value == "" {
		delete(l.people, id)
	} else {
		l.people[id] = value
	}
	return nil
}

func (l *directoryLinks) SetAccountStatus(_ context.Context, id shared.ID, status domain.AccountStatus, _ time.Time) (bool, error) {
	account, ok := l.accounts.byID[id]
	if !ok || account.Status == domain.AccountRestricted || account.Status == domain.AccountAnonymized {
		return false, nil
	}
	account.Status = status
	l.accounts.byID[id], l.accounts.byEmail[account.Email] = account, account
	return true, nil
}

func (l *directoryLinks) PageAccounts(_ context.Context, offset, limit int) ([]shared.ID, int, error) {
	var ids []shared.ID
	for id := range l.accounts.byID {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	total := len(ids)
	return window(ids, offset+1, limit), total, nil
}

func (l *directoryLinks) GroupExternalIDs(_ context.Context, ids []shared.ID) (map[shared.ID]string, error) {
	return pick(l.groups, ids), nil
}

func (l *directoryLinks) FindGroupByExternalID(_ context.Context, value string) (shared.ID, error) {
	return find(l.groups, value, "groups.not_found")
}

func (l *directoryLinks) FindGroupByName(_ context.Context, name string) (shared.ID, error) {
	if id, ok := l.names[name]; ok {
		return id, nil
	}
	return "", shared.ErrNotFound.WithDetail("groups.not_found")
}

func (l *directoryLinks) SetGroupExternalID(_ context.Context, id shared.ID, value string) error {
	l.groups[id] = value
	return nil
}

func (l *directoryLinks) PageGroups(context.Context, int, int) ([]shared.ID, int, error) {
	return nil, 0, nil
}

var _ repository.DirectoryLinks = (*directoryLinks)(nil)

type directoryFixture struct {
	accounts *accountStore
	links    *directoryLinks
	groups   *groupStore
	owners   *ownersFake
	sessions *sessionEnder
	sink     *auditSink
	service  Directory
}

func newDirectory(existing ...domain.Account) directoryFixture {
	accounts := newAccounts(existing...)
	f := directoryFixture{
		accounts: accounts, links: newLinks(accounts), groups: newGroups(), owners: &ownersFake{},
		sessions: &sessionEnder{}, sink: &auditSink{},
	}
	auth := &authorizer{}
	f.service = Directory{
		Accounts: accounts, Names: accounts, Links: f.links, Groups: f.groups, Sessions: f.sessions,
		Owners: f.owners, Authorizer: auth, Audit: f.sink, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), IDs: ids{next: invitedID},
		CreateGroup: createHandler(f.groups, accounts, auth, f.sink),
		UpdateGroup: updateHandler(f.groups, accounts, auth, f.sink),
	}
	return f
}

// UC-ID-28 check 1: a person the directory provisions waits for their first sign-in, holding the
// directory's identifier.
func TestADirectoryProvisionsAPersonWhoWaitsForTheirFirstSignIn(t *testing.T) {
	f := newDirectory()
	person, err := f.service.ProvisionPerson(t.Context(), admin(), ProvisionPersonCommand{
		Email: "Lena@Example.org", DisplayName: "Lena", ExternalID: "00u1", Active: true,
	})
	if err != nil {
		t.Fatalf("provisioning: %v", err)
	}
	if person.Account.Status != domain.AccountInvited || person.Account.Email != "lena@example.org" {
		t.Errorf("provisioned %+v, want an invited account under the folded address", person.Account)
	}
	if f.links.people[invitedID] != "00u1" || person.ExternalID != "00u1" {
		t.Errorf("identifier %q, want the directory's", f.links.people[invitedID])
	}
	if len(f.sink.entries) != 1 || f.sink.entries[0].Action != AccountProvisionedAction {
		t.Errorf("trail %v, want one provisioning entry", f.sink.entries)
	}

	// The same address again is the uniqueness conflict a directory expects.
	if _, err := f.service.ProvisionPerson(t.Context(), admin(), ProvisionPersonCommand{
		Email: "lena@example.org", Active: true,
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("provisioning twice: %v, want a conflict", err)
	}
}

// UC-ID-28 check 2: deprovisioning switches the person off and ends their sessions; nothing is
// erased, and provisioning the address again switches the same account on.
func TestDeprovisioningSwitchesOffAndProvisioningAgainSwitchesOn(t *testing.T) {
	active := invitedAccount(t)
	active.Status = domain.AccountActive
	f := newDirectory(active)

	if err := f.service.Deprovision(t.Context(), admin(), invitedID); err != nil {
		t.Fatalf("deprovisioning: %v", err)
	}
	if got := f.accounts.byID[invitedID].Status; got != domain.AccountDisabled {
		t.Fatalf("status %s, want DISABLED", got)
	}
	if !slices.Contains(f.sessions.ended, invitedID) {
		t.Error("the person's sessions were not ended")
	}

	again, err := f.service.ProvisionPerson(t.Context(), admin(), ProvisionPersonCommand{
		Email: active.Email, ExternalID: "00u9", Active: true,
	})
	if err != nil {
		t.Fatalf("provisioning again: %v", err)
	}
	if again.Account.ID != invitedID || f.accounts.byID[invitedID].Status != domain.AccountActive {
		t.Errorf("provisioned %+v, want the same account switched on", again.Account)
	}
	if len(f.accounts.inserted) != 0 {
		t.Error("a second account was made")
	}
}

// UC-ID-28 check 3: a directory does not switch off an owner, and not a restricted person.
func TestADirectoryKeepsOwnersAndRestrictedPeople(t *testing.T) {
	owner := invitedAccount(t)
	owner.Status = domain.AccountActive
	f := newDirectory(owner)
	f.owners.paths = []repository.OwnerPath{{AccountID: invitedID}}
	if err := f.service.Deprovision(t.Context(), admin(), invitedID); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "directory.owner_kept" {
		t.Errorf("switching off an owner: %v, want directory.owner_kept", err)
	}
	if !f.owners.locked {
		t.Error("the owner lock was not taken")
	}

	restricted := invitedAccount(t)
	restricted.Status = domain.AccountRestricted
	f = newDirectory(restricted)
	if err := f.service.Deprovision(t.Context(), admin(), invitedID); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "directory.restricted_kept" {
		t.Errorf("switching off a restricted person: %v, want directory.restricted_kept", err)
	}
}

// UC-ID-28 check 4: the address a person signs in with is not the directory's to change.
func TestADirectoryDoesNotChangeTheAddress(t *testing.T) {
	f := newDirectory(invitedAccount(t))
	other := "someone@example.org"
	_, err := f.service.ChangePerson(t.Context(), admin(), ChangePersonCommand{AccountID: invitedID, Email: &other})
	if shared.AsError(err) == nil || shared.AsError(err).DetailCode != "directory.address_immutable" {
		t.Errorf("changing the address: %v, want directory.address_immutable", err)
	}
	name := "Bert Neu"
	changed, err := f.service.ChangePerson(t.Context(), admin(), ChangePersonCommand{AccountID: invitedID, DisplayName: &name})
	if err != nil || changed.Account.DisplayName != name {
		t.Errorf("renaming: %+v, %v", changed.Account, err)
	}
}

// UC-ID-28 check 5: a directory finds a person by address and by its own identifier; a filter on
// anything else is refused rather than answered as everybody.
func TestADirectoryFindsAPersonByAddressAndIdentifier(t *testing.T) {
	f := newDirectory(invitedAccount(t))
	f.links.people[invitedID] = "00u1"
	for _, filter := range []DirectoryFilter{
		{Attribute: "userName", Value: invitedAccount(t).Email},
		{Attribute: "externalId", Value: "00u1"},
		{Attribute: "id", Value: invitedID.String()},
	} {
		page, err := f.service.People(t.Context(), admin(), filter, 1, 10)
		if err != nil || page.Total != 1 || page.Items[0].Account.ID != invitedID || page.Items[0].ExternalID != "00u1" {
			t.Errorf("%v: %+v, %v", filter, page, err)
		}
	}
	page, err := f.service.People(t.Context(), admin(), DirectoryFilter{Attribute: "externalId", Value: "none"}, 1, 10)
	if err != nil || page.Total != 0 {
		t.Errorf("an unknown identifier: %+v, %v", page, err)
	}
	if _, err := f.service.People(t.Context(), admin(), DirectoryFilter{Attribute: "title", Value: "x"}, 1, 10); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an unsupported filter: %v, want refused", err)
	}
}

// UC-ID-28 check 6: a group's members change by addition and removal through UpdateGroup.
func TestADirectoryChangesAGroupsMembersThroughTheGroupRules(t *testing.T) {
	second := shared.ID("01936f2a-7c1e-7000-8000-0000000000a3")
	other, err := domain.Invite(second, tenant, "cara@example.org", "Cara", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	f := newDirectory(invitedAccount(t), other)
	f.groups.byID[groupID] = settledGroup(t)
	f.groups.members[groupID] = []shared.ID{invitedID}

	changed, err := f.service.ChangeGroup(t.Context(), admin(), ChangeGroupCommand{
		GroupID: groupID, Add: []shared.ID{second}, Remove: []shared.ID{invitedID},
	})
	if err != nil {
		t.Fatalf("changing: %v", err)
	}
	if !slices.Equal(changed.Members, []shared.ID{second}) {
		t.Errorf("members %v, want the one added", changed.Members)
	}
	if len(f.groups.versions) != 1 {
		t.Errorf("UpdateGroup ran %d times, want once", len(f.groups.versions))
	}

	// Nothing to change writes nothing.
	if _, err := f.service.ChangeGroup(t.Context(), admin(), ChangeGroupCommand{
		GroupID: groupID, Add: []shared.ID{second},
	}); err != nil || len(f.groups.versions) != 1 {
		t.Errorf("an unchanged group was written: %v, %v", f.groups.versions, err)
	}
}
