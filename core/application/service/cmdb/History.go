// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// ListCiHistoryName is the use case's name (UC-AST-01).
const ListCiHistoryName = "ListCiHistory"

// The page sizes of api-guidelines.md §4.
const (
	historyPageSize    = 50
	maxHistoryPageSize = 200
)

// chronicle writes one step of an object's own history in the caller's transaction. It is the
// product's record, not the trail: field names, and the values of stages, people, places, objects
// and files; never an attribute's value. A nil History records nothing, a test's convenience.
func (s Inventory) chronicle(ctx context.Context, actor appshared.ActorContext, ciID shared.ID, verb domain.Verb, changes map[string]domain.Moved) error {
	if s.History == nil {
		return nil
	}
	kind := actor.Kind
	if kind == "" || kind == shared.ActorAnonymous {
		kind = shared.ActorSystem
	}
	step, err := domain.NewStep(s.IDs.NewID(), ciID, kind, actor.AccountID, verb, changes, s.Clock.Now())
	if err != nil {
		return err
	}
	return s.History.AppendCIHistory(ctx, step)
}

// sourceSide names the source that wrote, or none for a person's hand.
func sourceSide(source string, fields []string) map[string]domain.Moved {
	changes := map[string]domain.Moved{}
	if source != "" && source != domain.ManualSource {
		changes["source"] = domain.Moved{To: source}
	}
	for _, field := range fields {
		changes[field] = domain.Moved{Changed: true}
	}
	return changes
}

// chronicleChange writes what a change of an object moved: its stage as one step, its owner,
// place, protection need, review and quantity as another, the rest - name, class, attributes - as
// changed by hand or by the source.
func (s Inventory) chronicleChange(ctx context.Context, actor appshared.ActorContext, before, after domain.CI, source string, fields []string) error {
	if changes, moved := domain.StageStep(before, after); moved {
		if err := s.chronicle(ctx, actor, after.ID, domain.VerbStageChanged, changes); err != nil {
			return err
		}
	}
	changes, moved := domain.ComplianceStep(before, after)
	var rest []string
	for _, f := range fields {
		if f != domain.FieldLifecycle && f != domain.FieldCompliance && !isComplianceField(f) {
			rest = append(rest, f)
		}
	}
	if !moved && len(rest) == 0 {
		return nil
	}
	for field, m := range sourceSide(source, rest) {
		changes[field] = m
	}
	return s.chronicle(ctx, actor, after.ID, domain.VerbChanged, changes)
}

func isComplianceField(f string) bool {
	switch f {
	case "owner_account_id", "owner_group_id", "location_id", "protection", "review_due", "quantity":
		return true
	}
	return false
}

// chronicleRelation writes a relation made or removed on both of its objects, each naming the
// other end.
func (s Inventory) chronicleRelation(ctx context.Context, actor appshared.ActorContext, r domain.Relation, verb domain.Verb) error {
	for _, side := range []struct{ here, there shared.ID }{{r.SourceCI, r.TargetCI}, {r.TargetCI, r.SourceCI}} {
		changes := map[string]domain.Moved{"type": {To: r.Type}, "other_ci_id": {To: side.there.String()}}
		if side.here == r.TargetCI {
			changes["direction"] = domain.Moved{To: "INCOMING"}
		}
		if verb == domain.VerbUnrelated {
			changes["type"] = domain.Moved{From: r.Type}
			changes["other_ci_id"] = domain.Moved{From: side.there.String()}
		}
		if err := s.chronicle(ctx, actor, side.here, verb, changes); err != nil {
			return err
		}
	}
	return nil
}

// ListCiHistory reads an object's own history, newest first.
type ListCiHistory struct{ Inventory Inventory }

// Execute reads one page for a reader of the object's hub.
func (h ListCiHistory) Execute(ctx context.Context, actor appshared.ActorContext, ciID shared.ID, cursor string, size int) ([]domain.Step, workrepo.PageInfo, error) {
	s := h.Inventory
	ci, err := s.placeOf(ctx, actor, ciID)
	if err != nil {
		return nil, workrepo.PageInfo{}, err
	}
	if err := s.authorizeHub(ctx, actor, ci.HubID, false, CIReadAction, ciTarget, ciID); err != nil {
		return nil, workrepo.PageInfo{}, err
	}
	if s.History == nil {
		return nil, workrepo.PageInfo{}, nil
	}
	if size <= 0 {
		size = historyPageSize
	}
	var steps []domain.Step
	var info workrepo.PageInfo
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		page, err := s.History.CIHistory(ctx, ciID, workrepo.Page{Cursor: cursor, Size: min(size, maxHistoryPageSize)})
		steps, info = page.Steps, page.Info
		return err
	})
	return steps, info, err
}

// StepOutput is a step as every door returns it (schema CiHistoryStep).
func StepOutput(step domain.Step) usecase.Output {
	set := usecase.Output{}
	for field, m := range step.Changes {
		moved := usecase.Output{}
		if m.From != "" {
			moved["from"] = m.From
		}
		if m.To != "" {
			moved["to"] = m.To
		}
		if m.Changed {
			moved["changed"] = true
		}
		set[field] = moved
	}
	return usecase.Output{
		"id": step.ID.String(), "ci_id": step.CIID.String(), "code": step.Verb.Code(),
		"actor":       usecase.Output{"type": string(step.ActorKind), "id": idOrNil(step.ActorID)},
		"occurred_at": step.At, "change_set": set,
	}
}

// Descriptor is the catalogue entry.
func (h ListCiHistory) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiHistoryName, Permission: service.PermissionRead,
		Summary: "Reads a configuration item's own history, newest first: made, changed by hand or by a source, its " +
			"stage and responsibility changed, related, confirmed, attested, moved, files put on and taken off. The " +
			"product's record, not the audit trail: readable by whoever reads the hub, deleted with the object.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
			{Name: "cursor", Kind: usecase.KindString, Description: "The opaque cursor of the previous page."},
			{Name: "size", Kind: usecase.KindInt, Description: "How many steps to return, at most 200."},
		},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			steps, info, err := h.Execute(ctx, actor, id, in.String("cursor"), in.Int("size"))
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(steps))
			for _, step := range steps {
				data = append(data, StepOutput(step))
			}
			page := map[string]any{"next_cursor": nil, "has_more": info.HasMore}
			if info.NextCursor != "" {
				page["next_cursor"] = info.NextCursor
			}
			return usecase.Output{"data": data, "page": page}, nil
		}),
	})
}
