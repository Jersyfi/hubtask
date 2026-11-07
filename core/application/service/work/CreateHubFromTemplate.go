// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const CreateHubFromTemplateName = "CreateHubFromTemplate"

// HubTemplates takes a template into a hub made for it, inside the transaction it is called in,
// through the configuration's own plan and apply (configuration.md §6). Each refuses as the
// apply does.
type HubTemplates interface {
	// TakeWorkspacePartOnce applies the template's workspace part where the workspace has not
	// taken the template, and refuses a template without a hub part.
	TakeWorkspacePartOnce(ctx context.Context, actor appshared.ActorContext, take StarterTemplate) error
	TakeForHub(ctx context.Context, actor appshared.ActorContext, take StarterTemplate, hub shared.ID) error
}

// CreateHubFromTemplateCommand is the hub's name and look and the template it takes.
type CreateHubFromTemplateCommand struct {
	Name        string
	Description string
	Icon        string
	ColorToken  string
	Template    StarterTemplate
}

// CreateHubFromTemplate makes a hub from a catalogue template (user-experience.md §5.2): the
// template's workspace part where the workspace lacks it, the hub, and its hub part are one
// transaction, so a refused part leaves no hub behind - never a half-made one to clean up.
type CreateHubFromTemplate struct {
	Containers CreateContainer
	Templates  HubTemplates
}

// firstCollectionOf is the one collection a hub made from a template starts with, named as the
// hub - the person's word, not the server's (rule 8). Without it the hub's sample entries had no
// collection to land in and refused the whole, and its people a place to add the first entry.
func firstCollectionOf(hub domain.Container) CreateContainerCommand {
	return CreateContainerCommand{Type: domain.ContainerCollection, ParentID: hub.ID, Name: hub.Name}
}

// Execute makes the hub and answers it with its structure.
func (h CreateHubFromTemplate) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateHubFromTemplateCommand,
) (domain.Container, *domain.EffectiveStructure, error) {
	if cmd.Template.Key == "" || h.Templates == nil {
		return domain.Container{}, nil, invalidStarterField("/template", "usecase.field_required")
	}
	create := CreateContainerCommand{
		Type: domain.ContainerHub, Name: cmd.Name, Description: cmd.Description,
		Icon: cmd.Icon, ColorToken: cmd.ColorToken,
	}
	if err := h.Containers.authorize(ctx, actor, create); err != nil {
		return domain.Container{}, nil, err
	}
	var hub domain.Container
	var structure *domain.EffectiveStructure
	err := h.Containers.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// The workspace part first, as the starter takes it: the types and workflows a hub's
		// scheme names are the workspace's, and its default preset shapes the new hub.
		if err := h.Templates.TakeWorkspacePartOnce(ctx, actor, cmd.Template); err != nil {
			return err
		}
		var err error
		if hub, structure, err = h.Containers.create(ctx, actor, create); err != nil {
			return err
		}
		if _, _, err := h.Containers.create(ctx, actor, firstCollectionOf(hub)); err != nil {
			return err
		}
		return h.Templates.TakeForHub(ctx, actor, cmd.Template, hub.ID)
	})
	if err != nil {
		return domain.Container{}, nil, err
	}
	return hub, structure, nil
}

// Descriptor is the catalogue entry.
func (h CreateHubFromTemplate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionStructure,
		Name:       CreateHubFromTemplateName,
		Summary: "Creates a hub from a template of the catalogue: the template's workspace part where " +
			"the workspace has not taken it, the hub with a first collection named as the hub, and its " +
			"hub part, as ApplyConfig would apply them, all in one transaction - a refused part leaves " +
			"no hub. A template without a hub " +
			"part is refused. Needs the right to change the workspace's shape.",
		SideEffects: "Writes the hub as CreateContainer does - its event, change and audit entry - " +
			"and the configuration versions and audit entries ApplyConfig writes for each part taken.",
		TokenScope: workspaceManageScope,
		Input: []usecase.Field{
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 200 characters, on one line, unique among the hubs."},
			{Name: "description", Kind: usecase.KindString},
			{Name: "icon", Kind: usecase.KindString},
			{Name: "color_token", Kind: usecase.KindString, Description: "One of the ten label colours."},
			{Name: "template", Kind: usecase.KindString, Required: true, Description: "A template with a hub part, by key."},
			{Name: "answers", Kind: usecase.KindObject, Description: "The template's questions answered, by key."},
			{Name: "samples", Kind: usecase.KindBool, Description: "True takes the template's sample entries into the hub."},
			{
				Name: "acknowledge", Kind: usecase.KindBool,
				Description: "True once the person has seen that the template's plan hides data or changes permissions.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerCreatedAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a container is not an item, and the history is an item's: `ActivityEntry` is " +
				"keyed on `itemId`. The hub and the parts it took are in the audit trail.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateHubFromTemplate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	answers, err := starterAnswersFrom(in["answers"])
	if err != nil {
		return nil, err
	}
	hub, structure, err := h.Execute(ctx, actor, CreateHubFromTemplateCommand{
		Name: in.String("name"), Description: in.String("description"),
		Icon: in.String("icon"), ColorToken: in.String("color_token"),
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
