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

// CodeStarterClosed refuses a starter set where the first start was answered already, or the
// workspace holds something somebody made.
const CodeStarterClosed = "workspace.starter_closed"

// CodeStarterCollectionsTooMany refuses a set of more collections than maxStarterCollections.
const CodeStarterCollectionsTooMany = "workspace.starter_collections_too_many"

// CodeStarterSamplesTooMany refuses a collection of more sample entries than maxStarterSamples.
const CodeStarterSamplesTooMany = "workspace.starter_samples_too_many"

// maxStarterCollections bounds the set, as the contract does.
const maxStarterCollections = 10

// maxStarterSamples bounds a collection's sample entries, as the contract does.
const maxStarterSamples = 5

// StarterPlace is one hub or collection of a starter set: its name in the reader's language and
// the preset it starts from; a collection without one follows its hub. A collection's Samples are
// the titles of its sample entries, in the reader's language too.
type StarterPlace struct {
	Name    string
	Preset  domain.PresetKey
	Samples []string
}

// ApplyStarterCommand is the set: one hub and its collections, in their order, and the template
// it is built on, if any.
type ApplyStarterCommand struct {
	Hub         StarterPlace
	Collections []StarterPlace
	Template    StarterTemplate
}

// StarterTemplate is the template a set is built on: its key, its questions answered, whether its
// sample entries come along, and whether the person has seen a plan that hides data or changes
// permissions. A zero Key builds the set on presets alone.
type StarterTemplate struct {
	Key         string
	Answers     map[string]string
	Samples     bool
	Acknowledge bool
}

// StarterTemplates takes a template's parts for the first start inside the transaction it is
// called in, through the configuration's own plan and apply (configuration.md §6): the workspace
// part before the hub exists, the hub part once it does. Each refuses as the apply does.
type StarterTemplates interface {
	TakeForWorkspace(ctx context.Context, actor appshared.ActorContext, take StarterTemplate) error
	TakeForHub(ctx context.Context, actor appshared.ActorContext, take StarterTemplate, hub shared.ID) error
}

// StarterSamples makes a collection's sample entries, marked as the hub's samples so they go in
// one press, inside the transaction it is called in.
type StarterSamples interface {
	AddStarterSamples(ctx context.Context, actor appshared.ActorContext, hub, collection shared.ID, index int, titles []string) error
}

// StarterBaseline records the configuration as the first start leaves it - the workspace's, and the
// hub's it made - as a version (configuration.md §5), inside the transaction it is called in.
type StarterBaseline interface {
	Baseline(ctx context.Context, actor appshared.ActorContext, hub shared.ID) error
}

// ApplyStarter answers the first start with its set (UC-WRK-30 check 5). The hub, the collections
// and the close are one transaction: a failure part way leaves nothing behind and the question
// still open, and a set that landed closes it, so a second attempt - another key, another tab -
// is refused rather than making a second set. A question skipped while nothing was made takes
// the set too: Today offers it again (user-experience.md §3.3). A repeat of the same request under its
// Idempotency-Key replays the hub (api-guidelines.md §5).
type ApplyStarter struct {
	Starter    Starter
	Containers CreateContainer
	// Templates takes a set's template; a set naming one is refused where it is not wired.
	Templates StarterTemplates
	// Samples makes the collections' sample entries; a set naming one is refused where it is not wired.
	Samples StarterSamples
}

// Execute makes the set and answers its hub, with the hub's structure.
func (h ApplyStarter) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ApplyStarterCommand,
) (domain.Container, *domain.EffectiveStructure, error) {
	if err := validStarterSet(cmd, h.Templates, h.Samples); err != nil {
		return domain.Container{}, nil, err
	}
	s := h.Starter
	if err := s.Authorizer.Authorize(ctx, actor, structureAtWorkspace(StarterClosedAction, actor)); err != nil {
		return domain.Container{}, nil, err
	}
	var hub domain.Container
	var structure *domain.EffectiveStructure
	err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// The row lock first: a second set arriving at once waits for this one, and then finds
		// the hub it made - whether the question was open or skipped before.
		if err := s.Starts.Hold(ctx); err != nil {
			return err
		}
		open, err := s.Starts.Pending(ctx)
		if err != nil {
			return err
		}
		if !open {
			// A skip made nothing: the set may still be taken while nothing else was made
			// (user-experience.md §3.2). An answer made a hub, so it never reads as skipped.
			if open, err = s.Starts.Skipped(ctx); err != nil {
				return err
			}
		}
		if !open {
			return shared.ErrConflict.WithDetail(CodeStarterClosed)
		}
		now := s.Clock.Now()
		if _, err := s.Starts.Close(ctx, now); err != nil {
			return err
		}
		if cmd.Template.Key != "" {
			if err := h.Templates.TakeForWorkspace(ctx, actor, cmd.Template); err != nil {
				return err
			}
		}
		if hub, structure, err = h.Containers.create(ctx, actor, CreateContainerCommand{
			Type: domain.ContainerHub, Name: cmd.Hub.Name, Preset: cmd.Hub.Preset,
		}); err != nil {
			return err
		}
		for i, place := range cmd.Collections {
			collection, _, err := h.Containers.create(ctx, actor, CreateContainerCommand{
				Type: domain.ContainerCollection, ParentID: hub.ID, Name: place.Name, Preset: place.Preset,
			})
			if err != nil {
				return err
			}
			if len(place.Samples) > 0 {
				if err := h.Samples.AddStarterSamples(ctx, actor, hub.ID, collection.ID, i, place.Samples); err != nil {
					return err
				}
			}
		}
		if cmd.Template.Key != "" {
			if err := h.Templates.TakeForHub(ctx, actor, cmd.Template, hub.ID); err != nil {
				return err
			}
		}
		if s.Baseline != nil {
			if err := s.Baseline.Baseline(ctx, actor, hub.ID); err != nil {
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
func validStarterSet(cmd ApplyStarterCommand, templates StarterTemplates, samples StarterSamples) error {
	if cmd.Template.Key != "" && templates == nil {
		return invalidStarterField("/template", "usecase.field_not_in_enum")
	}
	// With a template the hub may start from the workspace's default: its hub part shapes it.
	if !cmd.Hub.Preset.BuiltIn() && (cmd.Template.Key == "" || cmd.Hub.Preset != "") {
		return invalidStarterField("/hub_preset", "usecase.field_not_in_enum")
	}
	if len(cmd.Collections) > maxStarterCollections {
		return invalidStarterField("/collections", CodeStarterCollectionsTooMany)
	}
	for i, place := range cmd.Collections {
		at := "/collections/" + strconv.Itoa(i)
		if place.Preset != "" && !place.Preset.BuiltIn() {
			return invalidStarterField(at+"/preset", "usecase.field_not_in_enum")
		}
		if len(place.Samples) > 0 && samples == nil {
			return invalidStarterField(at+"/samples", "usecase.field_not_in_enum")
		}
		if len(place.Samples) > maxStarterSamples {
			return invalidStarterField(at+"/samples", CodeStarterSamplesTooMany)
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
		samples, err := starterSamplesFrom(object["samples"], i)
		if err != nil {
			return nil, err
		}
		places = append(places, StarterPlace{Name: name, Preset: domain.PresetKey(preset), Samples: samples})
	}
	return places, nil
}

// starterSamplesFrom reads a collection's sample titles: a list of text.
func starterSamplesFrom(raw any, collection int) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	path := fmt.Sprintf("/collections/%d/samples", collection)
	list, ok := raw.([]any)
	if !ok {
		return nil, invalidStarterField(path, "usecase.field_type_invalid")
	}
	titles := make([]string, 0, len(list))
	for i, each := range list {
		title, ok := each.(string)
		if !ok {
			return nil, invalidStarterField(fmt.Sprintf("%s/%d", path, i), "usecase.field_type_invalid")
		}
		titles = append(titles, title)
	}
	return titles, nil
}

// starterAnswersFrom reads the template's answers: text by the question's key.
func starterAnswersFrom(raw any) (map[string]string, error) {
	if raw == nil {
		return nil, nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, invalidStarterField("/answers", "usecase.field_type_invalid")
	}
	answers := make(map[string]string, len(object))
	for key, value := range object {
		text, ok := value.(string)
		if !ok {
			return nil, invalidStarterField("/answers/"+key, "usecase.field_type_invalid")
		}
		answers[key] = text
	}
	return answers, nil
}

// Descriptor is the catalogue entry.
func (h ApplyStarter) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionStructure,
		Name:       ApplyStarterName,
		Summary: "Answers the workspace's first start with its starter set: creates a hub and its " +
			"collections, each on its preset - with a template, its workspace and hub parts as " +
			"ApplyConfig would apply them - and closes the first start, all in one transaction. " +
			"Refused with " + CodeStarterClosed + " once the first start was answered, or the workspace " +
			"holds anything somebody made; a skipped first start still takes the set. Needs the right to change the workspace's shape.",
		SideEffects: "Writes the hub and each collection as CreateContainer does - events, changes, " +
			"columns and audit entries - the moment the first start was closed, and its audit entry; " +
			"with a template, the configuration versions and audit entries ApplyConfig writes; and the " +
			"workspace's and the hub's configuration as the start left them as a version where it moved.",
		TokenScope: workspaceManageScope,
		Input: []usecase.Field{
			{Name: "hub_name", Kind: usecase.KindString, Required: true, Description: "Up to 200 characters, on one line."},
			{
				Name: "hub_preset", Kind: usecase.KindString,
				Enum: []string{
					string(domain.PresetList), string(domain.PresetTasks),
					string(domain.PresetProject), string(domain.PresetFlow),
				},
				Description: "The preset the hub starts from; required without a template, which " +
					"otherwise starts the hub from the workspace's default.",
			},
			{
				Name: "collections", Kind: usecase.KindList,
				Description: "The hub's collections, in their order: {name, preset, samples}; a " +
					"collection without a preset follows its hub, and its samples are the titles of " +
					"up to five sample entries made in it, marked as the hub's samples. At most ten.",
			},
			{
				Name: "template", Kind: usecase.KindString,
				Description: "A template the set is built on, by key: its workspace part is applied " +
					"first, then the hub takes its hub part, in the same transaction.",
			},
			{Name: "answers", Kind: usecase.KindObject, Description: "The template's questions answered, by key."},
			{Name: "samples", Kind: usecase.KindBool, Description: "True takes the template's sample entries into the hub."},
			{
				Name: "acknowledge", Kind: usecase.KindBool,
				Description: "True once the person has seen that the template's plan hides data or changes permissions.",
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
	answers, err := starterAnswersFrom(in["answers"])
	if err != nil {
		return nil, err
	}
	hub, structure, err := h.Execute(ctx, actor, ApplyStarterCommand{
		Hub:         StarterPlace{Name: in.String("hub_name"), Preset: domain.PresetKey(in.String("hub_preset"))},
		Collections: collections,
		Template: StarterTemplate{
			Key: in.String("template"), Answers: answers,
			Samples: in.Bool("samples"), Acknowledge: in.Bool("acknowledge"),
		},
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
