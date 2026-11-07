// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow

import (
	"context"
	"strconv"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// CopyWorkflowHereName is the catalogue name.
const CopyWorkflowHereName = "CopyWorkflowHere"

// CopyWorkflowHereCommand is the input, typed.
type CopyWorkflowHereCommand struct {
	SourceKey string
	HubID     shared.ID
	Key       string
	// Name is the copy's own name; nil keeps the source's.
	Name *string
}

// CopyWorkflowHere makes a hub's own copy of a workflow shared with it (UC-FLW-02 check 9): a
// shared workflow is read-only at the hub, and a hub that needs it different copies it.
type CopyWorkflowHere struct{ Desk Desk }

// Execute makes the copy and returns it.
func (h CopyWorkflowHere) Execute(ctx context.Context, actor appshared.ActorContext, cmd CopyWorkflowHereCommand) (domain.Workflow, error) {
	if cmd.SourceKey == "" {
		return domain.Workflow{}, shared.ErrValidation.WithDetail("workflows.key_required")
	}
	if err := h.Desk.authorizeAt(ctx, actor, WorkflowCreatedAction, cmd.HubID); err != nil {
		return domain.Workflow{}, err
	}
	var copied domain.Workflow
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Desk.requireHub(ctx, cmd.HubID); err != nil {
			return err
		}
		set, err := h.Desk.read(ctx)
		if err != nil {
			return err
		}
		// Only what the hub may run is a source: another hub's workflow is not this one's to see.
		source, found := set.Within(cmd.HubID).Find(cmd.SourceKey)
		if !found {
			return shared.ErrNotFound.WithDetail("workflows.not_found")
		}
		now := h.Desk.Clock.Now()
		copied, err = source.CopyAt(domain.WorkflowCopy{
			ID: h.Desk.IDs.NewID(), TenantID: actor.TenantID, HubID: cmd.HubID, Key: cmd.Key,
			Name: cmd.Name, By: actor.AccountID, Now: now, Text: h.Desk.Text,
		})
		if err != nil {
			return err
		}
		if err := h.Desk.Workflows.Insert(ctx, copied); err != nil {
			return err
		}
		if err := h.Desk.announce(ctx, actor, copied, changelog.Upsert); err != nil {
			return err
		}
		return h.Desk.trail(ctx, actor, WorkflowCreatedAction, copied.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, To: copied.Key},
			audit.Change{Field: "copied_from", Classification: audit.Open, To: copied.CopiedFrom},
			audit.Change{Field: "statuses", Classification: audit.Open, To: strconv.Itoa(len(copied.Published.Statuses))})
	})
	if err != nil {
		return domain.Workflow{}, err
	}
	return copied, nil
}

// Descriptor is the catalogue entry.
func (h CopyWorkflowHere) Descriptor() usecase.Descriptor {
	return workflowDescriptor(CopyWorkflowHereName,
		"Makes a hub's own copy of a workflow shared with it - the workspace's or the hub's own - "+
			"under a new key: the shape in force comes along, not a draft, and the copy names the "+
			"key it came from. Only the hub and its collections may use the copy; the source does "+
			"not change.",
		"Writes the copy, records a change for offline clients, and writes an audit entry.",
		typesWrite, false, WorkflowCreatedAction,
		[]usecase.Field{
			{Name: "source_key", Kind: usecase.KindString, Required: true, Description: "The key of the workflow to copy."},
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub the copy belongs to."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The copy's own key, free in the workspace; not default."},
			{Name: "name", Kind: usecase.KindString, Description: "The copy's name; unset keeps the source's."},
		},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			copied, err := h.Execute(ctx, actor, CopyWorkflowHereCommand{
				SourceKey: in.String("source_key"), HubID: hub, Key: in.String("key"),
				Name: in.OptionalString("name"),
			})
			if err != nil {
				return nil, err
			}
			return WorkflowOutput(copied), nil
		})
}
