// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateGroupName = "CreateGroup"
	UpdateGroupName = "UpdateGroup"
	DeleteGroupName = "DeleteGroup"
	groupTarget     = "group"

	GroupCreatedAction audit.Action = "group.created"
	GroupUpdatedAction audit.Action = "group.updated"
	GroupDeletedAction audit.Action = "group.deleted"
)

// CreateGroupCommand is the input, typed.
type CreateGroupCommand struct {
	Name        string
	Description string
	// Members is who is in the group from the start. Optional: a group is useful before anybody
	// is in it, because the memberships can be granted to it first.
	Members []shared.ID
}

// CreateGroup creates a named set of accounts.
type CreateGroup struct {
	Groups     repository.Groups
	Accounts   repository.Accounts
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Execute creates the group and returns it.
func (h CreateGroup) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateGroupCommand,
) (domain.Group, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     GroupCreatedAction,
		TokenScope: membersWrite,
		TargetType: groupTarget,
	}); err != nil {
		return domain.Group{}, err
	}

	group, err := domain.NewGroup(domain.NewGroupInput{
		ID:          h.IDs.NewID(),
		TenantID:    actor.TenantID,
		Name:        cmd.Name,
		Description: cmd.Description,
	})
	if err != nil {
		return domain.Group{}, err
	}

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Groups.Insert(ctx, group); err != nil {
			return err
		}
		if err := addMembers(ctx, h.Groups, h.Accounts, group.ID, cmd.Members); err != nil {
			return err
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID:   group.TenantID,
			OccurredAt: h.Clock.Now(),
			Action:     GroupCreatedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityInfo,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: groupTarget,
			TargetID:   group.ID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				// A group's name is something a tenant chose and can identify a team or a
				// customer, so it is masked like every other piece of user content (rule 10).
				audit.Change{Field: "name", Classification: audit.Sensitive, To: group.Name},
				audit.Change{Field: "members", Classification: audit.Open, To: len(cmd.Members)},
			),
		})
	})
	if err != nil {
		return domain.Group{}, err
	}
	return group, nil
}

// addMembers puts the accounts in the group, checking each exists first so that a typo is an
// answer rather than a foreign key violation.
func addMembers(
	ctx context.Context, groups repository.Groups, accounts repository.Accounts,
	groupID shared.ID, members []shared.ID,
) error {
	for _, accountID := range members {
		if accountID.IsZero() {
			continue
		}
		if _, err := accounts.Find(ctx, accountID); err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.
					WithDetail("accounts.not_found").
					WithParams(map[string]string{"account_id": accountID.String()})
			}
			return err
		}
		if err := groups.AddMember(ctx, groupID, accountID); err != nil {
			return err
		}
	}
	return nil
}

// Descriptor registers the use case in all three channels.
func (h CreateGroup) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateGroupName,
		Summary: "Creates a group: a named set of accounts that a role can be granted to, so that " +
			"the people who hold it change when the group does.",
		SideEffects: "Writes the group, its members, and an audit entry.",
		TokenScope:  membersWrite,
		Input: []usecase.Field{
			{
				Name: "name", Kind: usecase.KindString, Required: true,
				Description: "Up to 200 characters, on one line, unique within the workspace.",
			},
			{Name: "description", Kind: usecase.KindString},
			{
				Name: "members", Kind: usecase.KindIDList,
				Description: "Accounts to put in the group straight away. A group is useful before anybody is in it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: GroupCreatedAction, TargetType: groupTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateGroup) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	members, err := in.IDList("members")
	if err != nil {
		return nil, err
	}

	group, err := h.Execute(ctx, actor, CreateGroupCommand{
		Name:        in.String("name"),
		Description: in.String("description"),
		Members:     members,
	})
	if err != nil {
		return nil, err
	}
	return groupOutput(group), nil
}

func groupOutput(group domain.Group) usecase.Output {
	out := usecase.Output{
		"id":      group.ID.String(),
		"name":    group.Name,
		"version": group.Version,
	}
	if group.Description != "" {
		out["description"] = group.Description
	}
	return out
}

// UpdateGroupCommand is the input, typed. Name and description are pointers for the same reason
// the preferences are: omitted leaves, sent replaces.
type UpdateGroupCommand struct {
	GroupID     shared.ID
	Name        *string
	Description *string
	// Members, when given, is the complete membership after the change - not an addition. A
	// partial list would make "remove somebody" impossible to express.
	Members         []shared.ID
	ReplaceMembers  bool
	ExpectedVersion int
}

// UpdateGroup renames a group, changes its description, or replaces who is in it.
type UpdateGroup struct {
	Groups     repository.Groups
	Accounts   repository.Accounts
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute applies the change and returns the group.
func (h UpdateGroup) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateGroupCommand,
) (domain.Group, error) {
	if cmd.GroupID.IsZero() {
		return domain.Group{}, shared.ErrValidation.WithDetail("groups.identifier_required")
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     GroupUpdatedAction,
		TokenScope: membersWrite,
		TargetType: groupTarget,
		TargetID:   cmd.GroupID,
	}); err != nil {
		return domain.Group{}, err
	}

	var updated domain.Group
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		group, err := h.Groups.Find(ctx, cmd.GroupID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.
					WithDetail("groups.not_found").
					WithParams(map[string]string{"group_id": cmd.GroupID.String()})
			}
			return err
		}

		before := group
		if cmd.Name != nil {
			if group, err = group.Rename(*cmd.Name); err != nil {
				return err
			}
		}
		if cmd.Description != nil {
			if group, err = group.Describe(*cmd.Description); err != nil {
				return err
			}
		}

		// Zero means the caller did not read a version and accepts whatever is there. Anything
		// else is an If-Match, and a mismatch is a lost update refused (api-guidelines.md).
		expected := cmd.ExpectedVersion
		if expected == 0 {
			expected = before.Version
		}
		if err := h.Groups.Update(ctx, group, expected); err != nil {
			return err
		}

		if cmd.ReplaceMembers {
			if err := h.replaceMembers(ctx, group.ID, cmd.Members); err != nil {
				return err
			}
		}

		group.Version = expected + 1
		updated = group
		return h.recordAudit(ctx, before, group, cmd, actor)
	})
	if err != nil {
		return domain.Group{}, err
	}
	return updated, nil
}

// replaceMembers makes the group's membership exactly the list given. Additions are checked, and
// the removals are whatever is no longer named - which is what "the complete membership" means.
func (h UpdateGroup) replaceMembers(ctx context.Context, groupID shared.ID, members []shared.ID) error {
	current, err := h.Groups.Members(ctx, groupID)
	if err != nil {
		return err
	}

	wanted := map[shared.ID]bool{}
	for _, accountID := range members {
		if !accountID.IsZero() {
			wanted[accountID] = true
		}
	}

	for _, accountID := range current {
		if !wanted[accountID] {
			if err := h.Groups.RemoveMember(ctx, groupID, accountID); err != nil {
				return err
			}
		}
	}
	return addMembers(ctx, h.Groups, h.Accounts, groupID, members)
}

func (h UpdateGroup) recordAudit(
	ctx context.Context, before, after domain.Group, cmd UpdateGroupCommand, actor appshared.ActorContext,
) error {
	changes := []audit.Change{
		{Field: "name", Classification: audit.Sensitive, From: before.Name, To: after.Name},
	}
	if cmd.ReplaceMembers {
		changes = append(changes,
			audit.Change{Field: "members", Classification: audit.Open, To: len(cmd.Members)})
	}

	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   after.TenantID,
		OccurredAt: h.Clock.Now(),
		Action:     GroupUpdatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: groupTarget,
		TargetID:   after.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// Descriptor registers the use case in all three channels.
func (h UpdateGroup) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateGroupName,
		Summary: "Renames a group, changes its description, or replaces who is in it. The member " +
			"list, when given, is the complete membership afterwards rather than an addition.",
		SideEffects: "Writes the group, its members, and an audit entry.",
		TokenScope:  membersWrite,
		Input: []usecase.Field{
			{Name: "group_id", Kind: usecase.KindID, Required: true},
			{Name: "name", Kind: usecase.KindString},
			{Name: "description", Kind: usecase.KindString},
			{
				Name: "members", Kind: usecase.KindIDList,
				Description: "The complete membership after the change. Omitted leaves it as it is.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: GroupUpdatedAction, TargetType: groupTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateGroup) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	groupID, err := in.ID("group_id")
	if err != nil {
		return nil, err
	}
	members, err := in.IDList("members")
	if err != nil {
		return nil, err
	}

	group, err := h.Execute(ctx, actor, UpdateGroupCommand{
		GroupID:     groupID,
		Name:        in.OptionalString("name"),
		Description: in.OptionalString("description"),
		Members:     members,
		// The list is only a replacement when the caller sent one at all. Absent leaves the
		// membership alone; an empty list empties the group, which is a legitimate thing to want.
		ReplaceMembers: in.Present("members"),
	})
	if err != nil {
		return nil, err
	}
	return groupOutput(group), nil
}

// DeleteGroupCommand is the input, typed.
type DeleteGroupCommand struct {
	GroupID shared.ID
}

// DeleteGroup removes a group.
//
// What the group granted, nobody holds afterwards: the memberships go with it, which is the
// database's cascade rather than three statements that could half-run. What its members hold in
// their own right is untouched.
type DeleteGroup struct {
	Groups     repository.Groups
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute deletes the group.
func (h DeleteGroup) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DeleteGroupCommand,
) error {
	if cmd.GroupID.IsZero() {
		return shared.ErrValidation.WithDetail("groups.identifier_required")
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     GroupDeletedAction,
		TokenScope: membersWrite,
		TargetType: groupTarget,
		TargetID:   cmd.GroupID,
	}); err != nil {
		return err
	}

	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		group, err := h.Groups.Find(ctx, cmd.GroupID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.
					WithDetail("groups.not_found").
					WithParams(map[string]string{"group_id": cmd.GroupID.String()})
			}
			return err
		}
		if err := h.Groups.Delete(ctx, group.ID); err != nil {
			return err
		}

		return h.Audit.Append(ctx, audit.Entry{
			TenantID:   group.TenantID,
			OccurredAt: h.Clock.Now(),
			Action:     GroupDeletedAction,
			Outcome:    audit.OutcomeSuccess,
			// Notice: whatever this group granted, nobody holds any more - which is an access
			// change somebody may need to explain later.
			Severity:   audit.SeverityNotice,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: groupTarget,
			TargetID:   group.ID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "name", Classification: audit.Sensitive, From: group.Name},
			),
		})
	})
}

// Descriptor registers the use case in all three channels.
func (h DeleteGroup) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteGroupName,
		Summary: "Deletes a group. Whatever it granted, nobody holds afterwards; what its members " +
			"hold in their own right is untouched.",
		SideEffects: "Removes the group and its memberships, and writes an audit entry.",
		TokenScope:  membersWrite,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "group_id", Kind: usecase.KindID, Required: true},
		},
		Audit: usecase.AuditDeclaration{
			Action: GroupDeletedAction, TargetType: groupTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteGroup) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	groupID, err := in.ID("group_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, DeleteGroupCommand{GroupID: groupID}); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
