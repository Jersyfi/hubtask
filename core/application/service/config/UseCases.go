// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"strings"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ReadConfigSchemaName     = "ReadConfigSchema"
	ExportConfigName         = "ExportConfig"
	ExplainConfigName        = "ExplainConfig"
	ValidateConfigName       = "ValidateConfig"
	PlanConfigName           = "PlanConfig"
	ProposeConfigName        = "ProposeConfig"
	ApplyConfigName          = "ApplyConfig"
	ListConfigVersionsName   = "ListConfigVersions"
	ReadConfigVersionName    = "ReadConfigVersion"
	ListConfigProposalsName  = "ListConfigProposals"
	DecideConfigProposalName = "DecideConfigProposal"
	SetConfigPolicyName      = "SetConfigPolicy"

	maxListed = 100
)

var modes = []string{string(domain.ModeMerge), string(domain.ModeReplace)}

func documentFields(required bool) []usecase.Field {
	return []usecase.Field{
		{Name: "document", Kind: usecase.KindObject, Required: required, Description: "The configuration document or the part of it to change: sections of blocks by stable key (schema at ReadConfigSchema). A null block removes it."},
		{Name: "mode", Kind: usecase.KindString, Enum: modes, Description: "MERGE (default) changes only what the document names; REPLACE states every section it names whole."},
		hubField(),
	}
}

// desired reads the document a call asks for: the one handed in, or, for a rollback, an earlier
// version stated whole.
func (d Desk) desired(ctx context.Context, in usecase.Input) (domain.Document, domain.Mode, int, error) {
	if version := in.Int("to_version"); version > 0 {
		revision, err := d.Store.Find(ctx, version)
		if err != nil {
			return nil, "", 0, err
		}
		// A version restores the document it was of: a hub's never becomes the workspace's.
		if hub, err := hubOf(in); err != nil || hub != revision.HubID {
			return nil, "", 0, domain.ErrRefused([]shared.FieldError{{Path: "/to_version", Code: "config.version_other_scope"}})
		}
		return revision.Document, domain.ModeReplace, version, nil
	}
	if in.Bool("from_sandbox") {
		if hub, _ := hubOf(in); !hub.IsZero() || in.Present("document") {
			return nil, "", 0, domain.ErrRefused([]shared.FieldError{{Path: "/from_sandbox", Code: "config.sandbox_alone"}})
		}
		promotion, err := d.sandboxPromotion(ctx)
		if err != nil {
			return nil, "", 0, err
		}
		return promotion, domain.ModeMerge, 0, nil
	}
	if !in.Present("document") {
		return nil, "", 0, domain.ErrRefused([]shared.FieldError{{Path: "/document", Code: "usecase.field_required"}})
	}
	document, refusals := documentOf(in["document"])
	if len(refusals) > 0 {
		return nil, "", 0, domain.ErrRefused(refusals)
	}
	mode := domain.Mode(strings.ToUpper(in.String("mode")))
	if mode == "" {
		mode = domain.ModeMerge
	}
	return document, mode, 0, nil
}

// planned reads what is in force and plans the call's document against it, in one read.
func (d Desk) planned(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (plan, current, domain.Mode, int, error) {
	var (
		p       plan
		now     current
		mode    domain.Mode
		version int
	)
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		document, m, v, err := d.desired(ctx, in)
		if err != nil {
			return err
		}
		mode, version = m, v
		hub, err := hubOf(in)
		if err != nil {
			return err
		}
		now, err = d.export(ctx, actor, hub)
		if err != nil {
			return err
		}
		p = d.planOf(now, document, mode)
		if len(p.Refusals) > 0 {
			return nil
		}
		return d.reachOf(ctx, actor, &p)
	})
	return p, now, mode, version, err
}

// PlanOutput is a plan as every door answers it, in the order a person reads it: what changes,
// what it reaches, what is refused, what must be acknowledged (configuration.md §2).
func PlanOutput(p plan) usecase.Output {
	changes := make([]usecase.Output, 0, len(p.Changes))
	for _, change := range p.Changes {
		row := usecase.Output{
			"path": change.Path(), "section": change.Section, "key": change.Key, "op": string(change.Op),
			"fields": nonNil(change.Fields), "before": blockOrNil(change.Before), "after": blockOrNil(change.After),
		}
		changes = append(changes, row)
	}
	ops := make([]usecase.Output, 0, len(p.Ops))
	for _, op := range p.Ops {
		ops = append(ops, usecase.Output{"use_case": op.UseCase, "path": op.Path})
	}
	counts := domain.CountsOf(p.Changes)
	out := usecase.Output{
		"base_hash": p.Base, "plan_hash": nil, "changes": changes, "operations": ops,
		"refusals": RefusalsOutput(p.Refusals),
		"flags":    usecase.Output{"hides_data": p.Flags.HidesData, "changes_permissions": p.Flags.ChangesPermissions},
		"step_up":  p.StepUp,
		"reach":    ReachOutput(p.Reach),
		"counts":   usecase.Output{"added": counts.Added, "changed": counts.Changed, "removed": counts.Removed},
	}
	if p.Hash != "" {
		out["plan_hash"] = p.Hash
	}
	return out
}

// RefusalsOutput is the machine-readable refusals an agent corrects itself by.
func RefusalsOutput(refusals []shared.FieldError) []usecase.Output {
	out := make([]usecase.Output, 0, len(refusals))
	for _, r := range refusals {
		row := usecase.Output{"path": r.Path, "code": r.Code}
		if len(r.Params) > 0 {
			row["params"] = r.Params
		}
		out = append(out, row)
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func blockOrNil(b domain.Block) any {
	if b == nil {
		return nil
	}
	return map[string]any(b)
}

// ReadConfigSchema answers the document's JSON Schema.
type ReadConfigSchema struct{ Desk Desk }

func (h ReadConfigSchema) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadConfigSchemaName,
		Summary: "The configuration document's JSON Schema: every section, the stable keys it names " +
			"blocks by, each attribute's type, allowed values and description, taken from the " +
			"operations that write it. An agent writes valid configuration by it.",
		SideEffects: "None.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing, and the schema is the product's."},
		Handler: usecase.HandlerFunc(func(_ context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if !actor.IsAuthenticated() {
				return nil, shared.ErrUnauthenticated
			}
			return usecase.Output{"schema": h.Desk.Schema()}, nil
		}),
	}
}

// ExportConfig answers the document in force.
type ExportConfig struct{ Desk Desk }

func (h ExportConfig) export(ctx context.Context, actor appshared.ActorContext, hub shared.ID) (current, int, error) {
	if err := h.Desk.authorizeAt(ctx, actor, hub, ReadAction, scopeRead, true); err != nil {
		return current{}, 0, err
	}
	var (
		now     current
		version int
	)
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if now, err = h.Desk.export(ctx, actor, hub); err != nil {
			return err
		}
		latest, _, err := h.Desk.Store.Latest(ctx)
		version = latest.Version
		return err
	})
	return now, version, err
}

func (h ExportConfig) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ExportConfigName,
		Permission: service.PermissionStructure,
		Summary: "The workspace's configuration document as it is in force: modules, roles, fields, " +
			"link types, entry types with their forms, workflows and the configuration settings, " +
			"each block by its stable key. Its hash is what a plan is made against; a section the " +
			"reader may not read, or whose module is off, is listed as withheld. Given a hub, the " +
			"hub's own document: its modules, structure settings and workflow scheme.",
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
			now, version, err := h.export(ctx, actor, hub)
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"document": map[string]any(documentMap(now.Document)), "hash": now.hash(), "version": version,
				"withheld": nonNil(now.Withheld), "hub_id": idOrNil(hub),
			}, nil
		}),
	}
}

func documentMap(d domain.Document) map[string]any {
	out := make(map[string]any, len(d))
	for name, section := range d {
		blocks := make(map[string]any, len(section))
		for key, block := range section {
			blocks[key] = blockOrNil(block)
		}
		out[name] = blocks
	}
	return out
}

// ExplainConfig answers one key: its value, where it comes from, who may change it.
type ExplainConfig struct{ Desk Desk }

func (h ExplainConfig) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ExplainConfigName,
		Permission: service.PermissionStructure,
		Summary: "Explains one key of the configuration document - `types.incident`, " +
			"`modules.workflows`, `settings.config.agent_apply`: its value, the level it comes from " +
			"and which level locked it, the permission a person needs to change it and the " +
			"operation that does, and where it applies.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The key path: a section and a block key, dotted."},
			hubField(),
		},
		Audit:    usecase.AuditDeclaration{Action: ReadAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := hubOf(in)
			if err != nil {
				return nil, err
			}
			return h.Desk.explain(ctx, actor, in.String("key"), hub)
		}),
	}
}

func (d Desk) explain(ctx context.Context, actor appshared.ActorContext, path string, hub shared.ID) (usecase.Output, error) {
	if err := d.authorizeAt(ctx, actor, hub, ReadAction, scopeRead, true); err != nil {
		return nil, err
	}
	name, key, _ := strings.Cut(path, ".")
	unknown := domain.ErrRefused([]shared.FieldError{domain.Refusal("config.key_unknown", path, nil)})
	var out usecase.Output
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now, err := d.export(ctx, actor, hub)
		if err != nil {
			return err
		}
		block, held := now.Document[name][key]
		row, read := now.Rows[sectionSettings][key]
		if !held && (name != sectionSettings || !read) {
			return unknown
		}
		out = usecase.Output{
			"key": path, "value": map[string]any(block), "source": string(setting.LevelWorkspace),
			"locked_by": nil, "permission": string(service.PermissionStructure), "applies_to": "WORKSPACE",
		}
		if !hub.IsZero() {
			out["source"], out["applies_to"] = sourceHub, sourceHub
		}
		if name == sectionSettings && read {
			// A catalogue setting is explained even where the workspace left it to the level
			// above: its value in force, where that comes from, and who may change it here.
			out["value"] = map[string]any{settingValueField: row[settingValueField]}
			out["source"] = text(row["source"])
			if locked := text(row["locked_by"]); locked != "" {
				out["locked_by"] = locked
			}
			if definition, found := definitionOf(key); found {
				out["permission"] = definition.Writers[setting.LevelWorkspace]
				if len(definition.Options) > 0 {
					out["allowed"] = definition.Options
				}
			}
			if area, found := areaOf(key); found {
				if op, err := area.write(domain.BlockChange{Section: name, Key: key, Op: domain.OpChange, After: domain.Block{}}, row); err == nil {
					out["operation"] = op.UseCase
				}
			}
			return nil
		}
		if name == sectionSettings {
			p, err := d.agentApply(ctx, actor.TenantID)
			if err != nil {
				return err
			}
			out["source"] = string(p.source)
			out["permission"] = domain.AgentApplyDefinition().Writers[setting.LevelWorkspace]
			out["allowed"] = domain.AgentApplyDefinition().Options
			if p.locked {
				out["locked_by"] = string(p.source)
			}
			out["operation"] = SetConfigPolicyName
			return nil
		}
		for _, s := range now.sections() {
			if s.name != name {
				continue
			}
			if s.name == sectionModules {
				out["operation"] = "SetModule"
				row := now.Rows[name][key]
				out["source"] = text(row["source"])
				if locked := row["locked_by"]; locked != nil {
					out["locked_by"] = locked
				}
			} else {
				out["operation"] = s.change
				if s.ops != nil {
					out["operation"] = "SaveWorkflowDraft"
				}
			}
			if descriptor, found := d.Catalogue.Lookup(text(out["operation"])); found && descriptor.Permission != "" {
				out["permission"] = string(descriptor.Permission)
			}
		}
		return nil
	})
	return out, err
}

// ValidateConfig answers a document's refusals only.
type ValidateConfig struct{ Desk Desk }

func (h ValidateConfig) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ValidateConfigName,
		Permission: service.PermissionStructure,
		Summary: "Checks a configuration document against the schema and the configuration in force " +
			"and answers what it refuses - a message code, the key path and its parameters - and " +
			"nothing else. Nothing is computed or written.",
		SideEffects: "None.",
		TokenScope:  scopePlan,
		ReadOnly:    true,
		Input:       documentFields(true),
		Audit:       usecase.AuditDeclaration{Action: PlannedAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a check changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := hubOf(in)
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, PlannedAction, scopePlan, true); err != nil {
				return nil, err
			}
			p, _, _, _, err := h.Desk.planned(ctx, actor, in)
			if err != nil {
				if refusals := refusalsIn(err); refusals != nil {
					return usecase.Output{"valid": false, "refusals": RefusalsOutput(refusals)}, nil
				}
				return nil, err
			}
			return usecase.Output{"valid": len(p.Refusals) == 0, "refusals": RefusalsOutput(p.Refusals)}, nil
		}),
	}
}

func refusalsIn(err error) []shared.FieldError {
	var e *shared.Error
	if ok := asError(err, &e); ok && e.DetailCode == "config.refused" {
		return e.Fields
	}
	return nil
}

// PlanConfig answers what a document would change, writing nothing.
type PlanConfig struct{ Desk Desk }

func planFields() []usecase.Field {
	return append(documentFields(false),
		usecase.Field{Name: "to_version", Kind: usecase.KindInt, Description: "Plans a rollback: the configuration version to go back to, stated whole. Replaces document."},
		usecase.Field{Name: "from_sandbox", Kind: usecase.KindBool, Description: "Promotes the sandbox: plans what it changed since its copy against the configuration in force. Replaces document."})
}

func (h PlanConfig) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       PlanConfigName,
		Permission: service.PermissionStructure,
		Summary: "Plans a configuration document - or, with to_version, a rollback - against the " +
			"configuration in force: every change by key path, before and after, the operations " +
			"it runs, what it refuses and why, whether it hides data or changes permissions, and a " +
			"plan hash to apply it by. Writes nothing.",
		SideEffects: "None.",
		TokenScope:  scopePlan,
		ReadOnly:    true,
		Input:       planFields(),
		Output:      planAnswer(),
		Audit:       usecase.AuditDeclaration{Action: PlannedAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a plan changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := hubOf(in)
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, PlannedAction, scopePlan, true); err != nil {
				return nil, err
			}
			p, _, _, _, err := h.Desk.planned(ctx, actor, in)
			if err != nil {
				return nil, err
			}
			return PlanOutput(p), nil
		}),
	}
}

// planAnswer is what every plan answers, as PlanOutput writes it: an agent reads the plan hash it
// applies or proposes by, and the refusals it corrects itself from.
func planAnswer() []usecase.Field {
	return []usecase.Field{
		{Name: "base_hash", Kind: usecase.KindString, Required: true, Description: "The hash of the configuration the plan was made on."},
		{Name: "plan_hash", Kind: usecase.KindString, Description: "The hash to apply or propose the plan by; null where the plan refuses."},
		{Name: "changes", Kind: usecase.KindList, Required: true, Description: "Each change by key path, with before and after."},
		{Name: "operations", Kind: usecase.KindList, Required: true, Description: "The use cases the plan runs, in order."},
		{Name: "refusals", Kind: usecase.KindList, Required: true, Description: "What the plan refuses: path, code and parameters."},
		{Name: "flags", Kind: usecase.KindObject, Required: true, Description: "hides_data and changes_permissions."},
		{Name: "step_up", Kind: usecase.KindBool, Required: true, Description: "Whether applying needs a fresh step-up proof."},
		{Name: "reach", Kind: usecase.KindList, Required: true, Description: "The entries and people the plan reaches."},
		{Name: "counts", Kind: usecase.KindObject, Required: true, Description: "added, changed and removed."},
	}
}

func applyFields() []usecase.Field {
	return append(planFields(),
		usecase.Field{Name: "plan_hash", Kind: usecase.KindString, Required: true, Description: "The plan's hash, as PlanConfig answered it."},
		usecase.Field{Name: "base_hash", Kind: usecase.KindString, CallerOnly: true, Description: "The hash of the configuration the plan was made on (If-Match), so a changed document is told from a changed configuration."},
		usecase.Field{Name: "acknowledge", Kind: usecase.KindBool, Description: "True once the person has seen that the plan hides data or changes permissions; required when it does."},
		usecase.Field{Name: "template", Kind: usecase.KindString, Description: "The template@version the document came from, recorded as the workspace's provenance - or, with hub_id, as the hub's, for a template with a hub part."},
		usecase.Field{Name: "step_up_token", Kind: usecase.KindString, CallerOnly: true, Description: "The fresh proof from /auth/step-up, needed where the plan changes the sign-in rule (step_up)."},
	)
}

// ProposeConfig keeps a plan for a person to apply or decline (D-16).
type ProposeConfig struct{ Desk Desk }

func (h ProposeConfig) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ProposeConfigName,
		Permission: service.PermissionStructure,
		Summary: "Proposes a planned configuration change for a person to apply or decline: it " +
			"appears in the administration's Changes as proposed, with the same plan. The way an " +
			"agent with read and plan scopes brings a change to the workspace.",
		SideEffects: "Stores the proposal and writes an audit entry. Changes no configuration.",
		TokenScope:  scopePlan,
		Input:       applyFields(),
		Audit:       usecase.AuditDeclaration{Action: ProposedAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity:    usecase.ActivityDeclaration{Exempt: "configuration is not an item; the proposal is in the trail."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := hubOf(in)
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, ProposedAction, scopePlan, false); err != nil {
				return nil, err
			}
			return h.Desk.propose(ctx, actor, in)
		}),
	}
}

func (d Desk) propose(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	var out usecase.Output
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		document, mode, _, err := d.desired(ctx, in)
		if err != nil {
			return err
		}
		hub, err := hubOf(in)
		if err != nil {
			return err
		}
		now, err := d.export(ctx, actor, hub)
		if err != nil {
			return err
		}
		p := d.planOf(now, document, mode)
		if err := held(p, in.String("plan_hash")); err != nil {
			return err
		}
		at := d.Clock.Now()
		proposal := domain.Proposal{
			ID: d.IDs.NewID(), TenantID: actor.TenantID, State: domain.ProposalWaiting, Document: document,
			Mode: mode, BaseHash: p.Base, PlanHash: p.Hash, Template: in.String("template"),
			Counts: domain.CountsOf(p.Changes), Flags: p.Flags, ProposedBy: actor.AccountID, CreatedAt: at,
			HubID: hub,
		}
		if err := d.Store.InsertProposal(ctx, proposal); err != nil {
			return err
		}
		out = usecase.Output{"state": "PROPOSED", "proposal": ProposalOutput(proposal)}
		return d.trail(ctx, actor, ProposedAction, at,
			audit.Change{Field: "proposal", Classification: audit.Open, To: proposal.ID.String()},
			audit.Change{Field: "plan_hash", Classification: audit.Open, To: p.Hash})
	})
	return out, err
}

// held refuses a plan that refuses, or that is not the plan the caller saw.
func held(p plan, planHash string) error {
	if len(p.Refusals) > 0 {
		return domain.ErrRefused(p.Refusals)
	}
	if planHash != p.Hash {
		return domain.ErrPlanMismatch()
	}
	return nil
}

// ApplyConfig applies a plan by its hash, all or nothing.
type ApplyConfig struct{ Desk Desk }

func (h ApplyConfig) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ApplyConfigName,
		Permission: service.PermissionStructure,
		Summary: "Applies a planned configuration document, or a rollback, by the plan's hash - all or " +
			"nothing, against the configuration it was planned on: a configuration changed since is " +
			"refused as config.version_conflict. A plan that hides data or changes permissions needs " +
			"acknowledge. From an agent it waits for a person's approval unless the workspace allows " +
			"agents to apply (settings.config.agent_apply), and is answered as PROPOSED.",
		SideEffects: "Runs every operation of the plan in one transaction, writes a configuration " +
			"version and an audit entry naming the actor and the counts. Each operation writes its own " +
			"audit entry and announcements as it does through every other door.",
		TokenScope:  scopeApply,
		Destructive: true,
		Input:       applyFields(),
		Audit:       usecase.AuditDeclaration{Action: AppliedAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity:    usecase.ActivityDeclaration{Exempt: "configuration is not an item; the version and the trail hold the change."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := hubOf(in)
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, AppliedAction, scopeApply, false); err != nil {
				return nil, err
			}
			return h.Desk.apply(ctx, actor, in)
		}),
	}
}

func (d Desk) apply(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	if actor.Kind == appshared.ActorAIAgent {
		var p policy
		var p0 plan
		err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			p, err = d.agentApply(ctx, actor.TenantID)
			if err != nil {
				return err
			}
			p0, _, _, _, err = d.planned(ctx, actor, in)
			return err
		})
		if err != nil {
			return nil, err
		}
		switch domain.AgentApply(p.value) {
		case domain.AgentApplyNever:
			return nil, domain.ErrAgentApplyNever()
		case domain.AgentApplyApproval:
			return d.propose(ctx, actor, in)
		}
		// An agent never loosens what holds it: a plan that touches the policy waits for a person.
		if p0.Policy != nil {
			return d.propose(ctx, actor, in)
		}
	}
	var out usecase.Output
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		document, mode, rollback, err := d.desired(ctx, in)
		if err != nil {
			return err
		}
		hub, err := hubOf(in)
		if err != nil {
			return err
		}
		now, err := d.export(ctx, actor, hub)
		if err != nil {
			return err
		}
		p := d.planOf(now, document, mode)
		if len(p.Refusals) == 0 && in.String("plan_hash") != p.Hash {
			// Same base, other hash: the document is not the one planned. Otherwise the
			// configuration moved under the plan, through this door or any other.
			if base := in.String("base_hash"); base != "" && base == p.Base {
				return domain.ErrPlanMismatch()
			}
			return domain.ErrVersionConflict()
		}
		if err := held(p, in.String("plan_hash")); err != nil {
			return err
		}
		if p.Flags.Any() && !in.Bool("acknowledge") {
			return domain.ErrNotAcknowledged()
		}
		if err := d.run(ctx, actor, p.Ops, in.String("step_up_token")); err != nil {
			return err
		}
		template, err := d.provenance(ctx, actor, in.String("template"), hub)
		if err != nil {
			return err
		}
		revision, err := d.applied(ctx, actor, p, "", rollback, template.label, d.Clock.Now())
		if err != nil {
			return err
		}
		if err := template.record(ctx, d); err != nil {
			return err
		}
		out = usecase.Output{"state": "APPLIED", "version": RevisionOutput(revision)}
		return nil
	})
	return out, err
}

// ListConfigVersions answers the history.
type ListConfigVersions struct{ Desk Desk }

func (h ListConfigVersions) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListConfigVersionsName,
		Permission: service.PermissionStructure,
		Summary: "The configuration's versions, newest first: who applied each, whose proposal it " +
			"was, the counts of what it added, changed and removed, the template it took and the " +
			"version a rollback went back to.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "before", Kind: usecase.KindInt, Description: "Lists the versions before this one; omitted lists the newest."},
			hubField(),
		},
		Audit:    usecase.AuditDeclaration{Action: ReadAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := hubOf(in)
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			var rows []usecase.Output
			err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				revisions, err := h.Desk.Store.List(ctx, hub, in.Int("before"), maxListed)
				for _, revision := range revisions {
					rows = append(rows, RevisionOutput(revision))
				}
				return err
			})
			if rows == nil {
				rows = []usecase.Output{}
			}
			return usecase.Output{"data": rows}, err
		}),
	}
}

// RevisionOutput is a version as every door answers it.
func RevisionOutput(r domain.Revision) usecase.Output {
	out := usecase.Output{
		"version": r.Version, "hash": r.Hash, "plan_hash": r.PlanHash, "actor_kind": string(r.ActorKind),
		"actor_id": idOrNil(r.ActorID), "proposed_by": idOrNil(r.ProposedBy), "rolled_back_to": nil,
		"template": nil, "added": r.Counts.Added, "changed": r.Counts.Changed, "removed": r.Counts.Removed,
		"created_at": r.At, "hub_id": idOrNil(r.HubID),
	}
	if r.RolledBackTo > 0 {
		out["rolled_back_to"] = r.RolledBackTo
	}
	if r.Template != "" {
		out["template"] = r.Template
	}
	return out
}

func idOrNil(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

// ReadConfigVersion answers one version's document, so two versions are compared.
type ReadConfigVersion struct{ Desk Desk }

func (h ReadConfigVersion) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ReadConfigVersionName,
		Permission:  service.PermissionStructure,
		Summary:     "One configuration version with its document, as it was in force after that apply.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "version", Kind: usecase.KindInt, Required: true, Description: "The version."},
		},
		Audit:    usecase.AuditDeclaration{Action: ReadAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			var out usecase.Output
			err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				revision, err := h.Desk.Store.Find(ctx, in.Int("version"))
				if err != nil {
					return err
				}
				out = RevisionOutput(revision)
				out["document"] = documentMap(revision.Document)
				return nil
			})
			return out, err
		}),
	}
}

// ProposalOutput is a proposal as every door answers it.
func ProposalOutput(p domain.Proposal) usecase.Output {
	out := usecase.Output{
		"id": p.ID.String(), "state": string(p.State), "document": documentMap(p.Document), "mode": string(p.Mode),
		"base_hash": p.BaseHash, "plan_hash": p.PlanHash, "template": nil,
		"added": p.Counts.Added, "changed": p.Counts.Changed, "removed": p.Counts.Removed,
		"hides_data": p.Flags.HidesData, "changes_permissions": p.Flags.ChangesPermissions,
		"proposed_by": idOrNil(p.ProposedBy), "decided_by": idOrNil(p.DecidedBy),
		"created_at": p.CreatedAt, "decided_at": nil, "hub_id": idOrNil(p.HubID),
	}
	if p.Template != "" {
		out["template"] = p.Template
	}
	if p.DecidedAt != nil {
		out["decided_at"] = *p.DecidedAt
	}
	return out
}

// ListConfigProposals answers the plans waiting for a person, or every one.
type ListConfigProposals struct{ Desk Desk }

func (h ListConfigProposals) Descriptor() usecase.Descriptor {
	states := []string{string(domain.ProposalWaiting), string(domain.ProposalApplied), string(domain.ProposalDeclined)}
	return usecase.Descriptor{
		Name:       ListConfigProposalsName,
		Permission: service.PermissionStructure,
		Summary: "The configuration changes proposed by agents (and people) for a person to apply or " +
			"decline, newest first, with their counts and whether they hide data or change permissions.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "state", Kind: usecase.KindString, Enum: states, Description: "Only proposals in this state; omitted lists every one."},
		},
		Audit:    usecase.AuditDeclaration{Action: ReadAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				proposals, err := h.Desk.Store.ListProposals(ctx, domain.ProposalState(strings.ToUpper(in.String("state"))), maxListed)
				for _, proposal := range proposals {
					rows = append(rows, ProposalOutput(proposal))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// DecideConfigProposal applies or declines a proposal: a person's decision, never an agent's.
type DecideConfigProposal struct{ Desk Desk }

func (h DecideConfigProposal) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       DecideConfigProposalName,
		Permission: service.PermissionStructure,
		Summary: "Applies or declines a proposed configuration change. Applying re-plans it against " +
			"the configuration in force and refuses it as config.version_conflict where that moved " +
			"since it was proposed; a plan that hides data or changes permissions needs acknowledge. " +
			"An agent cannot decide a proposal.",
		SideEffects: "Applying runs the plan as ApplyConfig does and writes a version naming who " +
			"proposed and who approved; declining keeps the proposal as declined. Both write an audit entry.",
		TokenScope:  scopeApply,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "proposal_id", Kind: usecase.KindID, Required: true, Description: "The proposal."},
			{Name: "decision", Kind: usecase.KindString, Required: true, Enum: []string{"APPLY", "DECLINE"}, Description: "APPLY or DECLINE."},
			{Name: "acknowledge", Kind: usecase.KindBool, Description: "True once the person has seen that the plan hides data or changes permissions."},
			{Name: "step_up_token", Kind: usecase.KindString, CallerOnly: true, Description: "The fresh proof from /auth/step-up, needed where the plan changes the sign-in rule."},
		},
		Audit:    usecase.AuditDeclaration{Action: AppliedAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "configuration is not an item; the version and the trail hold the change."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if actor.Kind == appshared.ActorAIAgent {
				return nil, shared.ErrForbidden.WithDetail("config.agent_cannot_decide")
			}
			if err := h.Desk.authorize(ctx, actor, AppliedAction, scopeApply, false); err != nil {
				return nil, err
			}
			id, err := in.ID("proposal_id")
			if err != nil {
				return nil, err
			}
			return h.Desk.decide(ctx, actor, id, strings.ToUpper(in.String("decision")) == "APPLY", in.Bool("acknowledge"), in.String("step_up_token"))
		}),
	}
}

func (d Desk) decide(ctx context.Context, actor appshared.ActorContext, id shared.ID, apply, acknowledge bool, stepUp string) (usecase.Output, error) {
	var out usecase.Output
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		proposal, err := d.Store.FindProposal(ctx, id)
		if err != nil {
			return err
		}
		if proposal.State != domain.ProposalWaiting {
			return domain.ErrProposalDecided()
		}
		at := d.Clock.Now()
		if !apply {
			moved, err := d.Store.DecideProposal(ctx, id, domain.ProposalDeclined, actor.AccountID, at)
			if err != nil {
				return err
			}
			if !moved {
				return domain.ErrProposalDecided()
			}
			proposal.State, proposal.DecidedBy, proposal.DecidedAt = domain.ProposalDeclined, actor.AccountID, &at
			out = usecase.Output{"state": "DECLINED", "proposal": ProposalOutput(proposal)}
			return d.trail(ctx, actor, DeclinedAction, at,
				audit.Change{Field: "proposal", Classification: audit.Open, To: id.String()})
		}
		now, err := d.export(ctx, actor, proposal.HubID)
		if err != nil {
			return err
		}
		if now.hash() != proposal.BaseHash {
			return domain.ErrVersionConflict()
		}
		p := d.planOf(now, proposal.Document, proposal.Mode)
		if err := held(p, proposal.PlanHash); err != nil {
			return err
		}
		if p.Flags.Any() && !acknowledge {
			return domain.ErrNotAcknowledged()
		}
		if err := d.run(ctx, actor, p.Ops, stepUp); err != nil {
			return err
		}
		template, err := d.provenance(ctx, actor, proposal.Template, proposal.HubID)
		if err != nil {
			return err
		}
		revision, err := d.applied(ctx, actor, p, proposal.ProposedBy, 0, template.label, at)
		if err != nil {
			return err
		}
		if err := template.record(ctx, d); err != nil {
			return err
		}
		moved, err := d.Store.DecideProposal(ctx, id, domain.ProposalApplied, actor.AccountID, at)
		if err != nil {
			return err
		}
		if !moved {
			return domain.ErrProposalDecided()
		}
		proposal.State, proposal.DecidedBy, proposal.DecidedAt = domain.ProposalApplied, actor.AccountID, &at
		out = usecase.Output{"state": "APPLIED", "version": RevisionOutput(revision), "proposal": ProposalOutput(proposal)}
		return nil
	})
	return out, err
}

// SetConfigPolicy sets what an agent's apply does in the workspace (D-16).
type SetConfigPolicy struct{ Desk Desk }

func (h SetConfigPolicy) Descriptor() usecase.Descriptor {
	options := domain.AgentApplyDefinition().Options
	return usecase.Descriptor{
		Name:       SetConfigPolicyName,
		Permission: service.PermissionStructure,
		Summary: "Sets what an agent's configuration apply does in the workspace: APPROVAL (the " +
			"default) keeps it for a person to apply or decline, ALLOWED applies it, NEVER refuses " +
			"it. The installation or a plan may lock it; the workspace may only tighten what they leave.",
		SideEffects: "Stores the setting and writes an audit entry.",
		TokenScope:  scopeApply,
		Input: []usecase.Field{
			{Name: "agent_apply", Kind: usecase.KindString, Required: true, Enum: options, Description: "ALLOWED, APPROVAL or NEVER."},
		},
		Audit:    usecase.AuditDeclaration{Action: PolicySetAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a setting is not an item; the change is in the trail."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if actor.Kind == appshared.ActorAIAgent {
				return nil, shared.ErrForbidden.WithDetail("config.agent_cannot_decide")
			}
			if err := h.Desk.authorize(ctx, actor, PolicySetAction, scopeApply, false); err != nil {
				return nil, err
			}
			value := strings.ToUpper(in.String("agent_apply"))
			definition := domain.AgentApplyDefinition()
			err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				instance, planLayer, err := h.Desk.Settings.Above(ctx, "config.")
				if err != nil {
					return err
				}
				above := setting.Resolve(definition, instance, planLayer)
				entry := setting.Entry{Value: value, Hold: setting.HoldSet, By: actor.AccountID}
				if err := setting.Check(definition, setting.LevelWorkspace, above, entry); err != nil {
					return err
				}
				now := h.Desk.Clock.Now()
				place := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: actor.TenantID}
				if err := h.Desk.Settings.Put(ctx, h.Desk.IDs.NewID(), place, domain.AgentApplyKey, entry, now); err != nil {
					return err
				}
				return h.Desk.trail(ctx, actor, PolicySetAction, now,
					audit.Change{Field: string(domain.AgentApplyKey), Classification: audit.Open, To: value})
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"agent_apply": value}, nil
		}),
	}
}
