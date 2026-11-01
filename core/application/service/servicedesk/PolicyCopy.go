// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// CopySLAPolicyHereName is the catalogue name.
const CopySLAPolicyHereName = "CopySlaPolicyHere"

// CopySLAPolicyHereCommand is the input, typed.
type CopySLAPolicyHereCommand struct {
	SourceKey string
	HubID     shared.ID
	Key       string
	// Name is the copy's own name; nil keeps the source's.
	Name *string
}

// CopySlaPolicyHere makes a hub's own copy of a policy shared with it (UC-SVC-02 check 11): a
// shared policy is read-only at the hub, and a hub that needs it different copies it.
//
//nolint:revive // var-naming: a use case type is named as the catalogue names it
type CopySlaPolicyHere struct{ Desk Desk }

// Execute makes the copy and returns it.
func (h CopySlaPolicyHere) Execute(ctx context.Context, actor appshared.ActorContext, cmd CopySLAPolicyHereCommand) (domain.SLAPolicy, error) {
	if cmd.HubID.IsZero() {
		return domain.SLAPolicy{}, shared.ErrValidation.WithDetail("sla_policies.hub_required").
			WithFields(shared.FieldError{Path: "/hub_id", Code: "sla_policies.hub_required"})
	}
	if err := h.Desk.authorizePolicyAt(ctx, actor, PolicyDefinedAction, cmd.HubID); err != nil {
		return domain.SLAPolicy{}, err
	}
	var copied domain.SLAPolicy
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		source, err := h.Desk.Policies.Find(ctx, cmd.SourceKey)
		// Only what may measure the hub's entries is a source: another hub's is not this one's.
		if errors.Is(err, shared.ErrNotFound) || (err == nil && !source.UsableIn(cmd.HubID)) {
			return shared.ErrNotFound.WithDetail("sla_policies.not_found")
		}
		if err != nil {
			return err
		}
		now := h.Desk.Clock.Now()
		copied, err = source.CopyAt(domain.SLAPolicyCopy{
			ID: h.Desk.IDs.NewID(), TenantID: actor.TenantID, HubID: cmd.HubID, Key: cmd.Key,
			Name: cmd.Name, Now: now, Text: h.Desk.Text,
		})
		if err != nil {
			return err
		}
		if err := h.Desk.Policies.Insert(ctx, copied); err != nil {
			return err
		}
		return h.Desk.record(ctx, actor, PolicyDefinedAction, policyTarget, copied.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, To: copied.Key},
			audit.Change{Field: "copied_from", Classification: audit.Open, To: copied.CopiedFrom},
			audit.Change{Field: "hub_id", Classification: audit.Open, To: copied.HubID.String()})
	})
	if err != nil {
		return domain.SLAPolicy{}, err
	}
	return copied, nil
}

// Descriptor is the catalogue entry.
func (h CopySlaPolicyHere) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CopySLAPolicyHereName,
		Summary: "Makes a hub's own copy of an SLA policy shared with it - the workspace's or the hub's " +
			"own - under a new key, with everything of the source, naming the key it came from. The " +
			"copy measures only the hub's entries and is tried there before the workspace's; the " +
			"source does not change.",
		SideEffects: "Writes the copy and an audit entry. Entries already measured keep their clocks.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "source_key", Kind: usecase.KindString, Required: true, Description: "The key of the policy to copy."},
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub the copy belongs to."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The copy's own key, free in the workspace."},
			{Name: "name", Kind: usecase.KindString, Description: "The copy's name; unset keeps the source's."},
		},
		Audit: usecase.AuditDeclaration{Action: PolicyDefinedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			copied, err := h.Execute(ctx, actor, CopySLAPolicyHereCommand{
				SourceKey: in.String("source_key"), HubID: hub, Key: in.String("key"), Name: in.OptionalString("name"),
			})
			if err != nil {
				return nil, err
			}
			return PolicyOutput(copied), nil
		}),
	}, policyTarget)
}

// policiesFor narrows the policies to those that may measure the entry: the workspace's and its
// hub's own (UC-SVC-02 check 11). Without the containers, the workspace's alone.
func policiesFor(ctx context.Context, containers workrepo.Containers, item work.WorkItem, policies []domain.SLAPolicy) ([]domain.SLAPolicy, error) {
	var hub shared.ID
	if containers != nil {
		collection, err := containers.Find(ctx, item.CollectionID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return nil, err
		}
		hub = collection.ParentID
	}
	return domain.PoliciesWithin(policies, hub), nil
}
