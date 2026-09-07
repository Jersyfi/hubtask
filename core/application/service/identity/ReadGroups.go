// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListGroupsName = "ListGroups"
	GetGroupName   = "GetGroup"

	// GroupReadAction is the audit code of an attempted read. Declared even though an ordinary
	// read writes no entry: a read refused for its token scope is recorded against it (audit.md §4).
	GroupReadAction audit.Action = "group.read"
)

// ListGroups answers the workspace's groups, by name (F3-01).
//
// Any member of the workspace may read them, and the rule is written here rather than decided in
// a controller. It is the rule `GetAccount` records, reached the same way: a membership granted
// to a group is unreadable until the group can be shown as the people it reaches, and a member
// scoped to one hub holds nothing at the workspace that `READ` there could be judged against -
// requiring it would leave exactly the members screen F3-07 builds without its groups. What a
// group discloses is its name and its members' identifiers, and the identifiers resolve through
// the same minimal read (`data-protection.md` §9). The token scope still applies (ADR-0005), and
// the tenant boundary is the transaction's (ADR-0010).
//
// Read-only throughout: the transaction may be served by a replica (multi-tenancy.md §7).
type ListGroups struct {
	Groups     repository.Groups
	UnitOfWork persistence.UnitOfWork
}

// ListGroupsQuery is the input, typed.
type ListGroupsQuery struct {
	Cursor string
	Size   int
}

// Execute returns one page of the groups.
func (h ListGroups) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListGroupsQuery,
) (repository.GroupPage, error) {
	if err := actor.RequireScope(membersRead); err != nil {
		return repository.GroupPage{}, err
	}

	var page repository.GroupPage
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Groups.List(ctx, repository.Page{Cursor: query.Cursor, Size: pageSize(query.Size)})
		return err
	})
	if err != nil {
		return repository.GroupPage{}, err
	}
	return page, nil
}

// Descriptor registers the use case in all three channels.
func (h ListGroups) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListGroupsName,
		Summary: "Lists the workspace's groups by name. Any member may read them, for the reason " +
			"any member may resolve an account identifier to a name: a role granted to a group " +
			"is unreadable until the group can be shown as the people it reaches. The members " +
			"are getGroup's answer.",
		SideEffects: "None. Reads only.",
		TokenScope:  membersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The opaque cursor of the previous page. Omitted starts at the first name.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "How many groups to return. Clamped to the contract's maximum.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: GroupReadAction, TargetType: groupTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListGroups) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	page, err := h.Execute(ctx, actor, ListGroupsQuery{Cursor: in.String("cursor"), Size: in.Int("size")})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Groups))
	for _, group := range page.Groups {
		rows = append(rows, groupOutput(group))
	}
	return pageOutput(rows, page.Info), nil
}

// GetGroup answers one group with who is in it (F3-01). Who may read it is who may list them -
// see ListGroups. The members are identifiers rather than names, for the reason every record
// that says who is: the name is one request away, and a copy of it should not outlive the row.
type GetGroup struct {
	Groups     repository.Groups
	UnitOfWork persistence.UnitOfWork
}

// GroupDetail is the group and the accounts in it.
type GroupDetail struct {
	Group   domain.Group
	Members []shared.ID
}

// Execute returns the group and its members.
func (h GetGroup) Execute(
	ctx context.Context, actor appshared.ActorContext, groupID shared.ID,
) (GroupDetail, error) {
	if err := actor.RequireScope(membersRead); err != nil {
		return GroupDetail{}, err
	}
	if groupID.IsZero() {
		return GroupDetail{}, shared.ErrValidation.WithDetail("groups.identifier_required")
	}

	var detail GroupDetail
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		group, err := h.Groups.Find(ctx, groupID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.
					WithDetail("groups.not_found").
					WithParams(map[string]string{"group_id": groupID.String()})
			}
			return err
		}
		members, err := h.Groups.Members(ctx, group.ID)
		if err != nil {
			return err
		}
		detail = GroupDetail{Group: group, Members: members}
		return nil
	})
	if err != nil {
		return GroupDetail{}, err
	}
	return detail, nil
}

// Descriptor registers the use case in all three channels.
func (h GetGroup) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetGroupName,
		Summary: "Reads one group with the identifiers of its members, so that a role granted to " +
			"the group can be shown as the people it reaches. Resolve a member through getAccount.",
		SideEffects: "None. Reads only.",
		TokenScope:  membersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "group_id", Kind: usecase.KindID, Required: true},
		},
		Audit: usecase.AuditDeclaration{
			Action: GroupReadAction, TargetType: groupTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetGroup) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	groupID, err := in.ID("group_id")
	if err != nil {
		return nil, err
	}
	detail, err := h.Execute(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}

	out := groupOutput(detail.Group)
	members := make([]string, 0, len(detail.Members))
	for _, member := range detail.Members {
		members = append(members, member.String())
	}
	out["members"] = members
	return out, nil
}
