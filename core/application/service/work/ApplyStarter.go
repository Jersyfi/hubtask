// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"fmt"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const ApplyStarterName = "ApplyStarter"

// CodeStarterClosed refuses a starter set where the first start was answered or skipped already,
// or the workspace holds something somebody made.
const CodeStarterClosed = "workspace.starter_closed"

// CodeStarterCollectionsTooMany refuses a set of more collections than maxStarterCollections.
const CodeStarterCollectionsTooMany = "workspace.starter_collections_too_many"

// maxStarterCollections bounds the set, as the contract does.
const maxStarterCollections = 10

// StarterPlace is one hub or collection of a starter set: its name in the reader's language and
// the preset it starts from; a collection without one follows its hub.
type StarterPlace struct {
	Name   string
	Preset domain.PresetKey
}

// ApplyStarterCommand is the set: one hub and its collections, in their order.
type ApplyStarterCommand struct {
	Hub         StarterPlace
	Collections []StarterPlace
}

// ApplyStarter answers the first start with its set (UC-WRK-30 check 5). The hub, the collections
// and the close are one transaction: a failure part way leaves nothing behind and the question
// still open, and a set that landed closes it, so a second attempt - another key, another tab -
// is refused rather than making a second set. A repeat of the same request under its
// Idempotency-Key replays the hub (api-guidelines.md §5).
type ApplyStarter struct {
	Starter    Starter
	Containers CreateContainer
}

// Execute makes the set and answers its hub, with the hub's structure.
func (h ApplyStarter) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ApplyStarterCommand,
) (domain.Container, *domain.EffectiveStructure, error) {
	if err := validStarterSet(cmd); err != nil {
		return domain.Container{}, nil, err
	}
	s := h.Starter
	if err := s.Authorizer.Authorize(ctx, actor, structureAtWorkspace(StarterClosedAction, actor)); err != nil {
		return domain.Container{}, nil, err
	}
	var hub domain.Container
	var structure *domain.EffectiveStructure
	err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		pending, err := s.Starts.Pending(ctx)
		if err != nil {
			return err
		}
		if !pending {
			return shared.ErrConflict.WithDetail(CodeStarterClosed)
		}
		now := s.Clock.Now()
		// Closed first inside the transaction: the row lock makes a second set arriving at once
		// wait for this one, and then find the question closed.
		closed, err := s.Starts.Close(ctx, now)
		if err != nil {
			return err
		}
		if !closed {
			return shared.ErrConflict.WithDetail(CodeStarterClosed)
		}
		if hub, structure, err = h.Containers.create(ctx, actor, CreateContainerCommand{
			Type: domain.ContainerHub, Name: cmd.Hub.Name, Preset: cmd.Hub.Preset,
		}); err != nil {
			return err
		}
		for _, place := range cmd.Collections {
			if _, _, err := h.Containers.create(ctx, actor, CreateContainerCommand{
				Type: domain.ContainerCollection, ParentID: hub.ID, Name: place.Name, Preset: place.Preset,
			}); err != nil {
				return err
			}
		}
		return s.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: now,
			Action:     StarterClosedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityInfo,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: workspaceStructureTarget,
			TargetID:   actor.TenantID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	})
	if err != nil {
		return domain.Container{}, nil, err
	}
	return hub, structure, nil
}

// validStarterSet refuses a set the contract does not describe before anything is read: only
// Hubtask's own presets, and at most maxStarterCollections collections.
func validStarterSet(cmd ApplyStarterCommand) error {
	if !cmd.Hub.Preset.BuiltIn() {
		return invalidStarterField("/hub_preset", "usecase.field_not_in_enum")
	}
	if len(cmd.Collections) > maxStarterCollections {
		return invalidStarterField("/collections", CodeStarterCollectionsTooMany)
	}
	for i, place := range cmd.Collections {
		if place.Preset != "" && !place.Preset.BuiltIn() {
			return invalidStarterField("/collections/"+strconv.Itoa(i)+"/preset", "usecase.field_not_in_enum")
		}
	}
	return nil
}

func invalidStarterField(path, code string) error {
	return shared.ErrValidation.WithDetail("usecase.input_invalid").
		WithFields(shared.FieldError{Path: path, Code: code})
}

// starterPlacesFrom reads the collections of the input: a list of {name, preset}.
func starterPlacesFrom(raw any) ([]StarterPlace, error) {
	if raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, invalidStarterField("/collections", "usecase.field_type_invalid")
	}
	places := make([]StarterPlace, 0, len(list))
	for i, each := range list {
		object, ok := each.(map[string]any)
		name, named := object["name"].(string)
		if !ok || !named {
			return nil, invalidStarterField(fmt.Sprintf("/collections/%d/name", i), "usecase.field_required")
		}
		preset, _ := object["preset"].(string)
		places = append(places, StarterPlace{Name: name, Preset: domain.PresetKey(preset)})
	}
	return places, nil
}

// Descriptor is the catalogue entry.
func (h ApplyStarter) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionStructure,
		Name:       ApplyStarterName,
		Summary: "Answers the workspace's first start with its starter set: creates a hub and its " +
			"collections, each on its preset, and closes the first start, all in one transaction. " +
			"Refused with " + CodeStarterClosed + " once the first start was answered or skipped, or " +
			"the workspace holds anything somebody made. Needs the right to change the workspace's shape.",
		SideEffects: "Writes the hub and each collection as CreateContainer does - events, changes, " +
			"columns and audit entries - the moment the first start was closed, and its audit entry.",
		TokenScope: workspaceManageScope,
		Input: []usecase.Field{
			{Name: "hub_name", Kind: usecase.KindString, Required: true, Description: "Up to 200 characters, on one line."},
			{
				Name: "hub_preset", Kind: usecase.KindString, Required: true,
				Enum: []string{
					string(domain.PresetList), string(domain.PresetTasks),
					string(domain.PresetProject), string(domain.PresetFlow),
				},
				Description: "The preset the hub starts from.",
			},
			{
				Name: "collections", Kind: usecase.KindList,
				Description: "The hub's collections, in their order: {name, preset}; a collection " +
					"without a preset follows its hub. At most ten.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: StarterClosedAction, TargetType: workspaceStructureTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the workspace and its containers are not items, and the history is an item's: " +
				"`ActivityEntry` is keyed on `itemId`. The set is in the audit trail.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ApplyStarter) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collections, err := starterPlacesFrom(in["collections"])
	if err != nil {
		return nil, err
	}
	hub, structure, err := h.Execute(ctx, actor, ApplyStarterCommand{
		Hub:         StarterPlace{Name: in.String("hub_name"), Preset: domain.PresetKey(in.String("hub_preset"))},
		Collections: collections,
	})
	if err != nil {
		return nil, err
	}
	out := containerOutput(hub)
	if structure != nil {
		out["structure"] = effectiveStructureOutput(*structure)
	}
	return out, nil
}
