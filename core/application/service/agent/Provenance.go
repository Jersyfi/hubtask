// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// ListValueProvenanceName is the use case's name.
const ListValueProvenanceName = "ListValueProvenance"

const getWorkItem = "GetWorkItem"

// ListValueProvenance answers where an entry's AI-originated values came from (UC-AGT-05): each
// acceptance with the fields it wrote, the suggestion, the model and prompt, who accepted it and
// when (ai-first.md §2.1, AI-2). Whoever may read the entry reads it - the entry's own read decides,
// as for its history.
type ListValueProvenance struct{ Desk Desk }

func (h ListValueProvenance) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListValueProvenanceName,
		Summary: "Where an entry's AI-originated values came from: per acceptance, the names of the " +
			"fields it wrote, the suggestion, the model and prompt version, the agent where one " +
			"proposed it, and who accepted it when - kept beyond the suggestion's own retention and " +
			"in the archive. Never a value. Whoever may read the entry reads it.",
		SideEffects: "None. Reads only.",
		TokenScope:  "items:read",
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true},
		},
		Audit:    usecase.AuditDeclaration{Action: ProvenanceReadAction, TargetType: "work_item", Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			// The entry's own read is the permission: one the reader may not read is not found.
			if _, err := d.Catalogue.Invoke(ctx, getWorkItem, actor, usecase.Input{"item_id": id.String()}); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				marks, err := d.Store.ListMarks(ctx, "WORK_ITEM", id, maxListed)
				for _, m := range marks {
					rows = append(rows, MarkOutput(m))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// MarkOutput is a mark as every door answers it.
func MarkOutput(m domain.Mark) usecase.Output {
	fields := m.Fields
	if fields == nil {
		fields = []string{}
	}
	return usecase.Output{
		"id": m.ID.String(), "target_type": m.TargetType, "target_id": m.TargetID.String(), "fields": fields,
		"suggestion_id": idOrNil(m.SuggestionID), "agent_id": idOrNil(m.AgentID), "source": "AI", "model": m.Model,
		"prompt_id": m.PromptID, "prompt_version": m.PromptVersion, "accepted_by": idOrNil(m.AcceptedBy),
		"accepted_at": m.AcceptedAt.UTC(),
	}
}
