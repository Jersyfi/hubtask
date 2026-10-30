// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"errors"
	"maps"
	"sort"
	"testing"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// world is the workspace the fake doors read and write: entry types and custom roles.
type world struct {
	types map[string]map[string]any
	roles map[string]map[string]any
	// structure is the workspace's own structure values; nil withholds the section.
	structure map[string]any
	// workspace is what ReadWorkspace answers besides the structure; ai what ReadAiSource answers,
	// nil withholding it. inputs records what each writing use case was given.
	workspace map[string]any
	ai        map[string]any
	// locks and defaultPreset are the workspace structure's locks and the preset a new hub starts from.
	locks         map[string]any
	defaultPreset string
	inputs        map[string][]usecase.Input
	// failOn makes one use case refuse, as a block its own checks reject would.
	failOn string
	calls  []string
	// workflows, nil withholding the section, preview what PreviewWorkflowPublish answers and
	// grants the role grants ListRoles answers.
	workflows map[string]map[string]any
	preview   usecase.Output
	// previewWithheld makes the previews refuse the reader, as a token without their scope is.
	previewWithheld bool
	grants          []any
	// hub is the one hub GetContainer answers, hubStructure its own structure values and scheme
	// the workflow scheme ListWorkflows answers.
	hub          string
	hubStructure map[string]any
	scheme       []any
	// hubCalendars, nil withholding the section, are the hub's own calendars by key.
	hubCalendars map[string]map[string]any
	// fields, nil withholding the section, are the workspace's custom fields, and fieldReach what
	// PreviewCustomFieldReach answers for each.
	fields     map[string]map[string]any
	fieldReach int
}

func (w *world) clone() *world {
	out := &world{types: map[string]map[string]any{}, roles: map[string]map[string]any{}, failOn: w.failOn,
		workspace: w.workspace, ai: w.ai, inputs: maps.Clone(w.inputs), workflows: w.workflows, preview: w.preview, previewWithheld: w.previewWithheld, grants: w.grants,
		hub: w.hub, hubStructure: maps.Clone(w.hubStructure), scheme: w.scheme, hubCalendars: w.hubCalendars, fields: w.fields, fieldReach: w.fieldReach}
	if w.structure != nil {
		out.structure = maps.Clone(w.structure)
	}
	for k, v := range w.types {
		out.types[k] = maps.Clone(v)
	}
	for k, v := range w.roles {
		out.roles[k] = maps.Clone(v)
	}
	return out
}

type catalogue struct{ w *world }

func rowsOf(m map[string]map[string]any) []any {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

func (c catalogue) Invoke(_ context.Context, name string, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	w := c.w
	if name == w.failOn {
		return nil, shared.ErrValidation.WithDetail("entry_types.unknown")
	}
	switch name {
	case "ListEntryTypes":
		return usecase.Output{"data": rowsOf(w.types)}, nil
	case "ListCustomFields":
		if w.fields == nil {
			return nil, shared.ErrValidation.WithDetail("module.off")
		}
		return usecase.Output{"data": rowsOf(w.fields)}, nil
	case "PreviewCustomFieldReach":
		if w.inputs == nil {
			w.inputs = map[string][]usecase.Input{}
		}
		w.inputs[name] = append(w.inputs[name], in)
		if w.previewWithheld {
			return nil, shared.ErrForbidden.WithDetail("access.not_permitted")
		}
		return usecase.Output{"field_id": in["field_id"], "entries": w.fieldReach}, nil
	case "PreviewModuleReach":
		if w.inputs == nil {
			w.inputs = map[string][]usecase.Input{}
		}
		w.inputs[name] = append(w.inputs[name], in)
		if w.previewWithheld {
			return nil, shared.ErrForbidden.WithDetail("auth.scope_insufficient")
		}
		return usecase.Output{"module": in["module"], "on": true, "people": 7}, nil
	case "ListRoles":
		return usecase.Output{"custom": rowsOf(w.roles), "grants": w.grants}, nil
	case "ListWorkflows":
		if w.workflows == nil {
			return nil, shared.ErrValidation.WithDetail("module.off")
		}
		return usecase.Output{"data": rowsOf(w.workflows), "scheme": w.scheme}, nil
	case "GetContainer":
		if w.hub == "" || in.String("container_id") != w.hub {
			return usecase.Output{"type": "COLLECTION"}, nil
		}
		sources := map[string]any{}
		for name := range w.hubStructure {
			sources[name] = "HUB"
		}
		return usecase.Output{"type": "HUB", "structure": map[string]any{
			"settings": maps.Clone(w.hubStructure), "sources": sources,
		}}, nil
	case "PreviewContainerStructure", "PreviewWorkspaceStructure":
		if w.inputs == nil {
			w.inputs = map[string][]usecase.Input{}
		}
		w.inputs[name] = append(w.inputs[name], in)
		return usecase.Output{"narrows": true, "levels": []any{map[string]any{"type": "ACTIVITY", "entries": 3}},
			"fields": []any{map[string]any{"type": "TASK", "capability": "dates", "entries": 5}}}, nil
	case "ListHubCalendars":
		if w.hubCalendars == nil {
			return nil, shared.ErrValidation.WithDetail("module.off")
		}
		return usecase.Output{"data": rowsOf(w.hubCalendars)}, nil
	case "DefineHubCalendar", "DeleteHubCalendar":
		if w.inputs == nil {
			w.inputs = map[string][]usecase.Input{}
		}
		w.inputs[name] = append(w.inputs[name], in)
	case "SetContainerStructure", "SetWorkflowScheme":
		if w.inputs == nil {
			w.inputs = map[string][]usecase.Input{}
		}
		w.inputs[name] = append(w.inputs[name], in)
		if values, held := in["settings"].(map[string]any); held {
			maps.Copy(w.hubStructure, values)
		}
		if reset, held := in["reset"].([]any); held {
			for _, name := range reset {
				delete(w.hubStructure, name.(string))
			}
		}
	case "PreviewWorkflowPublish", "PreviewWorkflowScheme":
		if w.previewWithheld {
			return nil, shared.ErrForbidden.WithDetail("auth.scope_insufficient")
		}
		if w.inputs == nil {
			w.inputs = map[string][]usecase.Input{}
		}
		w.inputs[name] = append(w.inputs[name], in)
		return w.preview, nil
	case "ListModules":
		return usecase.Output{"modules": []any{map[string]any{"module": "WORKFLOWS", "on": true, "source": "WORKSPACE"}}}, nil
	case "CreateEntryType":
		row := map[string]any{"system": false}
		maps.Copy(row, in)
		w.types[in.String("key")] = row
	case "ChangeEntryType":
		maps.Copy(w.types[in.String("key")], in)
	case "DeleteEntryType":
		delete(w.types, in.String("key"))
	case "CreateRole", "ChangeRole":
		w.roles[in.String("key")] = map[string]any{"key": in["key"], "name": in["name"], "permissions": in["permissions"]}
	case "DeleteRole":
		delete(w.roles, in.String("key"))
	case "ReadWorkspace":
		if w.structure == nil {
			return nil, shared.ErrValidation.WithDetail("module.off")
		}
		sources := map[string]any{}
		for name := range w.structure {
			sources[name] = "WORKSPACE"
		}
		sources["views"] = "DEFAULT"
		out := usecase.Output{"structure": map[string]any{
			"settings": maps.Clone(w.structure), "sources": sources, "locks": w.locks, "default_preset": w.defaultPreset,
		}}
		maps.Copy(out, w.workspace)
		return out, nil
	case "ReadAiSource":
		if w.ai == nil {
			return nil, shared.ErrForbidden
		}
		return usecase.Output(maps.Clone(w.ai)), nil
	case "UpdateWorkspace", "ChooseAiSource", "SetAiPersonOptOut":
		if w.inputs == nil {
			w.inputs = map[string][]usecase.Input{}
		}
		w.inputs[name] = append(w.inputs[name], in)
	case "SetWorkspaceStructure":
		if w.inputs == nil {
			w.inputs = map[string][]usecase.Input{}
		}
		w.inputs[name] = append(w.inputs[name], in)
		if values, held := in["settings"].(map[string]any); held {
			maps.Copy(w.structure, values)
		}
		if reset, held := in["reset"].([]any); held {
			for _, name := range reset {
				delete(w.structure, name.(string))
			}
		}
	case "ListPresets":
		rows := []any{}
		for _, preset := range workmodel.BuiltInPresets() {
			rows = append(rows, map[string]any(work.PresetOutput(preset)))
		}
		rows = append(rows, map[string]any{
			"id": "0190a8f0-0000-7000-8000-0000000000cc", "origin": "INSTALLATION", "name": "Agency", "version": 3,
			"settings": map[string]any{"completion": "MANUAL"},
		})
		return usecase.Output{"data": rows}, nil
	case "SetModule":
	default:
		// Every other section is withheld, as a module switched off withholds it.
		return nil, shared.ErrValidation.WithDetail("module.off")
	}
	w.calls = append(w.calls, name)
	return usecase.Output{}, nil
}

func (catalogue) Lookup(name string) (usecase.Descriptor, bool) {
	if name != "CreateEntryType" {
		return usecase.Descriptor{}, false
	}
	return usecase.Descriptor{Name: name, Input: []usecase.Field{
		{Name: "key", Kind: usecase.KindString, Required: true},
		{Name: "level", Kind: usecase.KindString, Required: true, Enum: []string{"TASK", "WORK_PACKAGE", "ACTIVITY"}, Description: "The level."},
		{Name: "name", Kind: usecase.KindString, Required: true},
		{Name: "icon", Kind: usecase.KindString},
	}}, true
}

// unitOfWork rolls the world back when the outermost transaction fails: what makes an apply all
// or nothing in this test is what makes it so against the database.
type unitOfWork struct {
	w     *world
	store *store
	depth int
}

func (u *unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	if u.depth > 0 {
		return fn(ctx)
	}
	before, saved := u.w.clone(), u.store.clone()
	u.depth++
	err := fn(ctx)
	u.depth--
	if err != nil {
		*u.w = *before
		*u.store = *saved
	}
	return err
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, s persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, s, fn)
}

type store struct {
	revisions []domain.Revision
	proposals []domain.Proposal
	templates []domain.Template
	uses      []domain.TemplateUse
	sandbox   *domain.Sandbox
}

func (s *store) clone() *store {
	return &store{
		revisions: append([]domain.Revision(nil), s.revisions...), proposals: append([]domain.Proposal(nil), s.proposals...),
		templates: append([]domain.Template(nil), s.templates...), uses: append([]domain.TemplateUse(nil), s.uses...),
		sandbox: s.sandbox,
	}
}

func (s *store) Sandbox(context.Context) (domain.Sandbox, bool, error) {
	if s.sandbox == nil {
		return domain.Sandbox{}, false, nil
	}
	return *s.sandbox, true, nil
}
func (s *store) PutSandbox(_ context.Context, sandbox domain.Sandbox) error {
	s.sandbox = &sandbox
	return nil
}
func (s *store) DropSandbox(context.Context) (bool, error) {
	held := s.sandbox != nil
	s.sandbox = nil
	return held, nil
}

func (s *store) Latest(context.Context) (domain.Revision, bool, error) {
	if len(s.revisions) == 0 {
		return domain.Revision{}, false, nil
	}
	return s.revisions[len(s.revisions)-1], true, nil
}

func (s *store) Find(_ context.Context, version int) (domain.Revision, error) {
	for _, r := range s.revisions {
		if r.Version == version {
			return r, nil
		}
	}
	return domain.Revision{}, shared.ErrNotFound.WithDetail("config.version_unknown")
}

func (s *store) List(_ context.Context, hub shared.ID, _, _ int) ([]domain.Revision, error) {
	var out []domain.Revision
	for _, r := range s.revisions {
		if r.HubID == hub {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *store) Insert(_ context.Context, r domain.Revision) error {
	s.revisions = append(s.revisions, r)
	return nil
}
func (s *store) InsertProposal(_ context.Context, p domain.Proposal) error {
	s.proposals = append(s.proposals, p)
	return nil
}
func (s *store) FindProposal(_ context.Context, id shared.ID) (domain.Proposal, error) {
	for _, p := range s.proposals {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Proposal{}, shared.ErrNotFound
}
func (s *store) ListProposals(context.Context, domain.ProposalState, int) ([]domain.Proposal, error) {
	return s.proposals, nil
}
func (s *store) DecideProposal(_ context.Context, id shared.ID, state domain.ProposalState, by shared.ID, _ time.Time) (bool, error) {
	for i := range s.proposals {
		if s.proposals[i].ID == id && s.proposals[i].State == domain.ProposalWaiting {
			s.proposals[i].State, s.proposals[i].DecidedBy = state, by
			return true, nil
		}
	}
	return false, nil
}
func (s *store) InsertTemplate(_ context.Context, t domain.Template) error {
	s.templates = append(s.templates, t)
	return nil
}
func (s *store) ListTemplates(context.Context) ([]domain.Template, error) { return s.templates, nil }
func (s *store) FindTemplate(_ context.Context, key string, version int) (domain.Template, error) {
	for _, t := range s.templates {
		if t.Key == key && t.Version == version {
			return t, nil
		}
	}
	return domain.Template{}, domain.ErrTemplateUnknown(key)
}
func (s *store) PutTemplateUse(_ context.Context, use domain.TemplateUse) error {
	for i := range s.uses {
		if s.uses[i].TemplateKey == use.TemplateKey {
			s.uses[i] = use
			return nil
		}
	}
	s.uses = append(s.uses, use)
	return nil
}
func (s *store) TemplateUses(context.Context) ([]domain.TemplateUse, error) { return s.uses, nil }

type settings struct{ entries map[setting.Key]setting.Entry }

func (s *settings) Above(context.Context, string) (setting.Layer, setting.Layer, error) {
	return setting.Layer{Level: setting.LevelInstance}, setting.Layer{Level: setting.LevelPlan}, nil
}
func (s *settings) At(_ context.Context, places []settingrepo.Place, _ string) ([]settingrepo.Stored, error) {
	var out []settingrepo.Stored
	for key, entry := range s.entries {
		out = append(out, settingrepo.Stored{Place: places[0], Key: key, Entry: entry})
	}
	return out, nil
}
func (s *settings) Put(_ context.Context, _ shared.ID, _ settingrepo.Place, key setting.Key, entry setting.Entry, _ time.Time) error {
	s.entries[key] = entry
	return nil
}
func (s *settings) Remove(context.Context, settingrepo.Place, setting.Key) (bool, error) {
	return false, nil
}

type allow struct{}

func (allow) Authorize(context.Context, appshared.ActorContext, access.Request) error { return nil }

type trail struct{ actions []audit.Action }

func (t *trail) Append(_ context.Context, e audit.Entry) error {
	t.actions = append(t.actions, e.Action)
	return nil
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 10, 29, 6, 0, 0, 0, time.UTC) }

type ids struct{ n int }

func (i *ids) NewID() shared.ID {
	i.n++
	return shared.ID("0190a8f0-0000-7000-8000-" + pad(i.n))
}

func pad(n int) string {
	s := "000000000000"
	digits := []byte(s)
	for i := len(digits) - 1; n > 0 && i >= 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits)
}

type fixture struct {
	desk  Desk
	world *world
	store *store
	trail *trail
}

func newFixture() fixture {
	w := &world{
		types: map[string]map[string]any{
			"task": {"key": "task", "level": "TASK", "system": true, "position": 0},
		},
		roles: map[string]map[string]any{},
	}
	s := &store{}
	t := &trail{}
	return fixture{
		desk: Desk{
			Catalogue: catalogue{w}, Store: s, Settings: &settings{entries: map[setting.Key]setting.Entry{}},
			Authorizer: allow{}, Audit: t, UnitOfWork: &unitOfWork{w: w, store: s}, Clock: fixedClock{}, IDs: &ids{},
		},
		world: w, store: s, trail: t,
	}
}

var (
	person = appshared.ActorContext{Kind: appshared.ActorUser, TenantID: "0190a8f0-0000-7000-8000-00000000aaaa", AccountID: "0190a8f0-0000-7000-8000-00000000bbbb"}
	agent  = appshared.ActorContext{Kind: appshared.ActorAIAgent, TenantID: person.TenantID, AccountID: person.AccountID}
)

func invoke(t *testing.T, d usecase.Descriptor, actor appshared.ActorContext, in usecase.Input) usecase.Output {
	t.Helper()
	out, err := d.Handler.Invoke(t.Context(), actor, in)
	if err != nil {
		var e *shared.Error
		if errors.As(err, &e) {
			t.Fatalf("%s: %v %v", d.Name, err, e.Fields)
		}
		t.Fatalf("%s: %v", d.Name, err)
	}
	return out
}

func incident() map[string]any {
	return map[string]any{"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}}}
}

func TestTheExportNamesEveryBlockByItsStableKey(t *testing.T) {
	f := newFixture()
	out := invoke(t, ExportConfig{f.desk}.Descriptor(), person, nil)
	document := out["document"].(map[string]any)
	if _, held := document["types"].(map[string]any)["task"]; !held {
		t.Errorf("the system type is not under its key: %v", document["types"])
	}
	if document["modules"].(map[string]any)["workflows"] == nil {
		t.Error("a module is not under its lower-case key")
	}
	if document["settings"].(map[string]any)["config.agent_apply"].(map[string]any)["value"] != "APPROVAL" {
		t.Error("the agents' policy is not APPROVAL by default")
	}
	if withheld := out["withheld"].([]string); len(withheld) == 0 {
		t.Error("a section whose module is off is not listed as withheld")
	}
	again := invoke(t, ExportConfig{f.desk}.Descriptor(), person, nil)
	if out["hash"] != again["hash"] {
		t.Error("two exports of one configuration hash apart")
	}
}

func TestAPlanShowsTheChangesAndRefusalsAndWritesNothing(t *testing.T) {
	f := newFixture()
	in := usecase.Input{"document": map[string]any{
		"types": map[string]any{
			"incident": map[string]any{"level": "TASK", "name": "Incident"},
			"task":     nil,
		},
		"nonsense": map[string]any{},
	}}
	out := invoke(t, PlanConfig{f.desk}.Descriptor(), person, in)
	codes := map[string]string{}
	for _, r := range out["refusals"].([]usecase.Output) {
		codes[r["path"].(string)] = r["code"].(string)
	}
	if codes["/nonsense"] != "config.section_unknown" {
		t.Errorf("refusals %v", codes)
	}
	in["document"] = map[string]any{"types": map[string]any{"task": nil, "incident": map[string]any{"level": "TASK", "name": "Incident"}}}
	out = invoke(t, PlanConfig{f.desk}.Descriptor(), person, in)
	codes = map[string]string{}
	for _, r := range out["refusals"].([]usecase.Output) {
		codes[r["path"].(string)] = r["code"].(string)
	}
	if codes["/types/task"] != "config.not_removable" || out["plan_hash"] != nil {
		t.Errorf("a shipped type's removal was not refused: %v, hash %v", codes, out["plan_hash"])
	}
	out = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident()})
	if out["plan_hash"] == nil || len(out["changes"].([]usecase.Output)) != 1 {
		t.Fatalf("plan %v", out)
	}
	if len(f.world.calls) != 0 || len(f.store.revisions) != 0 {
		t.Errorf("a plan wrote: %v", f.world.calls)
	}
	fixed := invoke(t, ValidateConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{
		"types": map[string]any{"task": map[string]any{"level": "ACTIVITY"}},
	}})
	if fixed["valid"] != false || fixed["refusals"].([]usecase.Output)[0]["code"] != "config.attribute_fixed" {
		t.Errorf("a fixed attribute's change was not refused: %v", fixed)
	}
	invalid := invoke(t, ValidateConfig{f.desk}.Descriptor(), person, usecase.Input{"document": map[string]any{
		"types": map[string]any{"risk": map[string]any{"level": "PROJECT", "name": "Risk"}},
	}})
	if invalid["valid"] != false {
		t.Errorf("a value outside the operation's declaration was accepted: %v", invalid)
	}
}

func TestApplyRunsThePlanByItsHashAndRefusesAConfigurationChangedSince(t *testing.T) {
	f := newFixture()
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident()})
	in := usecase.Input{"document": incident(), "plan_hash": plan["plan_hash"], "base_hash": plan["base_hash"]}
	out := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, in)
	if out["state"] != "APPLIED" || f.world.types["incident"] == nil {
		t.Fatalf("not applied: %v", out)
	}
	if len(f.store.revisions) != 1 || f.store.revisions[0].Counts.Added != 1 || f.store.revisions[0].ActorKind != appshared.ActorUser {
		t.Errorf("version %+v", f.store.revisions)
	}
	if f.trail.actions[len(f.trail.actions)-1] != AppliedAction {
		t.Errorf("trail %v", f.trail.actions)
	}
	// The same plan again: the configuration it was planned on is gone.
	_, err := ApplyConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"document":  map[string]any{"types": map[string]any{"incident": map[string]any{"name": "Störung"}}},
		"plan_hash": plan["plan_hash"],
	})
	if !isDetail(err, "config.version_conflict") {
		t.Errorf("a stale plan applied: %v", err)
	}
}

func TestApplyIsAllOrNothing(t *testing.T) {
	f := newFixture()
	document := map[string]any{
		"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}},
		"roles": map[string]any{"desk_agent": map[string]any{"name": "Desk agent", "permissions": []any{"READ"}}},
	}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})
	f.world.failOn = "CreateEntryType"
	_, err := ApplyConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"document": document, "plan_hash": plan["plan_hash"], "acknowledge": true,
	})
	if err == nil {
		t.Fatal("the failing operation did not refuse the apply")
	}
	var e *shared.Error
	if !errors.As(err, &e) || e.Params["config_path"] != "types.incident" {
		t.Errorf("the refusal does not name the key path: %v", err)
	}
	if len(f.world.roles) != 0 || len(f.store.revisions) != 0 {
		t.Errorf("a part of the plan stayed: roles %v, versions %d", f.world.roles, len(f.store.revisions))
	}
}

func TestAPlanChangingPermissionsNeedsItsFlagAcknowledged(t *testing.T) {
	f := newFixture()
	document := map[string]any{"roles": map[string]any{"desk_agent": map[string]any{"name": "Desk agent", "permissions": []any{"READ"}}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})
	if plan["flags"].(usecase.Output)["changes_permissions"] != true {
		t.Fatalf("flags %v", plan["flags"])
	}
	_, err := ApplyConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"document": document, "plan_hash": plan["plan_hash"]})
	if !isDetail(err, "config.acknowledge_required") {
		t.Errorf("applied without the flag acknowledged: %v", err)
	}
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document, "plan_hash": plan["plan_hash"], "acknowledge": true})
	if f.world.roles["desk_agent"] == nil {
		t.Error("not applied with the flag acknowledged")
	}
}

// D-16: an agent's apply waits for a person by default; the person applies it; the version names
// both.
func TestAnAgentsPlanWaitsForAPerson(t *testing.T) {
	f := newFixture()
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), agent, usecase.Input{"document": incident()})
	out := invoke(t, ApplyConfig{f.desk}.Descriptor(), agent, usecase.Input{"document": incident(), "plan_hash": plan["plan_hash"]})
	if out["state"] != "PROPOSED" || f.world.types["incident"] != nil {
		t.Fatalf("an agent's apply did not wait: %v", out)
	}
	id := f.store.proposals[0].ID
	if _, err := (DecideConfigProposal{f.desk}).Descriptor().Handler.Invoke(t.Context(), agent, (usecase.Input{"proposal_id": id.String(), "decision": "APPLY"})); !isDetail(err, "config.agent_cannot_decide") {
		t.Errorf("an agent decided its own proposal: %v", err)
	}
	decided := invoke(t, DecideConfigProposal{f.desk}.Descriptor(), person, usecase.Input{"proposal_id": id.String(), "decision": "APPLY"})
	if decided["state"] != "APPLIED" || f.world.types["incident"] == nil {
		t.Fatalf("not applied: %v", decided)
	}
	if revision := f.store.revisions[0]; revision.ProposedBy != agent.AccountID || revision.ActorKind != appshared.ActorUser {
		t.Errorf("the version does not name proposer and approver: %+v", revision)
	}
	if _, err := (DecideConfigProposal{f.desk}).Descriptor().Handler.Invoke(t.Context(), person, (usecase.Input{"proposal_id": id.String(), "decision": "DECLINE"})); !isDetail(err, "config.proposal_decided") {
		t.Errorf("a decided proposal was decided again: %v", err)
	}

	invoke(t, SetConfigPolicy{f.desk}.Descriptor(), person, usecase.Input{"agent_apply": "NEVER"})
	other := map[string]any{"types": map[string]any{"problem": map[string]any{"level": "TASK", "name": "Problem"}}}
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), agent, usecase.Input{"document": other})
	if _, err := (ApplyConfig{f.desk}).Descriptor().Handler.Invoke(t.Context(), agent, (usecase.Input{"document": other, "plan_hash": plan["plan_hash"]})); !isDetail(err, "config.agent_apply_never") {
		t.Errorf("an agent applied under NEVER: %v", err)
	}
}

func TestARollbackPlansAnEarlierVersionWhole(t *testing.T) {
	f := newFixture()
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"types": map[string]any{}}, "plan_hash": invoke(t, PlanConfig{f.desk}.Descriptor(), person,
			usecase.Input{"document": map[string]any{"types": map[string]any{}}})["plan_hash"],
	})
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident()})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident(), "plan_hash": plan["plan_hash"]})
	back := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"to_version": 1})
	changes := back["changes"].([]usecase.Output)
	if len(changes) != 1 || changes[0]["op"] != "REMOVE" || changes[0]["path"] != "types.incident" {
		t.Fatalf("rollback plan %v", changes)
	}
	out := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"to_version": 1, "plan_hash": back["plan_hash"], "acknowledge": true})
	if f.world.types["incident"] != nil || out["version"].(usecase.Output)["rolled_back_to"] != 1 {
		t.Errorf("not rolled back: %v", out)
	}
}

// D-17: a template is taken with its provenance and upgraded by a three-way comparison that never
// overwrites what the workspace changed itself without showing it.
func TestATemplateIsTakenAndUpgradedThreeWay(t *testing.T) {
	f := newFixture()
	v1 := map[string]any{"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident", "icon": "alert"}}}
	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "service_desk", "name": "Service desk", "document": v1})
	take := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "service_desk"})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": take["document"], "plan_hash": take["plan_hash"], "template": take["template"],
	})
	if f.store.revisions[0].Template != "service_desk@1" || len(f.store.uses) != 1 {
		t.Fatalf("the provenance was not recorded: %+v", f.store.revisions)
	}

	// The workspace renames the type; the template's next version changes its name and its icon.
	rename := map[string]any{"types": map[string]any{"incident": map[string]any{"name": "Störung"}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": rename})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": rename, "plan_hash": plan["plan_hash"]})
	v2 := map[string]any{"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident (P1-P4)", "icon": "siren"}}}
	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "service_desk", "name": "Service desk", "document": v2})

	listed := invoke(t, ListConfigTemplates{f.desk}.Descriptor(), person, nil)["data"].([]usecase.Output)
	if last := listed[len(listed)-1]; last["key"] != "service_desk" || last["update_available"] != true {
		t.Errorf("no update offered: %v", listed)
	}
	upgrade := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "service_desk"})
	states := map[string]any{}
	for _, change := range upgrade["upgrade"].([]usecase.Output) {
		states[change["path"].(string)] = change["state"]
	}
	if states["types.incident.icon"] != "TAKE" || states["types.incident.name"] != "CONFLICT" {
		t.Fatalf("upgrade %v", states)
	}
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": upgrade["document"], "plan_hash": upgrade["plan_hash"], "template": upgrade["template"],
	})
	if f.world.types["incident"]["icon"] != "siren" || f.world.types["incident"]["name"] != "Störung" {
		t.Errorf("the upgrade overwrote the workspace's own name or left the take: %v", f.world.types["incident"])
	}
	if f.store.uses[0].Version != 2 {
		t.Errorf("the base did not move to the version taken: %+v", f.store.uses)
	}
}

func TestATemplateThatDoesNotPassIsNotPublished(t *testing.T) {
	f := newFixture()
	_, err := PublishConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"key": "broken", "name": "Broken", "document": map[string]any{"types": map[string]any{"x": map[string]any{"level": "PROJECT", "name": "X"}}},
	})
	if err == nil || len(f.store.templates) != 0 {
		t.Errorf("a broken template was published: %v", err)
	}
}

func TestTheSchemaIsTakenFromTheOperations(t *testing.T) {
	f := newFixture()
	schema := f.desk.Schema()
	types := schema["properties"].(map[string]any)["types"].(map[string]any)
	block := types["additionalProperties"].(map[string]any)["oneOf"].([]any)[0].(map[string]any)
	level := block["properties"].(map[string]any)["level"].(map[string]any)
	if len(level["enum"].([]string)) != 3 || level["type"] != "string" {
		t.Errorf("the level's schema is not the operation's declaration: %v", level)
	}
}

func isDetail(err error, code string) bool {
	var e *shared.Error
	return errors.As(err, &e) && e.DetailCode == code
}
