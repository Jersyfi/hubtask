// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListConfigTemplatesName   = "ListConfigTemplates"
	PublishConfigTemplateName = "PublishConfigTemplate"
	PlanConfigTemplateName    = "PlanConfigTemplate"
)

// taken is the template an apply records as the workspace's provenance - or a hub's, for the
// template's hub part - if it names one.
type taken struct {
	label    string
	template *domain.Template
	hub      shared.ID
}

// provenance reads `key@version`, refusing a template the workspace does not have, and for a hub
// one without a hub part.
func (d Desk) provenance(ctx context.Context, actor appshared.ActorContext, label string, hub shared.ID) (taken, error) {
	if label == "" {
		return taken{}, nil
	}
	key, raw, _ := strings.Cut(label, "@")
	version, err := strconv.Atoi(raw)
	if err != nil || version < 1 {
		return taken{}, domain.ErrTemplateUnknown(label)
	}
	template, found, err := d.presetTemplateOf(ctx, actor, key, version)
	if err != nil {
		return taken{}, err
	}
	if !found {
		if template, err = d.Store.FindTemplate(ctx, key, version); err != nil {
			return taken{}, err
		}
	}
	if !hub.IsZero() && !template.ForHubs() {
		return taken{}, refused("config.template_not_for_hubs", "/template", map[string]string{"template": label})
	}
	return taken{label: template.Provenance(), template: &template, hub: hub}, nil
}

// record keeps the template version taken, with its document then: the base of the next upgrade.
func (t taken) record(ctx context.Context, d Desk) error {
	if t.template == nil {
		return nil
	}
	document := t.template.Document
	if !t.hub.IsZero() {
		document = t.template.Hub
	}
	return d.Store.PutTemplateUse(ctx, domain.TemplateUse{
		HubID: t.hub, TemplateKey: t.template.Key, Version: t.template.Version, Document: document, At: d.Clock.Now(),
	})
}

// TemplateOutput is a template version as every door answers it.
func TemplateOutput(t domain.Template, taken *domain.TemplateUse) usecase.Output {
	out := usecase.Output{
		"key": t.Key, "version": t.Version, "name": t.Name, "origin": string(t.Origin),
		"provenance": t.Provenance(), "document": documentMap(t.Document), "created_at": t.CreatedAt,
		"taken_version": nil, "update_available": false, "hub_document": nil,
	}
	if t.ForHubs() {
		out["hub_document"] = documentMap(t.Hub)
	}
	questions := make([]any, 0, len(t.Questions))
	for _, q := range t.Questions {
		options := make([]any, 0, len(q.Options))
		for _, option := range q.Options {
			options = append(options, option)
		}
		var preset any
		if q.Default != "" {
			preset = q.Default
		}
		questions = append(questions, map[string]any{
			"key": q.Key, "kind": string(q.Kind), "required": q.Required, "default": preset, "options": options,
		})
	}
	out["questions"] = questions
	if taken != nil {
		out["taken_version"] = taken.Version
		out["update_available"] = taken.Version < t.Version
	}
	return out
}

// ListConfigTemplates answers the workspace's templates: the newest version of each, the version
// the workspace took and whether an update is available.
type ListConfigTemplates struct{ Desk Desk }

func (h ListConfigTemplates) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListConfigTemplatesName,
		Permission: service.PermissionStructure,
		Summary: "The templates the reader may take - bundles of building blocks, each version kept - with the " +
			"newest version of each, the version the workspace took and whether an update is available. " +
			"Every preset is among them as a template of its structure part: Hubtask's by its key (list, " +
			"tasks, project, flow), the installation's, the workspace's and the reader's own by identifier - and " +
			"Hubtask's own document templates (itil_service_desk). With hub_id, the templates a hub can take - " +
			"those with a hub part - with the version that hub took.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input:       []usecase.Field{hubField()},
		Audit:       usecase.AuditDeclaration{Action: ReadAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := hubOf(in)
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				templates, err := h.Desk.Store.ListTemplates(ctx)
				if err != nil {
					return err
				}
				uses, err := h.Desk.Store.TemplateUses(ctx, hub)
				if err != nil {
					return err
				}
				byKey := map[string]*domain.TemplateUse{}
				for i := range uses {
					byKey[uses[i].TemplateKey] = &uses[i]
				}
				newest := map[string]domain.Template{}
				var order []string
				presets, err := h.Desk.presetTemplates(ctx, actor)
				if err != nil {
					return err
				}
				for _, t := range append(presets, templates...) {
					held, seen := newest[t.Key]
					if !seen {
						order = append(order, t.Key)
					}
					if !seen || t.Version > held.Version {
						newest[t.Key] = t
					}
				}
				for _, key := range order {
					if !hub.IsZero() && !newest[key].ForHubs() {
						continue
					}
					rows = append(rows, TemplateOutput(newest[key], byKey[key]))
				}
				return nil
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// PublishConfigTemplate publishes the next version of a template.
type PublishConfigTemplate struct{ Desk Desk }

func (h PublishConfigTemplate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       PublishConfigTemplateName,
		Permission: service.PermissionStructure,
		Summary: "Publishes the next version of a workspace template: a configuration document in " +
			"MERGE form that taking it applies. It is checked first - every block must be one its " +
			"section's operations would accept - and a template that does not pass is not published. " +
			"A published version never changes; omitted, the document is the configuration in force " +
			"(save as template). A hub part - hub_document, or with hub_id that hub's own document in force - " +
			"makes it a hub template: a hub taking it applies that part to itself.",
		SideEffects: "Stores the template version and writes an audit entry.",
		TokenScope:  scopeApply,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The template's stable key."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
			{Name: "document", Kind: usecase.KindObject, Description: "The blocks the template sets up; omitted takes the configuration in force."},
			{Name: "hub_id", Kind: usecase.KindID, Description: "A hub whose own document in force is the template's hub part, unless hub_document names it."},
			{Name: "hub_document", Kind: usecase.KindObject, Description: "The hub part: a hub's document - modules, structure, scheme, business_calendars - that a hub taking the template applies to itself."},
		},
		Audit:    usecase.AuditDeclaration{Action: TemplateAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a template is configuration, not an item."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, TemplateAction, scopeApply, false); err != nil {
				return nil, err
			}
			return h.Desk.publish(ctx, actor, in)
		}),
	}
}

// templateDocument is what a template keeps of a workspace's document: neither its settings nor
// what only this workspace holds - its organisations, which name its own customers by their mail
// domains, and its own preset, named by an identifier no other workspace holds.
func templateDocument(document domain.Document) domain.Document {
	delete(document, sectionSettings)
	delete(document, sectionOrganisations)
	if preset := document[sectionStructure][structureDefaultPreset]; preset != nil && preset[presetIDAttribute] != nil {
		delete(document[sectionStructure], structureDefaultPreset)
	}
	return document
}

func (d Desk) publish(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	key, name := in.String("key"), strings.TrimSpace(in.String("name"))
	if !domain.ValidKey(key) {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/key", Code: "config.key_invalid"}})
	}
	if reservedKey(key) {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/key", Code: "config.template_key_shipped"}})
	}
	if name == "" || len([]rune(name)) > 120 || strings.ContainsAny(name, "\n\r") {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/name", Code: "config.name_invalid"}})
	}
	var out usecase.Output
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now, err := d.export(ctx, actor, "")
		if err != nil {
			return err
		}
		document := now.Document
		if in.Present("document") {
			var refusals []shared.FieldError
			if document, refusals = documentOf(in["document"]); len(refusals) > 0 {
				return domain.ErrRefused(refusals)
			}
		}
		document = templateDocument(document)
		// Linted as if taken by an empty workspace: every block must be one its create accepts.
		lint := d.planOf(current{Document: lintBase()}, withoutRemovals(document), domain.ModeMerge)
		if len(lint.Refusals) > 0 {
			return domain.ErrRefused(lint.Refusals)
		}
		hubPart, err := d.hubPart(ctx, actor, in)
		if err != nil {
			return err
		}
		templates, err := d.Store.ListTemplates(ctx)
		if err != nil {
			return err
		}
		version := 1
		for _, t := range templates {
			if t.Key == key && t.Version >= version {
				version = t.Version + 1
			}
		}
		at := d.Clock.Now()
		template := domain.Template{
			ID: d.IDs.NewID(), TenantID: actor.TenantID, Key: key, Version: version, Name: name,
			Origin: domain.OriginWorkspace, Document: document, Hub: hubPart, CreatedBy: actor.AccountID, CreatedAt: at,
		}
		if err := d.Store.InsertTemplate(ctx, template); err != nil {
			return err
		}
		out = TemplateOutput(template, nil)
		return d.trail(ctx, actor, TemplateAction, at,
			audit.Change{Field: "template", Classification: audit.Open, To: template.Provenance()})
	})
	return out, err
}

// lintHub stands for the hub a hub part is linted against: no operation of the lint runs.
const lintHub shared.ID = "00000000-0000-7000-8000-000000000000"

// hubPart reads a template's hub part: the document named, or the hub's own in force - linted as
// if an empty hub took it. Neither named, the template is the workspace's only.
func (d Desk) hubPart(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (domain.Document, error) {
	hub, err := hubOf(in)
	if err != nil {
		return nil, err
	}
	var part domain.Document
	switch {
	case in.Present("hub_document"):
		var refusals []shared.FieldError
		if part, refusals = documentOf(in["hub_document"]); len(refusals) > 0 {
			return nil, domain.ErrRefused(refusals)
		}
	case !hub.IsZero():
		now, err := d.export(ctx, actor, hub)
		if err != nil {
			return nil, err
		}
		part = now.Document
	default:
		return nil, nil
	}
	if part == nil {
		part = domain.Document{}
	}
	lint := d.planOf(current{Document: lintBase(), Hub: lintHub}, withoutRemovals(part), domain.ModeMerge)
	if len(lint.Refusals) > 0 {
		for i := range lint.Refusals {
			lint.Refusals[i].Path = "/hub_document/" + strings.TrimPrefix(lint.Refusals[i].Path, "/")
		}
		return nil, domain.ErrRefused(lint.Refusals)
	}
	return part, nil
}

// lintBase is an empty workspace: every module left to the level above, no block of its own.
func lintBase() domain.Document {
	modules := domain.Section{}
	for _, key := range module.Keys() {
		modules[strings.ToLower(string(key))] = domain.Block{switchAttribute: "INHERIT"}
	}
	return domain.Document{sectionModules: modules}
}

func withoutRemovals(d domain.Document) domain.Document {
	out := domain.Document{}
	for name, section := range d {
		kept := domain.Section{}
		for key, block := range section {
			if block != nil {
				kept[key] = block
			}
		}
		out[name] = kept
	}
	return out
}

// PlanConfigTemplate plans taking a template, or upgrading to a newer version by a three-way
// comparison (D-17).
type PlanConfigTemplate struct{ Desk Desk }

func (h PlanConfigTemplate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       PlanConfigTemplateName,
		Permission: service.PermissionStructure,
		Summary: "Plans taking a template version, or - for a template the workspace took - upgrading " +
			"to that version: a three-way comparison of what the template changed between the two " +
			"versions against what the workspace changed itself. Each change is TAKE (the template " +
			"changed it, the workspace did not) or CONFLICT (both did, differently); conflicts are " +
			"left unless named in take. Answers the changes, the document to apply and its plan; " +
			"ApplyConfig with template set applies it and records the provenance. With hub_id, the template's " +
			"hub part against that hub's own document, once the workspace took the template's own part.",
		SideEffects: "None.",
		TokenScope:  scopePlan,
		ReadOnly:    true,
		Input: []usecase.Field{
			hubField(),
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The template."},
			{Name: "version", Kind: usecase.KindInt, Description: "The version to take or upgrade to; omitted is the newest."},
			{Name: "take", Kind: usecase.KindList, Description: "Key paths of the upgrade's changes to take; omitted takes every TAKE and leaves every CONFLICT."},
			{Name: "answers", Kind: usecase.KindObject, Description: "The template's questions answered, by key: an answer takes the place of its mark, an open question its default; a block naming an open question with no default is left out."},
		},
		Audit:    usecase.AuditDeclaration{Action: PlannedAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a plan changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := hubOf(in)
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, PlannedAction, scopePlan, true); err != nil {
				return nil, err
			}
			take, err := in.StringList("take")
			if err != nil {
				return nil, err
			}
			answers, err := answersOf(in)
			if err != nil {
				return nil, err
			}
			return h.Desk.planTemplate(ctx, actor, hub, in.String("key"), in.Int("version"), take, answers)
		}),
	}
}

// answersOf reads the answers, each a string by its question's key.
func answersOf(in usecase.Input) (map[string]string, error) {
	raw, _ := in["answers"].(map[string]any)
	answers := make(map[string]string, len(raw))
	for key, value := range raw {
		text, ok := value.(string)
		if !ok {
			return nil, refused("config.answer_not_text", "answers."+key, nil)
		}
		answers[key] = text
	}
	return answers, nil
}

// answered is a template's document with its questions answered, a GROUP answer held to a group
// of the workspace the reader may see - a group nobody can find is refused, not escalated to.
func (d Desk) answered(ctx context.Context, actor appshared.ActorContext, t domain.Template, document domain.Document, answers map[string]string) (domain.Document, error) {
	if len(t.Questions) == 0 && len(answers) == 0 {
		return document, nil
	}
	answered, err := domain.Answer(t.Questions, document, answers)
	if err != nil {
		return nil, err
	}
	for _, q := range t.Questions {
		value, given := answers[q.Key]
		if q.Kind != domain.QuestionGroup || !given {
			continue
		}
		if _, err := d.preview(ctx, identity.GetGroupName, actor, usecase.Input{"group_id": strings.TrimSpace(value)}); err != nil {
			if errors.Is(err, shared.ErrNotFound) || withheld(err) {
				return nil, refused("config.answer_group_unknown", "answers."+q.Key, nil)
			}
			return nil, err
		}
	}
	return answered, nil
}

func (d Desk) planTemplate(ctx context.Context, actor appshared.ActorContext, hub shared.ID, key string, version int, take []string, answers map[string]string) (usecase.Output, error) {
	var out usecase.Output
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		templates, err := d.Store.ListTemplates(ctx)
		if err != nil {
			return err
		}
		presets, err := d.presetTemplates(ctx, actor)
		if err != nil {
			return err
		}
		templates = append(presets, templates...)
		var chosen *domain.Template
		for i, t := range templates {
			if t.Key == key && (version == 0 && (chosen == nil || t.Version > chosen.Version) || t.Version == version) {
				chosen = &templates[i]
			}
		}
		if chosen == nil {
			return domain.ErrTemplateUnknown(key)
		}
		uses, err := d.Store.TemplateUses(ctx, "")
		if err != nil {
			return err
		}
		document := chosen.Document
		if !hub.IsZero() {
			if document, uses, err = d.hubTemplate(ctx, *chosen, uses, hub); err != nil {
				return err
			}
		}
		if document, err = d.answered(ctx, actor, *chosen, document, answers); err != nil {
			return err
		}
		now, err := d.export(ctx, actor, hub)
		if err != nil {
			return err
		}
		upgrades := []usecase.Output{}
		from := 0
		for _, use := range uses {
			if use.TemplateKey != key {
				continue
			}
			from = use.Version
			// The base is answered as the new version is, so an answer given again is no change
			// and the template's own edits around it are TAKEs.
			base, err := domain.Answer(chosen.Questions, use.Document, answers)
			if err != nil {
				base = use.Document
			}
			changes := domain.ThreeWay(base, now.Document, document)
			for _, change := range changes {
				upgrades = append(upgrades, usecase.Output{
					"path": change.Path(), "state": string(change.State), "removed": change.Removed,
					"base": change.Base, "ours": change.Ours, "theirs": change.Theirs,
				})
			}
			document = domain.UpgradeDocument(changes, take)
		}
		p := d.planOf(now, document, domain.ModeMerge)
		out = PlanOutput(p)
		out["template"] = chosen.Provenance()
		out["hub_id"] = nil
		if !hub.IsZero() {
			out["hub_id"] = hub.String()
		}
		out["from_version"] = nil
		if from > 0 {
			out["from_version"] = from
		}
		out["upgrade"] = upgrades
		out["document"] = documentMap(document)
		out["mode"] = string(domain.ModeMerge)
		return nil
	})
	return out, err
}

// hubTemplate is what a hub takes of a template: its hub part, planned against what the hub took
// before. The template's workspace part comes first - the types and workflows a hub's scheme names
// are the workspace's - so a hub takes a template the workspace took.
func (d Desk) hubTemplate(ctx context.Context, chosen domain.Template, workspace []domain.TemplateUse, hub shared.ID) (domain.Document, []domain.TemplateUse, error) {
	if !chosen.ForHubs() {
		return nil, nil, refused("config.template_not_for_hubs", "/key", map[string]string{"template": chosen.Key})
	}
	if len(chosen.Document) > 0 && !slices.ContainsFunc(workspace, func(u domain.TemplateUse) bool { return u.TemplateKey == chosen.Key }) {
		return nil, nil, refused("config.template_workspace_first", "/key", map[string]string{"template": chosen.Key})
	}
	uses, err := d.Store.TemplateUses(ctx, hub)
	if err != nil {
		return nil, nil, err
	}
	return chosen.Hub, uses, nil
}
