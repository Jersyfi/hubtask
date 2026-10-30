// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// A directory provisions people and groups over SCIM 2.0 (UC-ID-28, identity.md §26). It acts
// through a service account's token, and every act is the one an administrator could take: the
// same permission, the same people limit, the same group rules - the group acts go through
// CreateGroup, UpdateGroup and DeleteGroup themselves. What is the directory's own is decided here:
// a person it provisions waits, INVITED, for their first sign-in through the workspace's provider;
// a person it deprovisions is switched off, never erased; an owner is not switched off by it.

const (
	// AccountProvisionedAction records a person a directory provisioned.
	AccountProvisionedAction audit.Action = "account.provisioned"
	// AccountSwitchedAction records a directory switching a person off or on.
	AccountSwitchedAction audit.Action = "account.directory_switched"
	// AccountDirectoryLinkedAction records the directory's identifier of a person changing.
	AccountDirectoryLinkedAction audit.Action = "account.directory_linked"
	// GroupDirectoryLinkedAction records the directory's identifier of a group changing.
	GroupDirectoryLinkedAction audit.Action = "group.directory_linked"
	// DirectoryReadAction is what a directory's read performs; recorded when refused.
	DirectoryReadAction audit.Action = "account.directory_read"

	// MaxDirectoryPage bounds one page of a directory's listing.
	MaxDirectoryPage = 200
)

// DirectoryPerson is a person as a directory sees them: the account and its identifier there.
type DirectoryPerson struct {
	Account    domain.Account
	ExternalID string
}

// DirectoryGroup is a group as a directory sees it, with its members where they were asked for.
type DirectoryGroup struct {
	Group      domain.Group
	ExternalID string
	Members    []shared.ID
}

// DirectoryFilter is the one comparison a directory's listing asks: an attribute equal to a value.
// The zero filter lists everybody.
type DirectoryFilter struct {
	// Attribute is "id", "userName", "externalId" or, for groups, "displayName".
	Attribute string
	Value     string
}

// DirectoryPage is a page by position, as SCIM pages (RFC 7644 §3.4.2.4).
type DirectoryPage[T any] struct {
	Items []T
	Total int
	// Start is the 1-based position of the first item.
	Start int
}

// ProvisionPersonCommand is a person a directory creates.
type ProvisionPersonCommand struct {
	Email       string
	DisplayName string
	ExternalID  string
	// Active false provisions the person switched off.
	Active bool
}

// ChangePersonCommand is what a directory changes of a person; nil leaves it.
type ChangePersonCommand struct {
	AccountID shared.ID
	// Email, when sent, must be the address the person has: the directory does not change it.
	Email       *string
	DisplayName *string
	ExternalID  *string
	Active      *bool
}

// ChangeGroupCommand is what a directory changes of a group; nil and empty leave it.
type ChangeGroupCommand struct {
	GroupID    shared.ID
	Name       *string
	ExternalID *string
	// Replace, when set, is the whole membership after the change; Add and Remove apply after it.
	Replace *[]shared.ID
	Add     []shared.ID
	Remove  []shared.ID
}

// Directory is the service a directory's door calls.
type Directory struct {
	Accounts   repository.Accounts
	Names      repository.AccountNames
	Links      repository.DirectoryLinks
	Groups     repository.Groups
	Sessions   SessionEnder
	Owners     repository.Owners
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Domains    text.DomainEncoder
	Text       text.Normalizer
	// Limit refuses a person past the workspace's people limit; nil is no limit.
	Limit PeopleLimit

	CreateGroup CreateGroup
	UpdateGroup UpdateGroup
	DeleteGroup DeleteGroup
}

func (d Directory) authorize(ctx context.Context, actor appshared.ActorContext, action audit.Action, scope, target string) error {
	return d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     action,
		TokenScope: scope,
		TargetType: target,
	})
}

// ProvisionPerson creates the person, or switches on again the one the directory switched off
// under that address. The address is the provider's sign-in, so it is required.
func (d Directory) ProvisionPerson(
	ctx context.Context, actor appshared.ActorContext, cmd ProvisionPersonCommand,
) (DirectoryPerson, error) {
	if err := d.authorize(ctx, actor, AccountProvisionedAction, membersWrite, accountTarget); err != nil {
		return DirectoryPerson{}, err
	}
	externalID, err := domain.ExternalID(cmd.ExternalID)
	if err != nil {
		return DirectoryPerson{}, err
	}
	person, err := domain.Invite(d.IDs.NewID(), actor.TenantID, cmd.Email, cmd.DisplayName, d.Domains, d.Text)
	if err != nil {
		return DirectoryPerson{}, err
	}
	if !cmd.Active {
		person.Status = domain.AccountDisabled
	}

	var out DirectoryPerson
	err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		existing, err := d.Accounts.FindByEmail(ctx, person.Email)
		switch {
		case err == nil:
			if existing.Kind != domain.AccountUser || existing.Status != domain.AccountDisabled || !cmd.Active {
				return alreadyHere(existing)
			}
			// The directory provisions again whom it switched off: the same person, back.
			if err := d.room(ctx, actor); err != nil {
				return err
			}
			if err := d.switchTo(ctx, actor, existing, domain.AccountActive); err != nil {
				return err
			}
			existing.Status = domain.AccountActive
			held, err := d.Links.AccountExternalIDs(ctx, []shared.ID{existing.ID})
			if err != nil {
				return err
			}
			if err := d.link(ctx, actor, existing.ID, held[existing.ID], externalID); err != nil {
				return err
			}
			out = DirectoryPerson{Account: existing, ExternalID: externalID}
			return nil
		case !errors.Is(err, shared.ErrNotFound):
			return err
		}
		if cmd.Active {
			if err := d.room(ctx, actor); err != nil {
				return err
			}
		}
		if err := d.Accounts.Insert(ctx, person); err != nil {
			return err
		}
		if externalID != "" {
			if err := d.Links.SetAccountExternalID(ctx, person.ID, externalID, d.Clock.Now()); err != nil {
				return err
			}
		}
		out = DirectoryPerson{Account: person, ExternalID: externalID}
		return d.record(ctx, actor, AccountProvisionedAction, audit.SeverityNotice, accountTarget, person.ID,
			audit.Change{Field: "email", Classification: audit.Sensitive, To: person.Email},
			audit.Change{Field: "status", Classification: audit.Open, To: string(person.Status)},
		)
	})
	return out, err
}

func (d Directory) room(ctx context.Context, actor appshared.ActorContext) error {
	if d.Limit == nil {
		return nil
	}
	return d.Limit.Accounts(ctx, actor.TenantID.String(), 1)
}

// Person reads one person; a service account or an erased person is not one a directory sees.
func (d Directory) Person(ctx context.Context, actor appshared.ActorContext, id shared.ID) (DirectoryPerson, error) {
	if err := d.authorize(ctx, actor, DirectoryReadAction, membersRead, accountTarget); err != nil {
		return DirectoryPerson{}, err
	}
	var out DirectoryPerson
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		people, err := d.people(ctx, []shared.ID{id})
		if err != nil {
			return err
		}
		if len(people) == 0 {
			return shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		out = people[0]
		return nil
	})
	return out, err
}

// people reads the accounts a directory sees, in the order asked, with their identifiers.
func (d Directory) people(ctx context.Context, ids []shared.ID) ([]DirectoryPerson, error) {
	found := make([]DirectoryPerson, 0, len(ids))
	for _, id := range ids {
		account, err := d.Accounts.Find(ctx, id)
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if account.Kind != domain.AccountUser || account.Status == domain.AccountAnonymized {
			continue
		}
		found = append(found, DirectoryPerson{Account: account})
	}
	if len(found) == 0 {
		return found, nil
	}
	keys := make([]shared.ID, len(found))
	for i, person := range found {
		keys[i] = person.Account.ID
	}
	external, err := d.Links.AccountExternalIDs(ctx, keys)
	if err != nil {
		return nil, err
	}
	for i := range found {
		found[i].ExternalID = external[found[i].Account.ID]
	}
	return found, nil
}

// People lists the people a directory sees: everybody by position, or those one filter finds.
func (d Directory) People(
	ctx context.Context, actor appshared.ActorContext, filter DirectoryFilter, start, count int,
) (DirectoryPage[DirectoryPerson], error) {
	if err := d.authorize(ctx, actor, DirectoryReadAction, membersRead, accountTarget); err != nil {
		return DirectoryPage[DirectoryPerson]{}, err
	}
	start, count = pageBounds(start, count)
	var out DirectoryPage[DirectoryPerson]
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var ids []shared.ID
		total := 0
		switch filter.Attribute {
		case "":
			page, all, err := d.Links.PageAccounts(ctx, start-1, count)
			if err != nil {
				return err
			}
			ids, total = page, all
		case "id":
			ids = []shared.ID{shared.ID(filter.Value)}
		case "userName", "emails.value":
			account, err := d.Accounts.FindByEmail(ctx, domain.LookupAddress(filter.Value, d.Domains))
			if err == nil {
				ids = []shared.ID{account.ID}
			} else if !errors.Is(err, shared.ErrNotFound) {
				return err
			}
		case "externalId":
			id, err := d.Links.FindAccountByExternalID(ctx, filter.Value)
			if err == nil {
				ids = []shared.ID{id}
			} else if !errors.Is(err, shared.ErrNotFound) {
				return err
			}
		default:
			return unsupportedFilter()
		}
		people, err := d.people(ctx, ids)
		if err != nil {
			return err
		}
		if filter.Attribute != "" {
			total = len(people)
			people = window(people, start, count)
		}
		out = DirectoryPage[DirectoryPerson]{Items: people, Total: total, Start: start}
		return nil
	})
	return out, err
}

func unsupportedFilter() error {
	return shared.ErrValidation.WithDetail("directory.filter_unsupported")
}

func pageBounds(start, count int) (int, int) {
	if start < 1 {
		start = 1
	}
	if count < 0 || count > MaxDirectoryPage {
		count = MaxDirectoryPage
	}
	return start, count
}

// window is the page of a filtered answer, which holds one item at most but pages all the same.
func window[T any](items []T, start, count int) []T {
	if start-1 >= len(items) {
		return []T{}
	}
	items = items[start-1:]
	if count < len(items) {
		items = items[:count]
	}
	return items
}

// ChangePerson applies what the directory changed: the name shown, its identifier, whether the
// person may act.
func (d Directory) ChangePerson(
	ctx context.Context, actor appshared.ActorContext, cmd ChangePersonCommand,
) (DirectoryPerson, error) {
	if err := d.authorize(ctx, actor, AccountSwitchedAction, membersWrite, accountTarget); err != nil {
		return DirectoryPerson{}, err
	}
	var externalID string
	if cmd.ExternalID != nil {
		checked, err := domain.ExternalID(*cmd.ExternalID)
		if err != nil {
			return DirectoryPerson{}, err
		}
		externalID = checked
	}
	var out DirectoryPerson
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		people, err := d.people(ctx, []shared.ID{cmd.AccountID})
		if err != nil {
			return err
		}
		if len(people) == 0 {
			return shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		person := people[0]
		if cmd.Email != nil && !person.Account.SameAddress(domain.LookupAddress(*cmd.Email, d.Domains)) {
			return shared.ErrValidation.WithDetail("directory.address_immutable").
				WithFields(shared.FieldError{Path: "/userName", Code: "directory.address_immutable"})
		}
		if cmd.DisplayName != nil && strings.TrimSpace(*cmd.DisplayName) != "" &&
			*cmd.DisplayName != person.Account.DisplayName {
			renamed, err := person.Account.WithDisplayName(*cmd.DisplayName, d.Text)
			if err != nil {
				return err
			}
			if err := d.Names.Rename(ctx, renamed, d.Clock.Now()); err != nil {
				return err
			}
			person.Account = renamed
		}
		if cmd.ExternalID != nil && externalID != person.ExternalID {
			if err := d.link(ctx, actor, person.Account.ID, person.ExternalID, externalID); err != nil {
				return err
			}
			person.ExternalID = externalID
		}
		if cmd.Active != nil {
			next, changed, err := domain.DirectoryStatus(person.Account.Status, *cmd.Active)
			if err != nil {
				return err
			}
			if changed {
				if next == domain.AccountDisabled {
					if err := d.notAnOwner(ctx, person.Account.ID); err != nil {
						return err
					}
				} else if err := d.room(ctx, actor); err != nil {
					return err
				}
				if err := d.switchTo(ctx, actor, person.Account, next); err != nil {
					return err
				}
				person.Account.Status = next
			}
		}
		out = person
		return nil
	})
	return out, err
}

// Deprovision switches the person off: a directory never erases anybody's data.
func (d Directory) Deprovision(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	off := false
	_, err := d.ChangePerson(ctx, actor, ChangePersonCommand{AccountID: id, Active: &off})
	return err
}

// notAnOwner refuses switching off a person who holds OWNER on the workspace, directly or through a
// group: an owner is the workspace's own to remove (domain-model.md §3.2), and this also keeps its
// last one.
func (d Directory) notAnOwner(ctx context.Context, accountID shared.ID) error {
	if d.Owners == nil {
		return shared.ErrInternal.WithDetail("memberships.owners_missing")
	}
	if err := d.Owners.LockOwners(ctx); err != nil {
		return err
	}
	paths, err := d.Owners.OwnerPaths(ctx)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if path.AccountID == accountID {
			return shared.ErrConflict.WithDetail("directory.owner_kept")
		}
	}
	return nil
}

// switchTo writes the status, ends a switched-off person's sessions and records it.
func (d Directory) switchTo(
	ctx context.Context, actor appshared.ActorContext, account domain.Account, next domain.AccountStatus,
) error {
	now := d.Clock.Now()
	moved, err := d.Links.SetAccountStatus(ctx, account.ID, next, now)
	if err != nil {
		return err
	}
	if !moved {
		return shared.ErrConflict.WithDetail("directory.status_moved")
	}
	if next == domain.AccountDisabled && d.Sessions != nil {
		if _, err := d.Sessions.RevokeAll(ctx, account.ID, now); err != nil {
			return err
		}
	}
	return d.record(ctx, actor, AccountSwitchedAction, audit.SeverityNotice, accountTarget, account.ID,
		audit.Change{Field: "status", Classification: audit.Open, From: string(account.Status), To: string(next)})
}

func (d Directory) link(ctx context.Context, actor appshared.ActorContext, id shared.ID, from, to string) error {
	if from == to {
		return nil
	}
	if err := d.Links.SetAccountExternalID(ctx, id, to, d.Clock.Now()); err != nil {
		return err
	}
	return d.record(ctx, actor, AccountDirectoryLinkedAction, audit.SeverityInfo, accountTarget, id,
		audit.Change{Field: "external_id", Classification: audit.Sensitive, From: from, To: to})
}

func (d Directory) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, severity audit.Severity,
	target string, id shared.ID, changes ...audit.Change,
) error {
	return d.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: d.Clock.Now(),
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   severity,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: target,
		TargetID:   id,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// ProvisionGroup creates a group with its members as CreateGroup does, and keeps the directory's
// identifier beside it.
func (d Directory) ProvisionGroup(
	ctx context.Context, actor appshared.ActorContext, name, externalID string, members []shared.ID,
) (DirectoryGroup, error) {
	checked, err := domain.ExternalID(externalID)
	if err != nil {
		return DirectoryGroup{}, err
	}
	if checked != "" {
		if err := d.groupIdentifierFree(ctx, actor, checked); err != nil {
			return DirectoryGroup{}, err
		}
	}
	group, err := d.CreateGroup.Execute(ctx, actor, CreateGroupCommand{Name: name, Members: members})
	if err != nil {
		return DirectoryGroup{}, err
	}
	if checked != "" {
		if err := d.linkGroup(ctx, actor, group.ID, "", checked); err != nil {
			return DirectoryGroup{}, err
		}
	}
	return d.Group(ctx, actor, group.ID, true)
}

func (d Directory) groupIdentifierFree(ctx context.Context, actor appshared.ActorContext, externalID string) error {
	return d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, err := d.Links.FindGroupByExternalID(ctx, externalID)
		switch {
		case err == nil:
			return shared.ErrConflict.WithDetail("directory.external_id_taken")
		case errors.Is(err, shared.ErrNotFound):
			return nil
		}
		return err
	})
}

func (d Directory) linkGroup(ctx context.Context, actor appshared.ActorContext, id shared.ID, from, to string) error {
	if err := d.authorize(ctx, actor, GroupDirectoryLinkedAction, membersWrite, groupTarget); err != nil {
		return err
	}
	return d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := d.Links.SetGroupExternalID(ctx, id, to); err != nil {
			return err
		}
		return d.record(ctx, actor, GroupDirectoryLinkedAction, audit.SeverityInfo, groupTarget, id,
			audit.Change{Field: "external_id", Classification: audit.Sensitive, From: from, To: to})
	})
}

// Group reads one group, with its members when asked.
func (d Directory) Group(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, withMembers bool,
) (DirectoryGroup, error) {
	if err := d.authorize(ctx, actor, DirectoryReadAction, membersRead, groupTarget); err != nil {
		return DirectoryGroup{}, err
	}
	var out DirectoryGroup
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		groups, err := d.groups(ctx, []shared.ID{id}, withMembers)
		if err != nil {
			return err
		}
		if len(groups) == 0 {
			return shared.ErrNotFound.WithDetail("groups.not_found")
		}
		out = groups[0]
		return nil
	})
	return out, err
}

func (d Directory) groups(ctx context.Context, ids []shared.ID, withMembers bool) ([]DirectoryGroup, error) {
	found := make([]DirectoryGroup, 0, len(ids))
	for _, id := range ids {
		group, err := d.Groups.Find(ctx, id)
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		entry := DirectoryGroup{Group: group}
		if withMembers {
			members, err := d.Groups.Members(ctx, id)
			if err != nil {
				return nil, err
			}
			slices.Sort(members)
			entry.Members = members
		}
		found = append(found, entry)
	}
	if len(found) == 0 {
		return found, nil
	}
	keys := make([]shared.ID, len(found))
	for i, group := range found {
		keys[i] = group.Group.ID
	}
	external, err := d.Links.GroupExternalIDs(ctx, keys)
	if err != nil {
		return nil, err
	}
	for i := range found {
		found[i].ExternalID = external[found[i].Group.ID]
	}
	return found, nil
}

// GroupsPage lists the groups: all by position, or those one filter finds.
func (d Directory) GroupsPage(
	ctx context.Context, actor appshared.ActorContext, filter DirectoryFilter, start, count int, withMembers bool,
) (DirectoryPage[DirectoryGroup], error) {
	if err := d.authorize(ctx, actor, DirectoryReadAction, membersRead, groupTarget); err != nil {
		return DirectoryPage[DirectoryGroup]{}, err
	}
	start, count = pageBounds(start, count)
	var out DirectoryPage[DirectoryGroup]
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var ids []shared.ID
		total := 0
		var find func(context.Context, string) (shared.ID, error)
		switch filter.Attribute {
		case "":
			page, all, err := d.Links.PageGroups(ctx, start-1, count)
			if err != nil {
				return err
			}
			ids, total = page, all
		case "id":
			ids = []shared.ID{shared.ID(filter.Value)}
		case "displayName":
			find = d.Links.FindGroupByName
		case "externalId":
			find = d.Links.FindGroupByExternalID
		default:
			return unsupportedFilter()
		}
		if find != nil {
			id, err := find(ctx, filter.Value)
			if err == nil {
				ids = []shared.ID{id}
			} else if !errors.Is(err, shared.ErrNotFound) {
				return err
			}
		}
		groups, err := d.groups(ctx, ids, withMembers)
		if err != nil {
			return err
		}
		if filter.Attribute != "" {
			total = len(groups)
			groups = window(groups, start, count)
		}
		out = DirectoryPage[DirectoryGroup]{Items: groups, Total: total, Start: start}
		return nil
	})
	return out, err
}

// ChangeGroup applies a directory's change through UpdateGroup, whose rules hold as they do for a
// person: the ceiling of every role the group holds, and a group holding OWNER changed only with a
// person's step-up, which a directory never has.
func (d Directory) ChangeGroup(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeGroupCommand,
) (DirectoryGroup, error) {
	current, err := d.Group(ctx, actor, cmd.GroupID, true)
	if err != nil {
		return DirectoryGroup{}, err
	}
	members := current.Members
	if cmd.Replace != nil {
		members = slices.Clone(*cmd.Replace)
	}
	for _, id := range cmd.Add {
		if !slices.Contains(members, id) {
			members = append(members, id)
		}
	}
	members = slices.DeleteFunc(slices.Clone(members), func(id shared.ID) bool { return slices.Contains(cmd.Remove, id) })
	slices.Sort(members)
	members = slices.Compact(members)

	update := UpdateGroupCommand{GroupID: cmd.GroupID, ExpectedVersion: current.Group.Version}
	changed := false
	if cmd.Name != nil && *cmd.Name != current.Group.Name {
		update.Name, changed = cmd.Name, true
	}
	if !slices.Equal(members, current.Members) {
		update.Members, update.ReplaceMembers, changed = members, true, true
	}
	if changed {
		if _, err := d.UpdateGroup.Execute(ctx, actor, update); err != nil {
			return DirectoryGroup{}, err
		}
	}
	if cmd.ExternalID != nil {
		checked, err := domain.ExternalID(*cmd.ExternalID)
		if err != nil {
			return DirectoryGroup{}, err
		}
		if checked != current.ExternalID {
			if err := d.linkGroup(ctx, actor, cmd.GroupID, current.ExternalID, checked); err != nil {
				return DirectoryGroup{}, err
			}
		}
	}
	return d.Group(ctx, actor, cmd.GroupID, true)
}

// RemoveGroup deletes the group through DeleteGroup; its people stay.
func (d Directory) RemoveGroup(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	return d.DeleteGroup.Execute(ctx, actor, DeleteGroupCommand{GroupID: id})
}
