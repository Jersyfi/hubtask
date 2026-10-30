// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListWorkTypesName  = "ListWorkTypes"
	DefineWorkTypeName = "DefineWorkType"
	ReadTimePolicyName = "ReadTimePolicy"
	SetTimePolicyName  = "SetTimePolicy"
)

// ListWorkTypes answers the workspace's kinds of work, to anybody who records time.
type ListWorkTypes struct{ Keeper Keeper }

// Execute reads them.
func (h ListWorkTypes) Execute(ctx context.Context, actor appshared.ActorContext) ([]domain.WorkType, error) {
	if err := person(actor); err != nil {
		return nil, err
	}
	var kinds []domain.WorkType
	err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		kinds, err = h.Keeper.Store.WorkTypes(ctx)
		return err
	})
	return kinds, err
}

// WorkTypeOutput is a kind of work on the wire.
func WorkTypeOutput(w domain.WorkType) usecase.Output {
	return usecase.Output{"key": w.Key, "name": w.Name, "billable": w.Billable, "archived": w.Archived, "position": w.Position, "version": w.Version}
}

// Descriptor is the catalogue entry.
func (h ListWorkTypes) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListWorkTypesName, Summary: "Lists the workspace's kinds of work, archived ones included, in their order.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: WorkTypesReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			kinds, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(kinds))
			for _, w := range kinds {
				data = append(data, WorkTypeOutput(w))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, workTypeTarget)
}

// DefineWorkType writes one of the workspace's kinds of work by its key; archiving one keeps the
// records of it and offers it no more.
type DefineWorkType struct{ Keeper Keeper }

// Execute writes it.
func (h DefineWorkType) Execute(ctx context.Context, actor appshared.ActorContext, w domain.WorkType) (domain.WorkType, error) {
	w.Name = strings.TrimSpace(w.Name)
	if err := domain.CheckWorkType(w); err != nil {
		return domain.WorkType{}, err
	}
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope()},
		Action: WorkTypeDefinedAction, TokenScope: workspaceManage, TargetType: workTypeTarget,
	}); err != nil {
		return domain.WorkType{}, err
	}
	now := h.Keeper.Clock.Now()
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		kinds, err := h.Keeper.Store.WorkTypes(ctx)
		if err != nil {
			return err
		}
		known := false
		for _, k := range kinds {
			known = known || k.Key == w.Key
		}
		if !known && len(kinds) >= domain.MaxWorkTypes {
			return shared.ErrValidation.WithDetail("work_types.too_many").
				WithParams(map[string]string{"maximum": strconv.Itoa(domain.MaxWorkTypes)})
		}
		if err := h.Keeper.Store.PutWorkType(ctx, h.Keeper.IDs.NewID(), w, now); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, WorkTypeDefinedAction, workTypeTarget, "", now,
			audit.Change{Field: "key", Classification: audit.Open, To: w.Key},
			audit.Change{Field: "billable", Classification: audit.Open, To: strconv.FormatBool(w.Billable)},
			audit.Change{Field: "archived", Classification: audit.Open, To: strconv.FormatBool(w.Archived)})
	})
	return w, err
}

// Descriptor is the catalogue entry.
func (h DefineWorkType) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineWorkTypeName, Permission: service.PermissionStructure,
		Summary: "Writes one of the workspace's kinds of work - development, meeting, support - by its key, at most " +
			"100: its name, whether its time starts billable, its place in the order, and whether it is archived " +
			"(kept on its records, offered no more). Needs STRUCTURE at the workspace.",
		SideEffects: "Writes the kind of work and an audit entry.",
		TokenScope:  workspaceManage,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Lowercase letters, digits and underscores, at most 40."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "One line of at most 80 characters."},
			{Name: "billable", Kind: usecase.KindBool, Description: "Whether its time starts billable."},
			{Name: "archived", Kind: usecase.KindBool, Description: "Offered no more."},
			{Name: "position", Kind: usecase.KindAny, Description: "Its place in the order, smallest first."},
		},
		Audit: usecase.AuditDeclaration{Action: WorkTypeDefinedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			position, _ := intField(in, "position")
			billable, _ := in["billable"].(bool)
			archived, _ := in["archived"].(bool)
			w, err := h.Execute(ctx, actor, domain.WorkType{
				Key: in.String("key"), Name: in.String("name"), Billable: billable, Archived: archived, Position: position,
			})
			if err != nil {
				return nil, err
			}
			return WorkTypeOutput(w), nil
		}),
	}, workTypeTarget)
}

// ReadTimePolicy answers the workspace's locked period and what a lead sees of other people's
// time: to everybody who records time, because both decide what they may write and who reads it.
type ReadTimePolicy struct{ Keeper Keeper }

// Execute reads it.
func (h ReadTimePolicy) Execute(ctx context.Context, actor appshared.ActorContext) (domain.Policy, error) {
	if err := person(actor); err != nil {
		return domain.Policy{}, err
	}
	return h.Keeper.policy(ctx, actor)
}

// PolicyOutput is the policy on the wire.
func PolicyOutput(p domain.Policy) usecase.Output {
	out := usecase.Output{"locked_through": nil, "visibility": string(p.Visibility), "version": p.Version}
	if p.LockedThrough != nil {
		out["locked_through"] = p.LockedThrough.String()
	}
	return out
}

// Descriptor is the catalogue entry.
func (h ReadTimePolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadTimePolicyName,
		Summary: "Answers the workspace's time policy: the last locked day, before and on which no record changes, " +
			"and whether other people's time is seen in FULL or only AGGREGATED into sums of at least three people.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: PolicyReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			p, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			return PolicyOutput(p), nil
		}),
	}, policyTarget)
}

// SetTimePolicy locks a period for everybody and chooses what a lead sees (UC-TIM-03): whoever
// approves time at the workspace sets it.
type SetTimePolicy struct{ Keeper Keeper }

// Execute writes the policy held to the version read.
func (h SetTimePolicy) Execute(ctx context.Context, actor appshared.ActorContext, p domain.Policy, expected int) (domain.Policy, error) {
	if !p.Visibility.Valid() {
		return domain.Policy{}, invalid("/visibility", "time.visibility_unknown")
	}
	request := approverRequest(PolicySetAction, workspaceManage)
	request.Path, request.TargetType = []identity.Scope{identity.TenantScope()}, policyTarget
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, request); err != nil {
		return domain.Policy{}, err
	}
	now := h.Keeper.Clock.Now()
	p.UpdatedAt, p.UpdatedBy = now, actor.AccountID
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := h.Keeper.Store.Policy(ctx)
		if err != nil {
			return err
		}
		if before.Version != expected {
			return shared.ErrVersionConflict.WithDetail("time.policy_version_conflict").
				WithParams(map[string]string{"expected_version": strconv.Itoa(expected)})
		}
		if err := h.Keeper.Store.PutPolicy(ctx, h.Keeper.IDs.NewID(), p, expected); err != nil {
			return err
		}
		p.Version = expected + 1
		return h.Keeper.record(ctx, actor, PolicySetAction, policyTarget, "", now,
			audit.Change{Field: "locked_through", Classification: audit.Open, From: dayText(before.LockedThrough), To: dayText(p.LockedThrough)},
			audit.Change{Field: "visibility", Classification: audit.Open, From: string(before.Visibility), To: string(p.Visibility)})
	})
	return p, err
}

func dayText(d *domain.Day) string {
	if d == nil {
		return ""
	}
	return d.String()
}

// Descriptor is the catalogue entry.
func (h SetTimePolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetTimePolicyName, Permission: service.PermissionApproveTime,
		Summary: "Sets the workspace's time policy: the last locked day (none unlocks every day), before and on " +
			"which nobody writes, changes or removes a record, and FULL or AGGREGATED visibility of other people's " +
			"time. Needs APPROVE_TIME or MANAGE_MEMBERS at the workspace.",
		SideEffects: "Writes the policy and an audit entry.",
		TokenScope:  workspaceManage,
		Input: []usecase.Field{
			{Name: "locked_through", Kind: usecase.KindString, Description: "The last locked day, YYYY-MM-DD; none for no locked period."},
			{Name: "visibility", Kind: usecase.KindString, Required: true, Enum: []string{"FULL", "AGGREGATED"}, Description: "What an approver sees of other people's time."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read; 0 before the first."},
		},
		Audit: usecase.AuditDeclaration{Action: PolicySetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			expected, ok := intField(in, "expected_version")
			if !ok {
				return nil, invalid("/expected_version", "time.version_required")
			}
			p := domain.Policy{Visibility: domain.Visibility(in.String("visibility"))}
			if in.String("locked_through") != "" {
				d, err := dayField(in, "locked_through", true)
				if err != nil {
					return nil, err
				}
				p.LockedThrough = &d
			}
			out, err := h.Execute(ctx, actor, p, expected)
			if err != nil {
				return nil, err
			}
			return PolicyOutput(out), nil
		}),
	}, policyTarget)
}
