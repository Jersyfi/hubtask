// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"strconv"
	"strings"

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

// taken is the template an apply records as the workspace's provenance, if it names one.
type taken struct {
	label    string
	template *domain.Template
}

// provenance reads `key@version`, refusing a template the workspace does not have.
func (d Desk) provenance(ctx context.Context, label string) (taken, error) {
	if label == "" {
		return taken{}, nil
	}
	key, raw, _ := strings.Cut(label, "@")
	version, err := strconv.Atoi(raw)
	if err != nil || version < 1 {
		return taken{}, domain.ErrTemplateUnknown(label)
	}
	template, found := shippedTemplate(key, version)
	if !found {
		if template, err = d.Store.FindTemplate(ctx, key, version); err != nil {
			return taken{}, err
		}
	}
	return taken{label: template.Provenance(), template: &template}, nil
}

// record keeps the template version taken, with its document then: the base of the next upgrade.
func (t taken) record(ctx context.Context, d Desk) error {
	if t.template == nil {
		return nil
	}
	return d.Store.PutTemplateUse(ctx, domain.TemplateUse{
		TemplateKey: t.template.Key, Version: t.template.Version, Document: t.template.Document, At: d.Clock.Now(),
	})
}

// TemplateOutput is a template version as every door answers it.
func TemplateOutput(t domain.Template, taken *domain.TemplateUse) usecase.Output {
	out := usecase.Output{
		"key": t.Key, "version": t.Version, "name": t.Name, "origin": string(t.Origin),
		"provenance": t.Provenance(), "document": documentMap(t.Document), "created_at": t.CreatedAt,
		"taken_version": nil, "update_available": false,
	}
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
		Summary: "The workspace's templates - bundles of building blocks, each version kept - with the " +
			"newest version of each, the version the workspace took and whether an update is available.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: ReadAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				templates, err := h.Desk.Store.ListTemplates(ctx)
				if err != nil {
					return err
				}
				uses, err := h.Desk.Store.TemplateUses(ctx)
				if err != nil {
					return err
				}
				byKey := map[string]*domain.TemplateUse{}
				for i := range uses {
					byKey[uses[i].TemplateKey] = &uses[i]
				}
				newest := map[string]domain.Template{}
				var order []string
				for _, t := range append(shipped(), templates...) {
					held, seen := newest[t.Key]
					if !seen {
						order = append(order, t.Key)
					}
					if !seen || t.Version > held.Version {
						newest[t.Key] = t
					}
				}
				for _, key := range order {
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
			"(save as template).",
		SideEffects: "Stores the template version and writes an audit entry.",
		TokenScope:  scopeApply,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The template's stable key."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
			{Name: "document", Kind: usecase.KindObject, Description: "The blocks the template sets up; omitted takes the configuration in force."},
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

func (d Desk) publish(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	key, name := in.String("key"), strings.TrimSpace(in.String("name"))
	if !domain.ValidKey(key) {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/key", Code: "config.key_invalid"}})
	}
	if _, held := shippedTemplate(key, 0); held {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/key", Code: "config.template_key_shipped"}})
	}
	if name == "" || len([]rune(name)) > 120 || strings.ContainsAny(name, "\n\r") {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/name", Code: "config.name_invalid"}})
	}
	var out usecase.Output
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now, err := d.export(ctx, actor)
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
		delete(document, sectionSettings)
		// Linted as if taken by an empty workspace: every block must be one its create accepts.
		lint := d.planOf(current{Document: lintBase()}, withoutRemovals(document), domain.ModeMerge)
		if len(lint.Refusals) > 0 {
			return domain.ErrRefused(lint.Refusals)
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
			Origin: domain.OriginWorkspace, Document: document, CreatedBy: actor.AccountID, CreatedAt: at,
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
			"ApplyConfig with template set applies it and records the provenance.",
		SideEffects: "None.",
		TokenScope:  scopePlan,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The template."},
			{Name: "version", Kind: usecase.KindInt, Description: "The version to take or upgrade to; omitted is the newest."},
			{Name: "take", Kind: usecase.KindList, Description: "Key paths of the upgrade's changes to take; omitted takes every TAKE and leaves every CONFLICT."},
		},
		Audit:    usecase.AuditDeclaration{Action: PlannedAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a plan changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, PlannedAction, scopePlan, true); err != nil {
				return nil, err
			}
			take, err := in.StringList("take")
			if err != nil {
				return nil, err
			}
			return h.Desk.planTemplate(ctx, actor, in.String("key"), in.Int("version"), take)
		}),
	}
}

func (d Desk) planTemplate(ctx context.Context, actor appshared.ActorContext, key string, version int, take []string) (usecase.Output, error) {
	var out usecase.Output
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		templates, err := d.Store.ListTemplates(ctx)
		if err != nil {
			return err
		}
		templates = append(shipped(), templates...)
		var chosen *domain.Template
		for i, t := range templates {
			if t.Key == key && (version == 0 && (chosen == nil || t.Version > chosen.Version) || t.Version == version) {
				chosen = &templates[i]
			}
		}
		if chosen == nil {
			return domain.ErrTemplateUnknown(key)
		}
		uses, err := d.Store.TemplateUses(ctx)
		if err != nil {
			return err
		}
		now, err := d.export(ctx, actor)
		if err != nil {
			return err
		}
		document := chosen.Document
		upgrades := []usecase.Output{}
		from := 0
		for _, use := range uses {
			if use.TemplateKey != key {
				continue
			}
			from = use.Version
			changes := domain.ThreeWay(use.Document, now.Document, chosen.Document)
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
