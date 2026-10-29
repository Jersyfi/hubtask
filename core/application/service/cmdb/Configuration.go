// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	ListCiClassesName        = "ListCiClasses"
	DefineCiClassName        = "DefineCiClass"
	ListCiRelationTypesName  = "ListCiRelationTypes"
	DefineCiRelationTypeName = "DefineCiRelationType"
	ListCiSourcesName        = "ListCiSources"
	DefineCiSourceName       = "DefineCiSource"
)

func list(in usecase.Input, field string) []map[string]any {
	raw, _ := in[field].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func text(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

func texts(raw any) []string {
	items, _ := raw.([]any)
	out := make([]string, 0, len(items))
	for _, i := range items {
		if s, ok := i.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func textMap(raw any) map[string]string {
	m, _ := raw.(map[string]any)
	if m == nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// ClassOutput is a class as every door returns it (schema CiClass); resolved adds what it
// inherits.
func ClassOutput(c domain.Class, resolved domain.Resolved) usecase.Output {
	attrs := func(list []domain.Attribute) []usecase.Output {
		out := make([]usecase.Output, 0, len(list))
		for _, a := range list {
			out = append(out, usecase.Output{"key": a.Key, "kind": string(a.Kind), "need": string(a.Need),
				"identifier": a.Identifier, "options": append([]string{}, a.Options...)})
		}
		return out
	}
	rules := make([]usecase.Output, 0, len(resolved.Rules))
	for _, r := range resolved.Rules {
		rules = append(rules, usecase.Output{"attributes": append([]string{}, r.Attributes...), "class": r.Class})
	}
	required := make([]usecase.Output, 0, len(resolved.Required))
	for _, r := range resolved.Required {
		required = append(required, usecase.Output{"types": append([]string{}, r.Types...), "outgoing": r.Outgoing})
	}
	return usecase.Output{
		"key": c.Key, "parent": c.Parent, "name": c.Name, "icon": c.Icon, "abstract": c.Abstract, "system": c.System,
		"attributes": attrs(c.Attributes), "all_attributes": attrs(resolved.Attributes), "identifier_rules": rules,
		"required_relations": required, "staleness_days": resolved.StalenessDays, "lineage": append([]string{}, resolved.Lineage...),
		"archived": c.ArchivedAt != nil, "version": c.Version,
	}
}

// ListCiClasses answers the class registry: the shipped hierarchy and the workspace's own.
type ListCiClasses struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiClasses) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiClassesName, Permission: service.PermissionRead,
		Summary: "Lists the configuration item classes: the hierarchy Hubtask ships (hardware, virtual and " +
			"cloud resources, software, business and technical services with their offerings, application " +
			"services, locations, contracts, processes, information objects) and the workspace's own, each " +
			"with what it inherits - attributes, identifier rules nearest first, the relations it needs.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: ConfigReadAction, TargetType: classTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Inventory.authorizeConfig(ctx, actor, false, ConfigReadAction, classTarget); err != nil {
				return nil, err
			}
			var reg registry
			err := h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				reg, err = h.Inventory.registry(ctx)
				return err
			})
			if err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			for _, c := range reg.classes.All() {
				rows = append(rows, ClassOutput(c, reg.classes.Resolve(c.Key)))
			}
			return usecase.Output{"data": rows}, nil
		}),
	})
}

// DefineCiClass adds a class of the workspace's own beneath another, or changes one.
type DefineCiClass struct{ Inventory Inventory }

func attributesOf(in usecase.Input) []domain.Attribute {
	var out []domain.Attribute
	for _, m := range list(in, "attributes") {
		need := domain.Need(text(m, "need"))
		if need == "" {
			need = domain.NeedOptional
		}
		identifier, _ := m["identifier"].(bool)
		out = append(out, domain.Attribute{Key: text(m, "key"), Kind: domain.AttributeKind(text(m, "kind")), Need: need,
			Identifier: identifier, Options: texts(m["options"])})
	}
	return out
}

// Descriptor is the catalogue entry.
func (h DefineCiClass) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineCiClassName, Permission: service.PermissionStructure,
		Summary: "Adds a configuration item class of the workspace's own beneath a shipped or an own class, or " +
			"changes an own class: its name, icon, whether it is abstract, its typed attributes (text, number, " +
			"boolean, date, choice, url, group, account; optional, recommended or required; identifying or " +
			"not), its identifier rules tried in order, the relations a CI of it is an orphan without and its " +
			"staleness period. A shipped class is not changed. Archiving stops new CIs of the class.",
		SideEffects: "Writes the class and an audit entry.", TokenScope: configWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores, starting with a letter."},
			{Name: "parent", Kind: usecase.KindString, Required: true, Description: "The class it inherits from."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
			{Name: "icon", Kind: usecase.KindString, Description: "The name of an icon of the design system's set."},
			{Name: "abstract", Kind: usecase.KindBool, Description: "Holds no CI of its own."},
			{Name: "attributes", Kind: usecase.KindList, Description: "The attributes it adds: key, kind, need, identifier, options."},
			{Name: "identifier_rules", Kind: usecase.KindList, Description: "Ordered sets of attribute keys that together identify a CI; `name` is the CI's name."},
			{Name: "required_relations", Kind: usecase.KindList, Description: "Relations a CI is an orphan without: types, outgoing."},
			{Name: "staleness_days", Kind: usecase.KindInt, Description: "Days unconfirmed before a CI is stale; 0 inherits."},
			{Name: "archived", Kind: usecase.KindBool, Description: "Archive the class: readable, offered for no new CI."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for a change."},
		},
		Audit: usecase.AuditDeclaration{Action: ClassDefinedAction, TargetType: classTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Inventory.authorizeConfig(ctx, actor, true, ClassDefinedAction, classTarget); err != nil {
				return nil, err
			}
			var rules []domain.IdentifierRule
			for _, m := range list(in, "identifier_rules") {
				rules = append(rules, domain.IdentifierRule{Attributes: texts(m["attributes"])})
			}
			var required []domain.RequiredRelation
			for _, m := range list(in, "required_relations") {
				outgoing, _ := m["outgoing"].(bool)
				required = append(required, domain.RequiredRelation{Types: texts(m["types"]), Outgoing: outgoing})
			}
			now := h.Inventory.Clock.Now()
			input := domain.ClassInput{
				ID: h.Inventory.IDs.NewID(), TenantID: actor.TenantID, Key: in.String("key"), Parent: in.String("parent"),
				Name: in.String("name"), Icon: in.String("icon"), Abstract: in.Bool("abstract"), Attributes: attributesOf(in),
				Rules: rules, Required: required, StalenessDays: in.Int("staleness_days"), Now: now,
			}
			var out usecase.Output
			err := h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				reg, err := h.Inventory.registry(ctx)
				if err != nil {
					return err
				}
				existing, found := reg.classes.Find(input.Key)
				var class domain.Class
				expected := 0
				switch {
				case found && existing.System:
					return shared.ErrConflict.WithDetail("cmdb.class_key_taken").WithParams(map[string]string{"key": input.Key})
				case found:
					if in.Present("expected_version") && in.Int("expected_version") != existing.Version {
						return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
					}
					expected = existing.Version
					class, err = existing.Changed(input, reg.classes)
				default:
					if len(reg.ownClass) >= domain.MaxClasses {
						return shared.ErrConflict.WithDetail("cmdb.classes_full")
					}
					class, err = domain.NewClass(input, reg.classes)
				}
				if err != nil {
					return err
				}
				class.ArchivedAt = nil
				if in.Bool("archived") {
					class.ArchivedAt = &now
				}
				if err := h.Inventory.Store.PutClass(ctx, class, expected); err != nil {
					return err
				}
				if err := h.Inventory.announceCatalogue(ctx, actor); err != nil {
					return err
				}
				own := append([]domain.Class{class}, reg.ownClass...)
				classes := domain.NewClasses(own)
				out = ClassOutput(class, classes.Resolve(class.Key))
				return h.Inventory.record(ctx, actor, ClassDefinedAction, classTarget, actor.TenantID,
					audit.Change{Field: "key", Classification: audit.Open, To: class.Key},
					audit.Change{Field: "name", Classification: audit.Sensitive, To: class.Name})
			})
			return out, err
		}),
	})
}

// RelationTypeOutput is a relation type as every door returns it (schema CiRelationType).
func RelationTypeOutput(t domain.RelationType) usecase.Output {
	pairs := make([]usecase.Output, 0, len(t.Pairs))
	for _, p := range t.Pairs {
		pairs = append(pairs, usecase.Output{"from": p.From, "to": p.To})
	}
	return usecase.Output{
		"key": t.Key, "name": t.Name, "reverse_name": t.ReverseName, "impact": string(t.Impact), "pairs": pairs,
		"system": t.System, "archived": t.ArchivedAt != nil, "version": t.Version,
	}
}

// ListCiRelationTypes answers the relation catalogue.
type ListCiRelationTypes struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiRelationTypes) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiRelationTypesName, Permission: service.PermissionRead,
		Summary: "Lists the relation types between configuration items - the shipped catalogue (depends on, " +
			"runs on, hosted on, installed on, contains, connects to, member of, offering of) and the " +
			"workspace's own - each with which end suffers when the other fails and the classes it may join.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: ConfigReadAction, TargetType: relTypeTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Inventory.authorizeConfig(ctx, actor, false, ConfigReadAction, relTypeTarget); err != nil {
				return nil, err
			}
			var reg registry
			err := h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				reg, err = h.Inventory.registry(ctx)
				return err
			})
			if err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			for _, t := range reg.types.All() {
				rows = append(rows, RelationTypeOutput(t))
			}
			return usecase.Output{"data": rows}, nil
		}),
	})
}

// DefineCiRelationType adds or changes a relation type of the workspace's own.
type DefineCiRelationType struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h DefineCiRelationType) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineCiRelationTypeName, Permission: service.PermissionStructure,
		Summary: "Adds a relation type of the workspace's own between configuration items, or changes one: its " +
			"words both ways, which end suffers when the other fails (impact NONE, SOURCE, TARGET or BOTH - " +
			"what impact analysis walks and protection need is inherited against) and the class pairs it may " +
			"join. A shipped type is not changed.",
		SideEffects: "Writes the type and an audit entry.", TokenScope: configWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores, starting with a letter."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "How it reads from its source."},
			{Name: "reverse_name", Kind: usecase.KindString, Required: true, Description: "How it reads from its target."},
			{Name: "impact", Kind: usecase.KindString, Required: true, Enum: []string{"NONE", "SOURCE", "TARGET", "BOTH"}, Description: "Which end suffers when the other fails."},
			{Name: "pairs", Kind: usecase.KindList, Description: "The class pairs (from, to) it may join; empty joins any."},
			{Name: "archived", Kind: usecase.KindBool, Description: "Archive the type: kept on relations, offered for no new one."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for a change."},
		},
		Audit: usecase.AuditDeclaration{Action: RelationTypeDefinedAction, TargetType: relTypeTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Inventory.authorizeConfig(ctx, actor, true, RelationTypeDefinedAction, relTypeTarget); err != nil {
				return nil, err
			}
			var pairs []domain.ClassPair
			for _, m := range list(in, "pairs") {
				pairs = append(pairs, domain.ClassPair{From: text(m, "from"), To: text(m, "to")})
			}
			now := h.Inventory.Clock.Now()
			input := domain.RelationTypeInput{
				ID: h.Inventory.IDs.NewID(), TenantID: actor.TenantID, Key: in.String("key"), Name: in.String("name"),
				ReverseName: in.String("reverse_name"), Impact: domain.Impact(in.String("impact")), Pairs: pairs, Now: now,
			}
			var out usecase.Output
			err := h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				reg, err := h.Inventory.registry(ctx)
				if err != nil {
					return err
				}
				existing, found := reg.types.Find(input.Key)
				var t domain.RelationType
				expected := 0
				switch {
				case found && existing.System:
					return shared.ErrConflict.WithDetail("cmdb.relation_type_key_taken").WithParams(map[string]string{"key": input.Key})
				case found:
					if in.Present("expected_version") && in.Int("expected_version") != existing.Version {
						return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
					}
					expected = existing.Version
					t, err = existing.Changed(input, reg.classes)
				default:
					t, err = domain.NewRelationType(input, reg.types, reg.classes)
				}
				if err != nil {
					return err
				}
				t.ArchivedAt = nil
				if in.Bool("archived") {
					t.ArchivedAt = &now
				}
				if err := h.Inventory.Store.PutRelationType(ctx, t, expected); err != nil {
					return err
				}
				if err := h.Inventory.announceCatalogue(ctx, actor); err != nil {
					return err
				}
				out = RelationTypeOutput(t)
				return h.Inventory.record(ctx, actor, RelationTypeDefinedAction, relTypeTarget, actor.TenantID,
					audit.Change{Field: "key", Classification: audit.Open, To: t.Key},
					audit.Change{Field: "impact", Classification: audit.Open, To: string(t.Impact)})
			})
			return out, err
		}),
	})
}

func mappingOutput(m *domain.FileMapping) any {
	if m == nil {
		return nil
	}
	return usecase.Output{
		"class": m.Class, "class_column": m.ClassColumn, "name_column": m.NameColumn,
		"native_key_column": m.NativeKeyColumn, "columns": m.Columns, "defaults": m.Defaults,
	}
}

func mappingOf(raw any) *domain.FileMapping {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return &domain.FileMapping{
		Class: text(m, "class"), ClassColumn: text(m, "class_column"), NameColumn: text(m, "name_column"),
		NativeKeyColumn: text(m, "native_key_column"), Columns: textMap(m["columns"]), Defaults: textMap(m["defaults"]),
	}
}

// SourceOutput is a source as every door returns it (schema CiSource).
func SourceOutput(s domain.Source) usecase.Output {
	var last any
	if s.LastRunAt != nil {
		last = *s.LastRunAt
	}
	return usecase.Output{
		"key": s.Key, "kind": string(s.Kind), "rank": s.Rank, "classes": append([]string{}, s.Classes...),
		"may_create": s.MayCreate, "mapping": mappingOutput(s.Mapping), "system": s.System, "last_run_at": last,
		"version": s.Version,
	}
}

// ListCiSources answers the sources CIs come from, by precedence.
type ListCiSources struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiSources) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiSourcesName, Permission: service.PermissionRead,
		Summary: "Lists the sources configuration items come from - people (`manual`), files, programs and " +
			"connectors - by precedence, a lower rank first, with the classes each may write, whether it may " +
			"make new CIs, a file source's mapping and when it last ran.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: ConfigReadAction, TargetType: sourceTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Inventory.authorizeConfig(ctx, actor, false, ConfigReadAction, sourceTarget); err != nil {
				return nil, err
			}
			var sources []domain.Source
			err := h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				sources, err = h.Inventory.Store.Sources(ctx)
				return err
			})
			if err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			for _, s := range append(domain.SystemSources(), sources...) {
				rows = append(rows, SourceOutput(s))
			}
			return usecase.Output{"data": rows}, nil
		}),
	})
}

// DefineCiSource adds or changes a source.
type DefineCiSource struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h DefineCiSource) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineCiSourceName, Permission: service.PermissionStructure,
		Summary: "Adds or changes a source configuration items come from: a file, a program or a connector, its " +
			"rank (a lower number wins an attribute a higher one last wrote, until the winner has gone silent " +
			"for the class's staleness period), the classes it may write, whether it may make new CIs, and a " +
			"file source's mapping (class, name column, the column with the source's own key, columns to " +
			"attributes, defaults).",
		SideEffects: "Writes the source and an audit entry.", TokenScope: configWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores; not `manual`."},
			{Name: "kind", Kind: usecase.KindString, Required: true, Enum: []string{"FILE", "API", "CONNECTOR"}, Description: "How it reaches the reconciliation."},
			{Name: "rank", Kind: usecase.KindInt, Required: true, Description: "1 to 1000; a lower rank wins. People write at 10."},
			{Name: "classes", Kind: usecase.KindList, Description: "The class keys it may write, with their descendants; empty is every class."},
			{Name: "may_create", Kind: usecase.KindBool, Description: "It may make new CIs; otherwise it only updates those it identifies."},
			{Name: "mapping", Kind: usecase.KindObject, Description: "How a CSV or JSON file of the source is read."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for a change."},
		},
		Audit: usecase.AuditDeclaration{Action: SourceDefinedAction, TargetType: sourceTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Inventory.authorizeConfig(ctx, actor, true, SourceDefinedAction, sourceTarget); err != nil {
				return nil, err
			}
			input := domain.SourceInput{
				ID: h.Inventory.IDs.NewID(), TenantID: actor.TenantID, Key: in.String("key"), Kind: domain.SourceKind(in.String("kind")),
				Rank: in.Int("rank"), Classes: texts(in["classes"]), MayCreate: in.Bool("may_create"), Mapping: mappingOf(in["mapping"]),
				Now: h.Inventory.Clock.Now(),
			}
			var out usecase.Output
			err := h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				reg, err := h.Inventory.registry(ctx)
				if err != nil {
					return err
				}
				var existing *domain.Source
				expected := 0
				if s, ok := reg.sources[input.Key]; ok && !s.System {
					if in.Present("expected_version") && in.Int("expected_version") != s.Version {
						return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
					}
					existing, expected = &s, s.Version
				}
				source, err := domain.NewSource(input, existing, reg.classes)
				if err != nil {
					return err
				}
				if err := h.Inventory.Store.PutSource(ctx, source, expected); err != nil {
					return err
				}
				out = SourceOutput(source)
				return h.Inventory.record(ctx, actor, SourceDefinedAction, sourceTarget, actor.TenantID,
					audit.Change{Field: "key", Classification: audit.Open, To: source.Key},
					audit.Change{Field: "rank", Classification: audit.Open, To: source.Rank})
			})
			return out, err
		}),
	})
}

// CatalogueOutput is the workspace's classes and relation types as ListCiClasses and
// ListCiRelationTypes answer them, in one record for a device's copy.
func CatalogueOutput(ownClasses []domain.Class, ownTypes []domain.RelationType) usecase.Output {
	classes := domain.NewClasses(ownClasses)
	classRows := []usecase.Output{}
	for _, c := range classes.All() {
		classRows = append(classRows, ClassOutput(c, classes.Resolve(c.Key)))
	}
	typeRows := []usecase.Output{}
	for _, t := range domain.NewRelationTypes(ownTypes).All() {
		typeRows = append(typeRows, RelationTypeOutput(t))
	}
	return usecase.Output{"classes": classRows, "relation_types": typeRows}
}
