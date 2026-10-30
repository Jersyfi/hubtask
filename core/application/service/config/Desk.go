// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package config holds the use cases of the configuration document (F-6, D-15 … D-17): its schema,
// export and explanation, validate, plan and apply with history and rollback, the agents' plans a
// person approves, and templates with their three-way upgrade.
//
// The document is read and written through the use cases every other door reaches - a section is
// declared by its list, create, change and remove operations - so a plan applied here is held to
// exactly what the screen, `hubctl` and MCP are held to (P-18), and one apply is atomic because
// every one of those operations joins its transaction.
package config

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	configrepo "github.com/Jersyfi/hubtask/core/application/repository/config"
	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	// The scopes of config-document.md §3: an agent token gets read and plan.
	scopeRead  = "config:read"
	scopePlan  = "config:plan"
	scopeApply = "config:apply"

	target = "configuration"

	// Stable audit codes (audit.md §2).
	ReadAction      audit.Action = "config.read"
	PlannedAction   audit.Action = "config.planned"
	AppliedAction   audit.Action = "config.applied"
	ProposedAction  audit.Action = "config.proposed"
	DeclinedAction  audit.Action = "config.declined"
	PolicySetAction audit.Action = "config.policy_set"
	TemplateAction  audit.Action = "config.template_published"
)

// Catalogue is the registry as this package needs it: the door every block is read and written
// through, and the declarations a schema and a validation are taken from.
type Catalogue interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
	Lookup(name string) (usecase.Descriptor, bool)
}

// Authorizer is the one authorisation question this package asks.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Desk is what every configuration use case shares.
type Desk struct {
	Catalogue  Catalogue
	Store      configrepo.Store
	Settings   settingrepo.Settings
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// current is the document in force with the rows behind it, so that a change names the block by
// what its use case takes (a field's identifier) while the document names it by key.
type current struct {
	Document domain.Document
	Rows     map[string]map[string]map[string]any
	// Withheld are the sections the reader may not read or whose module is off; WithheldSettings
	// the key prefixes of the settings areas the reader may not read.
	Withheld         []string
	WithheldSettings []string
	// Hub is the hub whose own document this is, empty for the workspace's.
	Hub shared.ID
	// Places are the hubs and collections by handle, and Account the reader - who a rule the
	// document brings acts as unless it names somebody.
	Places  places
	Account shared.ID
}

func (c current) hash() string { return domain.Hash(c.Document) }

// sections is the shape of the document in force: the workspace's, or a hub's.
func (c current) sections() []section {
	if c.Hub.IsZero() {
		return sections()
	}
	return hubSections(c.Hub)
}

// authorize asks for the workspace's configuration: STRUCTURE to change it, or READ_CONFIGURATION
// besides for a read, so an auditor reads what the workspace is configured to do (A-4).
func (d Desk) authorize(ctx context.Context, actor appshared.ActorContext, action audit.Action, scope string, read bool) error {
	return d.authorizeAt(ctx, actor, "", action, scope, read)
}

// authorizeAt asks the same at a hub for the hub's own document: STRUCTURE there, which a
// workspace's holds on every hub it reaches.
func (d Desk) authorizeAt(ctx context.Context, actor appshared.ActorContext, hub shared.ID, action audit.Action, scope string, read bool) error {
	request := access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: scope, TargetType: target, TargetID: actor.TenantID,
	}
	if !hub.IsZero() {
		// A hub the reader does not reach is not found, before any permission is asked of it, in
		// the words a missing one produces (T-04).
		if err := d.isHub(ctx, actor, hub); err != nil {
			return err
		}
		request.Path = append(request.Path, identity.HubScope(hub))
	}
	if read {
		request.Alternative = service.PermissionReadConfiguration
	}
	return d.Authorizer.Authorize(ctx, actor, request)
}

// export reads every section through its list use case, in the transaction the caller runs in:
// the workspace's document, or for a hub the hub's own.
func (d Desk) export(ctx context.Context, actor appshared.ActorContext, hub shared.ID) (current, error) {
	out := current{Document: domain.Document{}, Rows: map[string]map[string]map[string]any{}, Hub: hub, Account: actor.AccountID}
	if !hub.IsZero() {
		if err := d.isHub(ctx, actor, hub); err != nil {
			return current{}, err
		}
	}
	placed, err := d.readPlaces(ctx, actor, hub)
	if err != nil && !withheld(err) {
		return current{}, err
	}
	if err != nil {
		// A reader who may not list the places reads identifiers where handles would be.
		placed = places{key: map[string]string{}, id: map[string]string{}, hub: hub.String()}
	}
	out.Places = placed
	for _, s := range out.sections() {
		answer, err := d.list(ctx, actor, s)
		if err != nil {
			if withheld(err) {
				out.Withheld = append(out.Withheld, s.name)
				continue
			}
			return current{}, err
		}
		normal, err := normalise(map[string]any(answer))
		if err != nil {
			return current{}, err
		}
		rows, _ := normal.(map[string]any)[s.rows].([]any)
		if s.rowsOf != nil {
			rows = s.rowsOf(normal.(map[string]any))
		}
		blocks := domain.Section{}
		byKey := map[string]map[string]any{}
		for _, raw := range rows {
			row, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			key, block := s.read(row)
			if key == "" {
				continue
			}
			if s.present != nil {
				block = s.present(out.Places, block)
			}
			blocks[key] = block
			byKey[key] = row
		}
		out.Document[s.name] = blocks
		out.Rows[s.name] = byKey
	}
	if !hub.IsZero() {
		return out, nil
	}
	policy, err := d.agentApply(ctx, actor.TenantID)
	if err != nil {
		return current{}, err
	}
	settings := domain.Section{settingKey(domain.AgentApplyKey): domain.Block{settingValueField: policy.value}}
	rows := map[string]map[string]any{}
	for _, area := range settingAreas() {
		answer, err := d.Catalogue.Invoke(ctx, area.read, actor, usecase.Input{})
		if err != nil {
			if withheld(err) {
				out.WithheldSettings = append(out.WithheldSettings, area.prefixes...)
				continue
			}
			return current{}, err
		}
		normal, err := normalise(map[string]any(answer))
		if err != nil {
			return current{}, err
		}
		answerMap, _ := normal.(map[string]any)
		for key, row := range area.rows(answerMap) {
			rows[key] = row
			if block := settingBlock(key, row); block != nil {
				settings[key] = block
			}
		}
	}
	out.Document[sectionSettings] = settings
	out.Rows[sectionSettings] = rows
	return out, nil
}

// list reads a section's rows through its list use case, every page of a paged one.
func (d Desk) list(ctx context.Context, actor appshared.ActorContext, s section) (usecase.Output, error) {
	if !s.paged {
		return d.Catalogue.Invoke(ctx, s.list, actor, s.listInput)
	}
	var rows []any
	if err := d.eachPage(ctx, actor, s.list, s.listInput, func(row map[string]any) { rows = append(rows, row) }); err != nil {
		return nil, err
	}
	return usecase.Output{"data": rows}, nil
}

// withheld reports whether a section's read was refused for a reason that leaves the rest of the
// document readable: its module is off, or the reader may not read it.
func withheld(err error) bool {
	var e *shared.Error
	if !errors.As(err, &e) {
		return false
	}
	return e.DetailCode == "module.off" || e.Category == shared.ErrForbidden.Category
}

// read is a row as the document keeps it: the declared attributes, nothing empty.
func (s section) read(row map[string]any) (string, domain.Block) {
	if s.block != nil {
		return s.block(row)
	}
	block := domain.Block{}
	for _, attribute := range s.attributes {
		if value, held := row[attribute]; held && value != nil {
			block[attribute] = value
		}
	}
	return text(row[s.key]), block
}

// normalise is a value in the document's shapes: what a client's JSON decodes to, so that an
// answer and a request compare and hash alike.
func normalise(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("config.document_unreadable").WithCause(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, shared.ErrInternal.WithDetail("config.document_unreadable").WithCause(err)
	}
	return out, nil
}

// documentOf reads a document a door handed in: sections of blocks by key, a null block removing.
func documentOf(raw any) (domain.Document, []shared.FieldError) {
	normal, err := normalise(raw)
	if err != nil {
		return nil, []shared.FieldError{{Path: "/document", Code: "config.document_invalid"}}
	}
	sectionsIn, ok := normal.(map[string]any)
	if !ok {
		return nil, []shared.FieldError{{Path: "/document", Code: "config.document_invalid"}}
	}
	out := domain.Document{}
	var refusals []shared.FieldError
	for name, rawSection := range sectionsIn {
		blocks, ok := rawSection.(map[string]any)
		if !ok {
			refusals = append(refusals, domain.Refusal("config.document_invalid", name, nil))
			continue
		}
		section := domain.Section{}
		for key, rawBlock := range blocks {
			if rawBlock == nil {
				section[key] = nil
				continue
			}
			block, ok := rawBlock.(map[string]any)
			if !ok {
				refusals = append(refusals, domain.Refusal("config.document_invalid", name+"."+key, nil))
				continue
			}
			section[key] = domain.Block(block)
		}
		out[name] = section
	}
	return out, refusals
}

// plan is what a plan answers: the document it leads to, its changes in apply order with the
// operations they run, what it refuses, what it must have acknowledged, and its hash.
type plan struct {
	Base     string
	Target   domain.Document
	Changes  []domain.BlockChange
	Ops      []operation
	Refusals []shared.FieldError
	Flags    domain.Flags
	Hash     string
	Policy   *domain.AgentApply
	// StepUp is whether the plan changes the sign-in rule, which its apply proves afresh.
	StepUp bool
	// Reach is what the changes touch beyond the configuration, where a plan is read for a person.
	Reach []reach
	// Hub is the hub whose own document the plan changes, empty for the workspace's.
	Hub shared.ID
	// hiddenFields are the fields the plan removes or hides from readers, by the change's path,
	// with the identifier their reach is counted by.
	hiddenFields []hiddenField
}

// hiddenField is one field a plan removes or hides from readers.
type hiddenField struct{ Path, ID string }

// planOf compares a desired document, in its mode, with the one in force and checks every change
// against its section's declaration and the input its use case declares - refusals only, nothing
// run.
func (d Desk) planOf(now current, desired domain.Document, mode domain.Mode) plan {
	p := plan{Base: now.hash(), Hub: now.Hub}
	known := map[string]section{}
	for _, s := range now.sections() {
		known[s.name] = s
	}
	for _, name := range desired.Sections() {
		if _, held := known[name]; !held && (name != sectionSettings || !now.Hub.IsZero()) {
			p.Refusals = append(p.Refusals, domain.Refusal("config.section_unknown", name, nil))
		}
		for _, w := range now.Withheld {
			if w == name {
				p.Refusals = append(p.Refusals, domain.Refusal("config.section_withheld", name, nil))
			}
		}
		for key := range desired[name] {
			if !domain.ValidKey(key) && name != sectionSettings {
				p.Refusals = append(p.Refusals, domain.Refusal("config.key_invalid", name+"."+key, nil))
			}
			if name == sectionSettings {
				for _, prefix := range now.WithheldSettings {
					if strings.HasPrefix(key, prefix) {
						p.Refusals = append(p.Refusals, domain.Refusal("config.section_withheld", name+"."+key, nil))
					}
				}
			}
		}
	}
	if len(p.Refusals) > 0 {
		return p
	}
	p.Target = domain.Resolve(now.Document, desired, mode)
	for _, w := range now.Withheld {
		// A section nobody read is not compared: it is neither in force nor removed.
		delete(p.Target, w)
	}
	for key := range p.Target[sectionSettings] {
		for _, prefix := range now.WithheldSettings {
			if strings.HasPrefix(key, prefix) {
				delete(p.Target[sectionSettings], key)
			}
		}
	}
	changes := domain.Diff(now.Document, p.Target)
	var removals []operation
	var settingChanges []domain.BlockChange
	for _, s := range append(now.sections(), section{name: sectionSettings}) {
		early := len(p.Ops)
		for _, change := range changes {
			if change.Section != s.name {
				continue
			}
			p.Changes = append(p.Changes, change)
			if s.name == sectionSettings {
				settingChanges = append(settingChanges, change)
				if change.Key == settingKey(domain.AgentApplyKey) {
					value := domain.AgentApply(text(change.After[settingValueField]))
					p.Policy = &value
				}
				continue
			}
			if refusals := d.check(s, change, now.Rows[s.name][change.Key]); len(refusals) > 0 {
				p.Refusals = append(p.Refusals, refusals...)
				continue
			}
			var ops []operation
			var err error
			if s.opsAt != nil {
				ops, err = s.opsAt(now, change, now.Rows[s.name][change.Key])
			} else {
				ops, err = s.operations(change, now.Rows[s.name][change.Key])
			}
			if err != nil {
				p.Refusals = append(p.Refusals, fieldsOf(err, change.Path())...)
				continue
			}
			for i := range ops {
				ops[i].Input = withoutEmpty(ops[i].Input, change.Op == domain.OpAdd)
				if descriptor, found := d.Catalogue.Lookup(ops[i].UseCase); found {
					if err := descriptor.ValidateInput(ops[i].Input); err != nil {
						p.Refusals = append(p.Refusals, fieldsOf(err, change.Path())...)
					}
				}
			}
			switch {
			case s.releases != nil && s.releases(change):
				p.Ops = append(p.Ops[:early], append(ops, p.Ops[early:]...)...)
				early += len(ops)
			case change.Op == domain.OpRemove:
				removals = append(ops, removals...)
			default:
				p.Ops = append(p.Ops, ops...)
			}
			if s.hides != nil && s.hides(change) {
				p.Flags.HidesData = true
				if s.name == sectionFields {
					p.hiddenFields = append(p.hiddenFields, hiddenField{
						Path: change.Path(), ID: text(now.Rows[s.name][change.Key][s.locateFrom]),
					})
				}
			}
			if s.permissions {
				p.Flags.ChangesPermissions = true
			}
		}
	}
	ops, refusals := settingsOps(settingChanges, now.Rows[sectionSettings])
	p.Refusals = append(p.Refusals, refusals...)
	p.Ops = append(p.Ops, ops...)
	for _, op := range ops {
		p.StepUp = p.StepUp || op.StepUp
	}
	p.Ops = append(p.Ops, removals...)
	if len(p.Refusals) == 0 {
		p.Hash = domain.PlanHash(p.Base, p.Target)
	}
	return p
}

// check holds one change to its section's declaration.
func (d Desk) check(s section, change domain.BlockChange, row map[string]any) []shared.FieldError {
	var refusals []shared.FieldError
	path := change.Path()
	for _, attribute := range change.Fields {
		if !s.writable(attribute) {
			refusals = append(refusals, domain.Refusal("config.attribute_unknown", path+"."+attribute, nil))
		}
	}
	switch change.Op {
	case domain.OpAdd:
		if s.create == "" && s.ops == nil {
			refusals = append(refusals, domain.Refusal("config.key_unknown", path, nil))
		}
		if s.name == sectionModules {
			refusals = append(refusals, domain.Refusal("config.key_unknown", path, nil))
		}
	case domain.OpChange:
		if s.ops != nil {
			break
		}
		for _, attribute := range change.Fields {
			if !s.writable(attribute) {
				continue
			}
			if _, separate := s.separate[attribute]; separate {
				continue
			}
			if !contains(s.changeable, attribute) {
				refusals = append(refusals, domain.Refusal("config.attribute_fixed", path+"."+attribute, nil))
			}
		}
	case domain.OpRemove:
		if system, _ := row[s.system].(bool); (s.system != "" && system) || (s.remove == "" && s.ops == nil) {
			refusals = append(refusals, domain.Refusal("config.not_removable", path, nil))
		}
	}
	return refusals
}

// fieldsOf turns a refusal into field errors under the change's key path.
func fieldsOf(err error, path string) []shared.FieldError {
	var e *shared.Error
	if !errors.As(err, &e) {
		return []shared.FieldError{domain.Refusal("config.refused", path, nil)}
	}
	if len(e.Fields) == 0 {
		code := e.DetailCode
		if code == "" {
			code = e.Code
		}
		return []shared.FieldError{domain.Refusal(code, path, e.Params)}
	}
	out := make([]shared.FieldError, 0, len(e.Fields))
	for _, field := range e.Fields {
		fe := domain.Refusal(field.Code, path, field.Params)
		fe.Path += field.Path
		out = append(out, fe)
	}
	return out
}

// withoutEmpty leaves out what a create would read as a value it refuses: an empty list or text.
func withoutEmpty(in usecase.Input, creating bool) usecase.Input {
	if !creating {
		return in
	}
	out := usecase.Input{}
	for name, value := range in {
		switch v := value.(type) {
		case string:
			if v == "" {
				continue
			}
		case []any:
			if len(v) == 0 {
				continue
			}
		case nil:
			continue
		}
		out[name] = value
	}
	return out
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func refused(code, path string, params map[string]string) error {
	return domain.ErrRefused([]shared.FieldError{domain.Refusal(code, path, params)})
}

// settingKey is a setting's block key in the `settings` section: the catalogue key, which the
// document names as it is.
func settingKey(key setting.Key) string { return string(key) }

// policy is the agents' apply setting as the workspace resolves it.
type policy struct {
	value  string
	locked bool
	source setting.Level
}

func (d Desk) agentApply(ctx context.Context, tenant shared.ID) (policy, error) {
	definition := domain.AgentApplyDefinition()
	resolution, err := d.resolveAgentApply(ctx, definition, tenant)
	if err != nil {
		return policy{}, err
	}
	return policy{value: string(domain.AgentApplyOf(resolution)), locked: resolution.Locked(), source: resolution.Source}, nil
}

func (d Desk) resolveAgentApply(ctx context.Context, definition setting.Definition, tenant shared.ID) (setting.Resolution, error) {
	if d.Settings == nil {
		return setting.Resolve(definition), nil
	}
	instance, planLayer, err := d.Settings.Above(ctx, "config.")
	if err != nil {
		return setting.Resolution{}, err
	}
	workspace := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: tenant}
	stored, err := d.Settings.At(ctx, []settingrepo.Place{workspace}, "config.")
	if err != nil {
		return setting.Resolution{}, err
	}
	layers := append([]setting.Layer{instance, planLayer}, settingrepo.LayersOf([]settingrepo.Place{workspace}, stored)...)
	return setting.Resolve(definition, layers...), nil
}

// run applies a plan's operations in the caller's transaction; the first refusal ends it, named
// by the key path it was for.
// stepUp is the fresh proof an apply carries, handed to the operation changing the sign-in rule.
// ApplyScopes are the per-section scopes of an apply (config-document.md §3): a token holding
// `config:apply:<section>` beside `config:apply` applies that section's blocks without holding each
// block's own scope - an agent's token is given the sections it configures, not the areas' scopes.
// The person's permission is asked by every block's use case as always; only the credential's
// bound moves.
func ApplyScopes() []string {
	names := map[string]bool{}
	for _, s := range sections() {
		names[s.name] = true
	}
	for _, s := range hubSections("") {
		names[s.name] = true
	}
	names[sectionSettings] = true
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, scopeApply+":"+name)
	}
	slices.Sort(out)
	return out
}

// blockActor is the actor a block's operation runs as within an apply: the block's own scope
// added where the token holds the section's apply scope.
func (d Desk) blockActor(actor appshared.ActorContext, op operation) appshared.ActorContext {
	descriptor, found := d.Catalogue.Lookup(op.UseCase)
	section, _, _ := strings.Cut(op.Path, ".")
	return withScope(actor, descriptor.TokenScope, found && actor.HasScope(scopeApply) && actor.HasScope(scopeApply+":"+section))
}

// preview runs a read a plan's reach asks for. A token holding `config:plan` reads the previews of
// what it plans (the S-09 gap): a preview is a read and changes nothing, so the plan scope admits
// the read's own scope; the person's permission is asked as always.
func (d Desk) preview(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	descriptor, found := d.Catalogue.Lookup(name)
	return d.Catalogue.Invoke(ctx, name, withScope(actor, descriptor.TokenScope, found && descriptor.ReadOnly && actor.HasScope(scopePlan)), in)
}

func withScope(actor appshared.ActorContext, scope string, admitted bool) appshared.ActorContext {
	if !admitted || scope == "" || actor.HasScope(scope) {
		return actor
	}
	actor.Scopes = append(slices.Clone(actor.Scopes), scope)
	return actor
}

func (d Desk) run(ctx context.Context, actor appshared.ActorContext, ops []operation, stepUp string) error {
	var previous usecase.Output
	for _, op := range ops {
		in := op.Input
		if op.StepUp && stepUp != "" {
			in = usecase.Input{"step_up_token": stepUp}
			for k, v := range op.Input {
				in[k] = v
			}
		}
		if op.Chain != "" {
			in = chainedInput(in, previous, op.Chain)
		}
		answer, err := d.Catalogue.Invoke(ctx, op.UseCase, d.blockActor(actor, op), in)
		previous = answer
		if err != nil {
			var e *shared.Error
			if errors.As(err, &e) {
				params := map[string]string{"config_path": op.Path}
				for k, v := range e.Params {
					params[k] = v
				}
				return e.WithParams(params)
			}
			return err
		}
	}
	return nil
}

// chainedInput puts the previous answer's field where the operation holds the chained
// placeholder; an answer without it leaves the placeholder, which the use case refuses as unknown.
func chainedInput(in usecase.Input, previous usecase.Output, field string) usecase.Input {
	out := usecase.Input{}
	for k, v := range in {
		out[k] = v
		if v == chained {
			if id, held := previous[field]; held {
				out[k] = id
			}
		}
	}
	return out
}

// applied writes the version after an apply and its trail entry: who, the counts, never content.
func (d Desk) applied(
	ctx context.Context, actor appshared.ActorContext, p plan, proposedBy shared.ID, rolledBackTo int,
	template string, now time.Time,
) (domain.Revision, error) {
	after, err := d.export(ctx, actor, p.Hub)
	if err != nil {
		return domain.Revision{}, err
	}
	latest, _, err := d.Store.Latest(ctx)
	if err != nil {
		return domain.Revision{}, err
	}
	revision := domain.Revision{
		TenantID: actor.TenantID, Version: latest.Version + 1, Document: after.Document, Hash: after.hash(),
		PlanHash: p.Hash, ActorKind: actor.Kind, ActorID: actor.AccountID, ProposedBy: proposedBy,
		RolledBackTo: rolledBackTo, Template: template, Counts: domain.CountsOf(p.Changes), At: now,
		HubID: p.Hub,
	}
	if err := d.Store.Insert(ctx, revision); err != nil {
		return domain.Revision{}, err
	}
	changes := []audit.Change{
		{Field: "version", Classification: audit.Open, To: strconv.Itoa(revision.Version)},
		{Field: "added", Classification: audit.Open, To: strconv.Itoa(revision.Counts.Added)},
		{Field: "changed", Classification: audit.Open, To: strconv.Itoa(revision.Counts.Changed)},
		{Field: "removed", Classification: audit.Open, To: strconv.Itoa(revision.Counts.Removed)},
	}
	if !proposedBy.IsZero() {
		changes = append(changes, audit.Change{Field: "proposed_by", Classification: audit.Open, To: proposedBy.String()})
	}
	if template != "" {
		changes = append(changes, audit.Change{Field: "template", Classification: audit.Open, To: template})
	}
	if !p.Hub.IsZero() {
		changes = append(changes, audit.Change{Field: "hub_id", Classification: audit.Open, To: p.Hub.String()})
	}
	return revision, d.trail(ctx, actor, AppliedAction, now, changes...)
}

func (d Desk) trail(ctx context.Context, actor appshared.ActorContext, action audit.Action, now time.Time, changes ...audit.Change) error {
	return d.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: target, TargetID: actor.TenantID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}
