// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/plan"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Plans: a provider's groups of workspaces, between the installation and the workspace
// (multi-tenancy.md §4.2, ADR-0086). The control plane holds them; what a plan does to a workspace
// is the resolvers' to apply.

const (
	ListPlansName  = "ListPlans"
	ReadPlanName   = "ReadPlan"
	CreatePlanName = "CreatePlan"
	ChangePlanName = "ChangePlan"
	RemovePlanName = "RemovePlan"

	planTarget = "plan"

	planListedAction  audit.Action = "plan.listed"
	planReadAction    audit.Action = "plan.read"
	planCreatedAction audit.Action = "plan.created"
	planChangedAction audit.Action = "plan.changed"
	planRemovedAction audit.Action = "plan.removed"
)

// PlanWriter is what the plan use cases share: the instance writer for the register, the level and
// the journal, the plans, and the mode - plans exist in multi mode only.
type PlanWriter struct {
	Instance InstanceWriter
	Plans    adminrepo.Plans
	Tenancy  env.TenancyMode
}

func (w PlanWriter) authorize(ctx context.Context, actor appshared.ActorContext, scopes ...string) error {
	if err := w.Instance.authorize(ctx, actor, scopes...); err != nil {
		return err
	}
	return requireMulti(w.Tenancy)
}

// PlanCommand is what a create or a change carries.
type PlanCommand struct {
	Name        string
	DisplayName string
	Values      plan.Values
}

// PlanView is a plan as the control plane answers it: the plan and how many workspaces are on it.
type PlanView struct {
	Plan       plan.Plan
	Workspaces int64
}

// ListPlans answers the plans not removed.
type ListPlans struct{ Writer PlanWriter }

// Execute lists them.
func (h ListPlans) Execute(ctx context.Context, actor appshared.ActorContext) ([]PlanView, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return nil, err
	}
	var views []PlanView
	err := h.Writer.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			plans, err := h.Writer.Plans.List(ctx)
			if err != nil {
				return err
			}
			counts, err := h.Writer.Plans.Workspaces(ctx)
			if err != nil {
				return err
			}
			views = make([]PlanView, 0, len(plans))
			for _, held := range plans {
				views = append(views, PlanView{Plan: held, Workspaces: counts[held.Name]})
			}
			return nil
		})
	return views, err
}

// ReadPlan answers one plan.
type ReadPlan struct{ Writer PlanWriter }

// Execute reads it.
func (h ReadPlan) Execute(ctx context.Context, actor appshared.ActorContext, name string) (PlanView, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return PlanView{}, err
	}
	var view PlanView
	err := h.Writer.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := h.Writer.view(ctx, name)
			view = read
			return err
		})
	return view, err
}

func (w PlanWriter) view(ctx context.Context, name string) (PlanView, error) {
	found, err := w.Plans.Find(ctx, name)
	if err != nil {
		return PlanView{}, err
	}
	counts, err := w.Plans.Workspaces(ctx)
	if err != nil {
		return PlanView{}, err
	}
	return PlanView{Plan: found, Workspaces: counts[found.Name]}, nil
}

// CreatePlan writes a new plan.
type CreatePlan struct{ Writer PlanWriter }

// Execute creates it.
func (h CreatePlan) Execute(ctx context.Context, actor appshared.ActorContext, cmd PlanCommand) (PlanView, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return PlanView{}, err
	}
	return h.Writer.create(ctx, actor, cmd, false)
}

func (w PlanWriter) create(
	ctx context.Context, actor appshared.ActorContext, cmd PlanCommand, fromFile bool,
) (PlanView, error) {
	if err := plan.ValidName(cmd.Name); err != nil {
		return PlanView{}, err
	}
	if err := plan.ValidDisplayName(cmd.DisplayName); err != nil {
		return PlanView{}, err
	}
	var view PlanView
	err := w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		if err := w.fitsUnderTheInstallation(ctx, cmd.Values, fromFile); err != nil {
			return err
		}
		now := w.Instance.Clock.Now()
		created := plan.Plan{
			ID: w.Instance.IDs.NewID(), Name: cmd.Name, DisplayName: cmd.DisplayName, Version: 1,
			Values: cmd.Values, CreatedAt: now, UpdatedAt: now,
		}
		if err := w.Plans.Insert(ctx, created, actor.AccountID); err != nil {
			return err
		}
		if err := w.record(ctx, actor, planCreatedAction, created.Name, now); err != nil {
			return err
		}
		read, err := w.view(ctx, created.Name)
		view = read
		return err
	})
	return view, err
}

// ChangePlan replaces a plan's display name and values.
type ChangePlan struct{ Writer PlanWriter }

// Execute changes it. With `expected`, a plan somebody saved since it was read is refused.
func (h ChangePlan) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd PlanCommand, expected *int,
) (PlanView, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return PlanView{}, err
	}
	return h.Writer.change(ctx, actor, cmd, expected, false)
}

func (w PlanWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd PlanCommand, expected *int, fromFile bool,
) (PlanView, error) {
	if err := plan.ValidDisplayName(cmd.DisplayName); err != nil {
		return PlanView{}, err
	}
	var view PlanView
	err := w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		if err := w.fitsUnderTheInstallation(ctx, cmd.Values, fromFile); err != nil {
			return err
		}
		found, err := w.Plans.Find(ctx, cmd.Name)
		if err != nil {
			return err
		}
		found.DisplayName, found.Values = cmd.DisplayName, cmd.Values
		now := w.Instance.Clock.Now()
		written, err := w.Plans.Replace(ctx, found, expected, actor.AccountID, now)
		if err != nil {
			return err
		}
		if !written {
			// Either somebody saved first, or somebody removed the plan meanwhile: the second is
			// answered as the plan being gone, not as a version to read again.
			if _, err := w.Plans.Find(ctx, cmd.Name); err != nil {
				return err
			}
			return shared.ErrVersionConflict.WithDetail("admin.plan_version_moved")
		}
		if err := w.record(ctx, actor, planChangedAction, found.Name, now); err != nil {
			return err
		}
		read, err := w.view(ctx, found.Name)
		view = read
		return err
	})
	return view, err
}

// RemovePlan ends a plan no workspace is on. Its short name stays taken.
type RemovePlan struct{ Writer PlanWriter }

// Execute removes it.
func (h RemovePlan) Execute(ctx context.Context, actor appshared.ActorContext, name string) error {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return err
	}
	return h.Writer.remove(ctx, actor, name, false)
}

func (w PlanWriter) remove(ctx context.Context, actor appshared.ActorContext, name string, fromFile bool) error {
	return w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		if err := w.notEnforced(ctx, fromFile); err != nil {
			return err
		}
		found, err := w.Plans.Find(ctx, name)
		if err != nil {
			return err
		}
		now := w.Instance.Clock.Now()
		outcome, err := w.Plans.Remove(ctx, found.ID, actor.AccountID, now)
		if err != nil {
			return err
		}
		switch outcome {
		case adminrepo.PlanInUse:
			return shared.ErrConflict.WithDetail("admin.plan_in_use")
		case adminrepo.PlanNotFound:
			return shared.ErrNotFound.WithDetail("admin.plan_not_found")
		}
		return w.record(ctx, actor, planRemovedAction, found.Name, now)
	})
}

// fitsUnderTheInstallation is the one check every door's write passes: the file, the API and the
// screen meet the same refusals (UC-INS-13 check 7).
func (w PlanWriter) fitsUnderTheInstallation(ctx context.Context, values plan.Values, fromFile bool) error {
	level, err := w.Instance.Settings.Read(ctx)
	if err != nil {
		return err
	}
	if level.IsEnforcedFromFile && !fromFile {
		return enforcedFromFile(level)
	}
	return values.FitsUnder(aboveOf(level))
}

func (w PlanWriter) notEnforced(ctx context.Context, fromFile bool) error {
	level, err := w.Instance.Settings.Read(ctx)
	if err != nil {
		return err
	}
	if level.IsEnforcedFromFile && !fromFile {
		return enforcedFromFile(level)
	}
	return nil
}

// enforcedFromFile is the level's own refusal: in `enforce` mode the file is the source of the
// plans as of every other value, and a save through the API would be undone at the next start.
func enforcedFromFile(level identityrepo.InstanceLevel) error {
	return shared.ErrConflict.
		WithDetail("admin.instance_enforced_from_file").
		WithParams(map[string]string{"source": level.Source})
}

func aboveOf(level identityrepo.InstanceLevel) plan.Above {
	return plan.Above{Policy: level.Policy, Legal: level.Legal, Quotas: level.Quotas, Ai: level.Ai}
}

// record writes the journal entry. The short name is an identifier; the display name is the
// operator's text and stays out of a place nothing ever deletes from (rule 10).
func (w PlanWriter) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, name string, at time.Time,
) error {
	return w.Instance.Journal.Record(ctx, adminrepo.InstanceEvent{
		ID: w.Instance.IDs.NewID(), OccurredAt: at, Action: string(action),
		ActorLabel: actor.AccountName, Details: map[string]any{"plan": name},
	})
}

// planCommandFrom reads a plan from a document: the REST body, a tool call, or the instance file.
// The areas are read by the installation level's own reading, so a key means the same at both
// levels; localisation is refused rather than ignored - a plan sets no language
// (multi-tenancy.md §4.2) - and `left_out` names the features the plan leaves out.
func planCommandFrom(in usecase.Input) (PlanCommand, error) {
	if sent, isObject := in[areaLocalisation].(map[string]any); isObject && len(sent) > 0 {
		names := make([]string, 0, len(sent))
		for name := range sent {
			names = append(names, name)
		}
		slices.Sort(names)
		return PlanCommand{}, shared.ErrValidation.WithDetail("admin.plan_localisation").
			WithParams(map[string]string{"setting": areaLocalisation + "." + names[0]}).
			WithFields(shared.FieldError{Path: "/" + areaLocalisation, Code: "admin.plan_localisation"})
	}
	level, err := instanceLevelFrom(in)
	if err != nil {
		return PlanCommand{}, err
	}
	leftOut, err := in.StringList("left_out")
	if err != nil {
		return PlanCommand{}, err
	}
	// In the domain's order, so that two documents naming the same features read the same; a
	// name outside the set is kept for the check to refuse.
	features := make([]plan.Feature, 0, len(leftOut))
	for _, known := range plan.Features() {
		if slices.Contains(leftOut, string(known)) {
			features = append(features, known)
		}
	}
	for _, name := range leftOut {
		if feature := plan.Feature(name); !feature.Valid() && !slices.Contains(features, feature) {
			features = append(features, feature)
		}
	}
	return PlanCommand{
		Name: in.String("name"), DisplayName: in.String("display_name"),
		Values: plan.Values{
			Policy: level.Policy, Legal: level.Legal, Quotas: level.Quotas, Ai: level.Ai, LeftOut: features,
		},
	}, nil
}

// planOutput answers a plan in the installation level's own shape, so a screen draws both levels
// with one reading.
func planOutput(view PlanView) usecase.Output {
	held := view.Plan
	// A plan's own document describes no lock on the account link: nothing below a plan sets one.
	out := levelOutput(identityrepo.InstanceLevel{
		Policy: held.Values.Policy, Legal: held.Values.Legal, Quotas: held.Values.Quotas, Ai: held.Values.Ai,
	}, false)
	for _, levelOnly := range []string{
		areaLocalisation, "source", "is_enforced_from_file", "blocklist_file", "metering",
		"reserved_short_names", "owners_change_address", "address_change_cooldown_days",
		"owners_own_domains",
	} {
		delete(out, levelOnly)
	}
	leftOut := make([]any, 0, len(held.Values.LeftOut))
	for _, feature := range held.Values.LeftOut {
		leftOut = append(leftOut, string(feature))
	}
	out["name"] = held.Name
	out["display_name"] = held.DisplayName
	out["version"] = held.Version
	out["workspaces"] = view.Workspaces
	out["created_at"] = held.CreatedAt
	out["updated_at"] = held.UpdatedAt
	out["left_out"] = leftOut
	return out
}

// The values every write carries, declared once for the three descriptors that take them.
var planValueFields = []usecase.Field{
	{Name: "display_name", Kind: usecase.KindString, Required: true,
		Description: "The operator's own name for the plan, 1 to 200 characters."},
	{Name: "sign_in", Kind: usecase.KindObject,
		Description: "Sign-in switches by name, each `{value, locked}`, as the installation's level takes them."},
	{Name: "legal", Kind: usecase.KindObject,
		Description: "Legal links by name, each `{value, locked}`."},
	{Name: "quotas", Kind: usecase.KindObject,
		Description: "One ceiling per quota, each `{value, locked}`; 0 is unlimited."},
	{Name: "ai", Kind: usecase.KindObject,
		Description: "`sources`, `min_jurisdiction` and `person_opt_out`, each `{value, locked}`."},
	{Name: "left_out", Kind: usecase.KindList,
		Description: "The features the plan leaves out: AUTOMATION_RULES, WEBHOOKS, OWN_SIGN_IN_PROVIDERS."},
}

var planNameField = usecase.Field{
	Name: "name", Kind: usecase.KindString, Required: true,
	Description: "The plan's short name, which never changes and is never reused.",
}

// Descriptor is the catalogue entry.
func (h ListPlans) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListPlansName,
		Summary: "Lists the installation's plans not removed, oldest first, each with what it " +
			"decides in the installation level's shape, the features it leaves out and how many " +
			"workspaces are on it. Multi mode only.",
		TokenScope: adminTenantsScope,
		ReadOnly:   true,
		Audit:      usecase.AuditDeclaration{Action: planListedAction, TargetType: planTarget, Severity: audit.SeverityInfo},
		Activity:   usecase.ActivityDeclaration{Exempt: "a read of the control plane touches no item."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			views, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			rows := make([]usecase.Output, 0, len(views))
			for _, view := range views {
				rows = append(rows, planOutput(view))
			}
			return usecase.Output{"data": rows}, nil
		}),
	}
}

// Descriptor is the catalogue entry.
func (h ReadPlan) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ReadPlanName,
		Summary:    "Reads one plan by its short name. Multi mode only.",
		TokenScope: adminTenantsScope,
		ReadOnly:   true,
		Input:      []usecase.Field{planNameField},
		Audit:      usecase.AuditDeclaration{Action: planReadAction, TargetType: planTarget, Severity: audit.SeverityInfo},
		Activity:   usecase.ActivityDeclaration{Exempt: "a read of the control plane touches no item."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			view, err := h.Execute(ctx, actor, in.String("name"))
			if err != nil {
				return nil, err
			}
			return planOutput(view), nil
		}),
	}
}

// Descriptor is the catalogue entry.
func (h CreatePlan) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreatePlanName,
		Summary: "Creates a plan: a short name that never changes and is never reused, and what " +
			"the plan decides - nothing under an installation lock, nothing looser than the " +
			"installation leaves in force. Multi mode only.",
		SideEffects: "Writes the plan and one journal entry naming its short name.",
		TokenScope:  adminTenantsScope,
		Input:       append([]usecase.Field{planNameField}, planValueFields...),
		Audit: usecase.AuditDeclaration{
			Action: planCreatedAction, TargetType: planTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "the installation's configuration is not an entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			cmd, err := planCommandFrom(in)
			if err != nil {
				return nil, err
			}
			view, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return planOutput(view), nil
		}),
	}
}

// Descriptor is the catalogue entry.
func (h ChangePlan) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ChangePlanName,
		Summary: "Replaces a plan's display name and values whole: a value the body does not name " +
			"is no longer decided by the plan. With `expected_version`, a save made on a plan " +
			"somebody else changed since it was read is refused. Applies to every workspace on the " +
			"plan at its next request. Multi mode only.",
		SideEffects: "Writes the plan and one journal entry naming its short name.",
		TokenScope:  adminTenantsScope,
		Input: append([]usecase.Field{planNameField,
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The plan's version as last read; a stale one is refused with version_conflict."},
		}, planValueFields...),
		Audit: usecase.AuditDeclaration{
			Action: planChangedAction, TargetType: planTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "the installation's configuration is not an entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			cmd, err := planCommandFrom(in)
			if err != nil {
				return nil, err
			}
			var expected *int
			if in.Present("expected_version") {
				version := in.Int("expected_version")
				expected = &version
			}
			view, err := h.Execute(ctx, actor, cmd, expected)
			if err != nil {
				return nil, err
			}
			return planOutput(view), nil
		}),
	}
}

// Descriptor is the catalogue entry.
func (h RemovePlan) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemovePlanName,
		Summary: "Removes a plan no workspace is on; refused while one is. Its short name stays " +
			"taken. Multi mode only.",
		SideEffects: "Marks the plan removed and writes one journal entry naming its short name.",
		TokenScope:  adminTenantsScope,
		Input:       []usecase.Field{planNameField},
		Audit: usecase.AuditDeclaration{
			Action: planRemovedAction, TargetType: planTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "the installation's configuration is not an entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Execute(ctx, actor, in.String("name")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}
}

// PlanFileOutcome is what writing the file's plans came to: nothing in single mode, where plans do
// not exist, and the plans an enforcing file no longer names that stayed because workspaces are on
// them.
type PlanFileOutcome struct {
	Skipped bool
	Kept    []string
}

// ApplyFile writes the instance file's plans, the third door (ADR-0070 §5) for plans: `seed` writes
// them where no plan exists yet; `enforce` writes them at every start and removes the plans the file
// no longer names - except one a workspace is on, which no door can remove and which is answered as
// kept. It goes through the same reading and the same checks the API does, as the file, so a plan a
// file may set is one the API accepts (UC-INS-13 check 7).
func (w PlanWriter) ApplyFile(
	ctx context.Context, documents []map[string]any, enforce bool, source string,
) (PlanFileOutcome, error) {
	if w.Tenancy != env.TenancyMulti {
		return PlanFileOutcome{Skipped: true}, nil
	}
	var existing []plan.Plan
	var ever bool
	if err := w.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		read, err := w.Plans.List(ctx)
		if err != nil {
			return err
		}
		existing = read
		ever, err = w.Plans.Ever(ctx)
		return err
	}); err != nil {
		return PlanFileOutcome{}, err
	}
	// Seeded once: a plan ever created, removed ones included, means the file wrote its plans
	// before - and a plan an operator removed since is not written again.
	if !enforce && ever {
		return PlanFileOutcome{}, nil
	}
	held := map[string]plan.Plan{}
	for _, standing := range existing {
		held[standing.Name] = standing
	}

	file := appshared.ActorContext{AccountName: source}
	named := map[string]bool{}
	for _, document := range documents {
		cmd, err := planCommandFrom(usecase.Input(document))
		if err != nil {
			return PlanFileOutcome{}, err
		}
		named[cmd.Name] = true
		if standing, isHeld := held[cmd.Name]; isHeld {
			// Unchanged is left alone: an enforcing file is read at every start, and a write per
			// start would be a journal entry and a new version per restart for nothing.
			if samePlan(standing, cmd) {
				continue
			}
			_, err = w.change(ctx, file, cmd, nil, true)
		} else {
			_, err = w.create(ctx, file, cmd, true)
		}
		if err != nil {
			return PlanFileOutcome{}, err
		}
	}

	var outcome PlanFileOutcome
	if !enforce {
		return outcome, nil
	}
	for _, standing := range existing {
		if named[standing.Name] {
			continue
		}
		err := w.remove(ctx, file, standing.Name, true)
		var refusal *shared.Error
		switch {
		case err == nil:
		case errors.As(err, &refusal) && refusal.DetailCode == "admin.plan_in_use":
			outcome.Kept = append(outcome.Kept, standing.Name)
		default:
			return PlanFileOutcome{}, err
		}
	}
	return outcome, nil
}

// samePlan reports whether a command would leave a plan as it stands: the same display name, and
// the same values as the control plane answers them.
func samePlan(standing plan.Plan, cmd PlanCommand) bool {
	if standing.DisplayName != cmd.DisplayName {
		return false
	}
	return reflect.DeepEqual(
		planOutput(PlanView{Plan: plan.Plan{Values: standing.Values}}),
		planOutput(PlanView{Plan: plan.Plan{Values: cmd.Values}}),
	)
}
