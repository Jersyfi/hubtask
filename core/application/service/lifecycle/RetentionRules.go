// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateRetentionPolicyName  = "CreateRetentionPolicy"
	ListRetentionPoliciesName  = "ListRetentionPolicies"
	PreviewRetentionPolicyName = "PreviewRetentionPolicy"

	// The two scopes. Reading which rules are in force tells somebody what this workspace deletes
	// and when, which is worth a scope of its own; writing one decides it.
	retentionRead   = "retention:read"
	retentionManage = "retention:manage"

	policyTarget = "retention_policy"

	// RuleChangedAction is the entry data-retention.md §4.4 asks for and more besides. A warning
	// rather than an info: a rule is a standing instruction to delete somebody's work, and "who
	// decided that things go after ninety days" is a question with an answer.
	RuleChangedAction audit.Action = "lifecycle.rule_changed"
)

// BroadFirstRunShare is §5's five per cent: "A new rule always starts in NOTIFY_ONLY mode if its
// first run would affect more than 5% of the holdings - with a clear notice rather than a silent
// mass deletion."
const BroadFirstRunShare = 0.05

// Rules is what the three rule use cases share.
type Rules struct {
	Rules repository.Rules
	// Policies is the old table, read for one thing only: the upper bound an operator has set for
	// a kind (§4.4). The periods in it are carried into rules by the sweep; the bounds stay where
	// an operator writes them.
	Policies   repository.Policies
	Marking    repository.Marking
	Holds      repository.LegalHolds
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Conditions compiles a rule's expression when it is written (G-06). A port, so this layer
	// never learns which engine evaluates one - and the same port the automation rules use, which is
	// what makes "the same expression language" one statement rather than two.
	Conditions expression.Compiler
}

// CreateRetentionPolicy writes down what this workspace deletes and when.
type CreateRetentionPolicy struct{ Rules Rules }

// ListRetentionPolicies answers which rules exist, and which of them is in force where.
type ListRetentionPolicies struct{ Rules Rules }

// PreviewRetentionPolicy answers what a rule would do, before it does it.
type PreviewRetentionPolicy struct{ Rules Rules }

// CreateRetentionPolicyCommand is the input, typed.
type CreateRetentionPolicyCommand struct {
	Scope          domain.Scope
	DataKind       domain.DataKind
	Condition      string
	RetainDays     int
	Action         domain.Action
	ThenAfterDays  int
	ThenAction     domain.Action
	GraceDays      *int
	Notify         *domain.Notify
	Justification  string
	Enabled        *bool
	ExportTargetID shared.ID
}

// Preview is what a rule would do (data-retention.md §5).
type Preview struct {
	// Matched is how many entries are past the rule's period in its scope. Exact rather than a
	// page: the switch below is about a proportion, and a count that stopped at a batch would
	// under-report exactly the runs it exists to catch.
	Matched int
	// Blocked is what would be kept back and why, from the sample rather than from the whole set:
	// deciding a legal hold against a million rows to draw a preview would cost more than the run.
	Blocked map[string]int
	// ShareOfScope is Matched over what the scope holds, which is what §5's switch reads.
	ShareOfScope float64
	// Samples are a handful of the objects, so that "four thousand entries" is a sentence somebody
	// can check.
	Samples []Sample
	// Broad reports a rule whose first run would touch more than five per cent of the holdings.
	Broad bool
}

// Sample is one object a preview shows.
type Sample struct {
	ID    shared.ID
	Title string
	// EffectiveAt is when the act would fall due for this one: its own anchor plus the period and
	// the grace, rather than one date for the whole set.
	EffectiveAt time.Time
}

// SampleSize is how many objects a preview shows. Enough to recognise what a rule has caught, few
// enough that the answer is a page rather than an export.
const SampleSize = 10

// Execute writes the rule down.
//
// §5's switch is applied here rather than at the first run, and that is what "starts in NOTIFY_ONLY"
// means: the rule that gets stored is the safe one, so an installation that never runs the engine
// still cannot have a broad rule waiting to fire. What a caller gets back says which mode it is in,
// which is the "clear notice" the document asks for.
func (h CreateRetentionPolicy) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateRetentionPolicyCommand,
) (domain.Rule, Preview, error) {
	// The owner's line rather than the administrator's. A retention rule is a standing instruction
	// to destroy work, which is the one thing domain-model.md §3.2 keeps out of an administrator's
	// hands - and it is the same line a backup target and a destructive restore sit on.
	if err := h.Rules.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     RuleChangedAction,
		TokenScope: retentionManage,
		TargetType: policyTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return domain.Rule{}, Preview{}, err
	}

	now := h.Rules.Clock.Now()
	ceiling, err := h.Rules.ceilingFor(ctx, actor, cmd.DataKind)
	if err != nil {
		return domain.Rule{}, Preview{}, err
	}

	if err := h.Rules.checkCondition(cmd.Condition); err != nil {
		return domain.Rule{}, Preview{}, err
	}

	rule, err := domain.NewRule(domain.NewRuleInput{
		ID: h.Rules.IDs.NewID(), TenantID: actor.TenantID, Scope: cmd.Scope,
		DataKind: cmd.DataKind, Condition: cmd.Condition, RetainDays: cmd.RetainDays,
		Action: cmd.Action, ThenAfterDays: cmd.ThenAfterDays, ThenAction: cmd.ThenAction,
		GraceDays: cmd.GraceDays, Notify: cmd.Notify, Justification: cmd.Justification,
		Enabled: cmd.Enabled, ExportTargetID: cmd.ExportTargetID,
		CreatedBy: actor.AccountID, Now: now, Ceiling: ceiling,
	})
	if err != nil {
		return domain.Rule{}, Preview{}, err
	}

	var preview Preview
	err = h.Rules.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		preview, err = h.Rules.assess(ctx, rule, now)
		if err != nil {
			return err
		}
		if preview.Broad {
			// The notice, and the safety. A rule that would take a twentieth of the workspace on
			// its first run is more likely to be a mistake than an intention, so it is stored as
			// an announcement and the caller is told which mode it is in.
			rule.Action = domain.ActionNotifyOnly
			rule.ThenAction, rule.ThenAfterDays = "", 0
		}
		if err := h.Rules.Rules.Insert(ctx, rule); err != nil {
			return err
		}
		return h.Rules.record(ctx, actor, rule, preview, now)
	})
	if err != nil {
		return domain.Rule{}, Preview{}, err
	}
	return rule, preview, nil
}

// ceilingFor is the upper bound an operator has set for a kind, and zero where they have set none.
//
// Read from the old table rather than from the catalogue, because §4.4 makes it the operator's
// setting rather than a property of the kind - and that column is where an operator has always put
// it. Absent is not an error: most kinds have no maximum, which is the honest default for a period
// the tenant is choosing.
func (r Rules) ceilingFor(
	ctx context.Context, actor appshared.ActorContext, kind domain.DataKind,
) (int, error) {
	if r.Policies == nil {
		return 0, nil
	}
	var ceiling int
	err := r.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		policy, err := r.Policies.Find(ctx, kind)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return nil
			}
			return err
		}
		if policy.MaxDays != nil {
			ceiling = *policy.MaxDays
		}
		return nil
	})
	return ceiling, err
}

// checkCondition compiles a retention rule's expression, or refuses it with the position.
//
// The second consumer of the expression port, and the reason it is a port rather than a helper
// private to the rule engine: two engines read one language, and a check written twice would be two
// dialects. E-07 refused a condition outright because there was nothing that could evaluate one -
// this is what replaced the refusal, and RE's tests flip rather than disappear.
func (r Rules) checkCondition(text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if r.Conditions == nil {
		// Fail closed, for the reason the rule engine's writer does: a build that cannot check a
		// condition cannot promise it means what it says, and a retention rule that matched more
		// than it says deletes more than it says.
		return shared.ErrInternal.WithDetail("lifecycle.expression_engine_unavailable")
	}

	if _, err := r.Conditions.Compile(text, condition.RetentionEnvironment(), expression.Boolean); err != nil {
		finding := shared.FieldError{Path: "/condition", Code: expression.CodeSyntax}
		var coded *shared.Error
		if errors.As(err, &coded) {
			finding.Code, finding.Params = coded.DetailCode, coded.Params
		}
		return shared.ErrValidation.
			WithDetail("lifecycle.condition_invalid").
			WithFields(finding)
	}
	return nil
}

// EffectiveRule is one rule and where it came from, which is what §6's question needs: "which rules
// actually apply here?", including where each came from.
type EffectiveRule struct {
	Rule domain.Rule
	// InForce is whether this is the rule that would act on an object in the container asked
	// about. False for one a narrower rule beats, and for one that is switched off.
	InForce bool
}

// Execute answers the rules, and which of them is in force in a container when one is named.
func (h ListRetentionPolicies) Execute(
	ctx context.Context, actor appshared.ActorContext, containerID shared.ID, effectiveOnly bool,
) ([]EffectiveRule, error) {
	if err := h.Rules.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		// What this workspace deletes and when is the configuration read an auditor most needs:
		// an entry saying a rule removed four hundred objects cannot be judged without it
		// (A-4, G-12). Writing a rule stays where it was - it is a standing instruction to
		// destroy work, and it asks the owner's line.
		Alternative: service.PermissionReadConfiguration,
		Path:        []identity.Scope{identity.TenantScope()},
		Action:      RuleChangedAction,
		TokenScope:  retentionRead,
		TargetType:  policyTarget,
		TargetID:    actor.TenantID,
	}); err != nil {
		return nil, err
	}

	var rules []domain.Rule
	err := h.Rules.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		rules, err = h.Rules.Rules.List(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Without a container the question is "what exists", and nothing is in force anywhere in
	// particular. Answering something else would be answering a question nobody asked.
	answered := make([]EffectiveRule, 0, len(rules))
	for _, rule := range rules {
		entry := EffectiveRule{Rule: rule}
		if !containerID.IsZero() {
			// The container is treated as both, which is what makes one parameter answer for a hub
			// and for a collection: a hub-scoped rule matches when the hub is named, and a
			// collection-scoped one when the collection is. What a caller cannot get this way is
			// "the collection inside that hub", which is what naming the collection is for.
			winner, found := domain.Effective(rules, rule.DataKind, containerID, containerID)
			entry.InForce = found && winner.ID == rule.ID
		}
		if effectiveOnly && !entry.InForce {
			continue
		}
		answered = append(answered, entry)
	}
	return answered, nil
}

// Execute answers what a rule would do, and changes nothing.
func (h PreviewRetentionPolicy) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (Preview, error) {
	if id.IsZero() {
		return Preview{}, shared.ErrValidation.WithDetail(domain.CodeRuleNotFound).
			WithFields(shared.FieldError{Path: "/policy_id", Code: domain.CodeRuleNotFound})
	}
	if err := h.Rules.Authorizer.Authorize(ctx, actor, access.Request{
		Permission:  service.PermissionStructure,
		Alternative: service.PermissionReadConfiguration,
		Path:        []identity.Scope{identity.TenantScope()},
		Action:      RuleChangedAction,
		TokenScope:  retentionRead,
		TargetType:  policyTarget,
		TargetID:    id,
	}); err != nil {
		return Preview{}, err
	}

	var preview Preview
	err := h.Rules.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rule, err := h.Rules.Rules.Find(ctx, id)
		if err != nil {
			return err
		}
		preview, err = h.Rules.assess(ctx, rule, h.Rules.Clock.Now())
		return err
	})
	if err != nil {
		return Preview{}, err
	}
	return preview, nil
}

// assess is what a rule would do, and it is the one calculation both the preview and the
// five-per-cent switch read.
//
// One calculation rather than two, deliberately: RE-7 requires the share a newly activated rule
// reports to match what a preview said, and two implementations of "how much would this touch"
// would eventually disagree by a rounding or by a filter one of them forgot.
func (r Rules) assess(ctx context.Context, rule domain.Rule, now time.Time) (Preview, error) {
	kind, known := domain.FindKind(rule.DataKind)
	if !known {
		return Preview{}, shared.Internalf("lifecycle: a stored rule for the unknown kind %q", rule.DataKind)
	}
	anchor, ok := rule.StageAnchor(kind, false)
	if !ok {
		return Preview{}, shared.Internalf("lifecycle: %q has no anchor", rule.DataKind)
	}

	cutoff := rule.Cutoff(now)
	matched, err := r.Marking.CountDue(ctx, anchor, rule.Scope, cutoff)
	if err != nil {
		return Preview{}, err
	}
	held, err := r.Marking.CountScope(ctx, rule.Scope)
	if err != nil {
		return Preview{}, err
	}

	preview := Preview{Matched: matched, Blocked: map[string]int{}}
	if held > 0 {
		preview.ShareOfScope = float64(matched) / float64(held)
	}
	preview.Broad = preview.ShareOfScope > BroadFirstRunShare

	// The samples, and the block reasons with them. Drawn from a page rather than from the whole
	// set: deciding a legal hold against a million rows to draw a preview would cost more than the
	// run it is previewing, and what a person reads a preview for is "does this look right".
	candidates, err := r.Marking.Due(ctx, anchor, cutoff, SampleSize)
	if err != nil {
		return Preview{}, err
	}
	holds, err := r.Holds.Active(ctx)
	if err != nil {
		return Preview{}, err
	}

	for _, candidate := range candidates {
		if !rule.Applies(candidate.HubID, candidate.CollectionID) {
			continue
		}
		if _, held := holds.Blocking(domain.Target{
			ItemID:          candidate.ID,
			ContainerIDs:    nonZero(candidate.HubID, candidate.CollectionID),
			AncestorItemIDs: work.PathIDs(candidate.Path),
		}); held {
			preview.Blocked[domain.BlockedByLegalHold]++
			continue
		}
		if len(preview.Samples) < SampleSize {
			preview.Samples = append(preview.Samples, Sample{
				ID: candidate.ID, Title: candidate.Title,
				EffectiveAt: rule.EffectiveAt(candidate.AnchoredAt),
			})
		}
	}
	return preview, nil
}

// record writes the audit entry a rule owes.
//
// The justification is in it when there is one, because §4.4 makes exceeding the upper bound
// auditable and the reason is the whole of what an auditor is looking for. Everything else is a
// code or a number - never a container name, never a title (rule 10).
func (r Rules) record(
	ctx context.Context, actor appshared.ActorContext,
	rule domain.Rule, preview Preview, at time.Time,
) error {
	changes := []audit.Change{
		{Field: "data_kind", Classification: audit.Open, To: string(rule.DataKind)},
		{Field: "scope_kind", Classification: audit.Open, To: string(rule.Scope.Kind)},
		{Field: "action", Classification: audit.Open, To: string(rule.Action)},
		{Field: "retain_days", Classification: audit.Open, To: itoa(rule.RetainDays)},
	}
	if rule.Justification != "" {
		// The one free text an audit entry here carries, and it is the operator's own words about
		// their own policy rather than anybody's content.
		changes = append(changes, audit.Change{
			Field: "justification", Classification: audit.Open, To: rule.Justification,
		})
	}
	if preview.Broad {
		changes = append(changes, audit.Change{
			Field: "notify_only_first_run", Classification: audit.Open, To: "true",
		})
	}

	return r.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: at,
		Action: RuleChangedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: policyTarget, TargetID: rule.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// itoa is what an audit change carries a number as: a string, because a change is a pair of
// strings and a number formatted at three call sites is a number formatted three ways.
func itoa(value int) string { return strconv.Itoa(value) }

// Descriptor registers writing a rule down in all three channels.
func (h CreateRetentionPolicy) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateRetentionPolicyName,
		Summary: "Writes down what this workspace deletes and when: which kind of data, how long " +
			"it is kept, what happens then, and optionally what happens after that. A rule " +
			"applies to the whole workspace, to a hub or to a collection, and the narrower one " +
			"wins. A rule whose first run would touch more than five per cent of what the scope " +
			"holds is stored as an announcement instead, and the answer says so - a mass " +
			"deletion should not be something a typo can start.",
		SideEffects: "Writes the rule and an audit entry. Nothing is deleted now: the engine acts " +
			"on its own schedule, and it announces before it acts.",
		TokenScope: retentionManage,
		Input: []usecase.Field{
			{
				Name: "data_kind", Kind: usecase.KindString, Required: true,
				Description: "Which class of data. Hubtask refuses a kind it does not yet " +
					"remove, rather than storing a period nothing enforces.",
			},
			{
				Name: "retain_days", Kind: usecase.KindInt, Required: true,
				Description: "How long it is kept, counted from the moment that kind of data is " +
					"anchored to - a completed entry from its completion, an archived one from " +
					"its archiving.",
			},
			{
				Name: "action", Kind: usecase.KindString, Required: true,
				Enum: []string{
					"ARCHIVE", "TRASH", "ANONYMIZE", "HARD_DELETE",
					"EXPORT_THEN_DELETE", "NOTIFY_ONLY",
				},
				Description: "What happens when the period is up.",
			},
			{
				Name: "scope", Kind: usecase.KindString,
				Enum:        []string{"TENANT", "HUB", "COLLECTION"},
				Description: "How far the rule reaches. The whole workspace unless said otherwise.",
			},
			{
				Name: "scope_id", Kind: usecase.KindID,
				Description: "Which hub or collection, for a rule that is not workspace-wide.",
			},
			{
				Name: "then_after_days", Kind: usecase.KindInt,
				Description: "The second stage of a chain, counted from what the first stage did " +
					"rather than from the original moment.",
			},
			{
				Name: "then_action", Kind: usecase.KindString,
				Enum: []string{"ARCHIVE", "TRASH", "ANONYMIZE", "HARD_DELETE", "EXPORT_THEN_DELETE"},
				Description: "What the second stage does. Only after an action that leaves " +
					"something to act on.",
			},
			{
				Name: "grace_days", Kind: usecase.KindInt,
				Description: "How long between the announcement and the act. Fourteen days " +
					"unless said otherwise, and not available for a kind that is its own grace " +
					"period.",
			},
			{
				Name: "notify_before_days", Kind: usecase.KindInt,
				Description: "How long before the act the warning goes out. Inside the grace " +
					"period, because a warning after the act is a condolence.",
			},
			{
				Name: "notify_recipients", Kind: usecase.KindList,
				Enum:        []string{"ITEM_MEMBERS", "COLLECTION_ADMINS", "TENANT_ADMINS"},
				Description: "Who is warned. An empty list switches the warning off.",
			},
			{
				Name: "justification", Kind: usecase.KindString,
				Description: "Why the period exceeds the upper bound for that kind. Required " +
					"exactly then, and written into the audit trail.",
			},
			{
				Name: "condition", Kind: usecase.KindString,
				Description: "Not available yet: the expression language arrives with the " +
					"automation rules. Narrow the rule with its scope instead.",
			},
			{
				Name: "enabled", Kind: usecase.KindBool,
				Description: "Whether the rule is on. On unless said otherwise.",
			},
			{
				Name: "export_target_id", Kind: usecase.KindID,
				Description: "Where an export-then-delete writes its archive.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleChangedAction, TargetType: policyTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateRetentionPolicy) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd := CreateRetentionPolicyCommand{
		Scope:         domain.Scope{Kind: domain.ScopeTenant},
		DataKind:      domain.DataKind(in.String("data_kind")),
		Condition:     in.String("condition"),
		RetainDays:    in.Int("retain_days"),
		Action:        domain.Action(in.String("action")),
		ThenAfterDays: in.Int("then_after_days"),
		ThenAction:    domain.Action(in.String("then_action")),
		Justification: in.String("justification"),
	}
	if named := in.String("scope"); named != "" {
		cmd.Scope.Kind = domain.ScopeKind(named)
	}
	if in.Present("scope_id") {
		scopeID, err := in.ID("scope_id")
		if err != nil {
			return nil, err
		}
		cmd.Scope.ID = scopeID
	}
	if in.Present("export_target_id") {
		targetID, err := in.ID("export_target_id")
		if err != nil {
			return nil, err
		}
		cmd.ExportTargetID = targetID
	}
	if in.Present("grace_days") {
		grace := in.Int("grace_days")
		cmd.GraceDays = &grace
	}
	if in.Present("enabled") {
		enabled := in.Bool("enabled")
		cmd.Enabled = &enabled
	}
	if in.Present("notify_recipients") || in.Present("notify_before_days") {
		notify := domain.Notify{BeforeDays: in.Int("notify_before_days")}
		named, err := in.StringList("notify_recipients")
		if err != nil {
			return nil, err
		}
		for _, recipient := range named {
			notify.Recipients = append(notify.Recipients, domain.Recipient(recipient))
		}
		cmd.Notify = &notify
	}

	rule, preview, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	out := ruleOutput(rule)
	out["preview"] = previewOutput(preview)
	return out, nil
}

// Descriptor registers the listing.
func (h ListRetentionPolicies) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListRetentionPoliciesName,
		Summary: "The retention rules this workspace has. Name a container and each rule says " +
			"whether it is the one in force there - which is the answer to \"what actually " +
			"applies here\", inheritance included, because the narrower rule wins and a rule " +
			"switched off in a collection lets the wider one through rather than stopping it.",
		SideEffects: "None. Reads only.",
		TokenScope:  retentionRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID,
				Description: "Which hub or collection to answer for.",
			},
			{
				Name: "effective", Kind: usecase.KindBool,
				Description: "Only the rules actually in force in that container.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleChangedAction, TargetType: policyTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListRetentionPolicies) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	var containerID shared.ID
	if in.Present("container_id") {
		var err error
		if containerID, err = in.ID("container_id"); err != nil {
			return nil, err
		}
	}

	rules, err := h.Execute(ctx, actor, containerID, in.Present("effective") && in.Bool("effective"))
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(rules))
	for _, entry := range rules {
		out := ruleOutput(entry.Rule)
		if !containerID.IsZero() {
			// Only when a container was named. With nothing to be in force in, the question has no
			// answer - and `false` would be one.
			out["in_force"] = entry.InForce
		}
		rows = append(rows, out)
	}
	return usecase.Output{"data": rows}, nil
}

// Descriptor registers the preview.
func (h PreviewRetentionPolicy) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: PreviewRetentionPolicyName,
		Summary: "What a rule would do, without doing it: how many objects are past its period, " +
			"what share of the scope that is, what would be kept back and why, and a handful of " +
			"the objects themselves so that a number is something somebody can check.",
		SideEffects: "None. Reads only, and deliberately: a preview that changed anything would " +
			"be the one thing it exists to avoid.",
		TokenScope: retentionRead,
		ReadOnly:   true,
		Input: []usecase.Field{
			{Name: "policy_id", Kind: usecase.KindID, Required: true, Description: "Which rule."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleChangedAction, TargetType: policyTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h PreviewRetentionPolicy) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("policy_id")
	if err != nil {
		return nil, err
	}
	preview, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return previewOutput(preview), nil
}

// ruleOutput is one rule as the three channels answer it.
func ruleOutput(rule domain.Rule) usecase.Output {
	out := usecase.Output{
		"id":          rule.ID.String(),
		"data_kind":   string(rule.DataKind),
		"retain_days": rule.RetainDays,
		"action":      string(rule.Action),
		"grace_days":  rule.GraceDays,
		"enabled":     rule.Enabled,
		"scope": map[string]any{
			"kind": string(rule.Scope.Kind),
			"id":   rule.Scope.ID.String(),
		},
	}
	if rule.Chained() {
		out["then_after_days"] = rule.ThenAfterDays
		out["then_action"] = string(rule.ThenAction)
	}
	if !rule.Notify.Silent() {
		recipients := make([]string, 0, len(rule.Notify.Recipients))
		for _, recipient := range rule.Notify.Recipients {
			recipients = append(recipients, string(recipient))
		}
		out["notify"] = map[string]any{
			"before_days": rule.Notify.BeforeDays, "recipients": recipients,
		}
	}
	for name, value := range map[string]string{
		"justification": rule.Justification, "condition": rule.Condition,
	} {
		if value != "" {
			out[name] = value
		}
	}
	if !rule.ExportTargetID.IsZero() {
		out["export_target_id"] = rule.ExportTargetID.String()
	}
	return out
}

// previewOutput is what a rule would do, as the contract spells it.
func previewOutput(preview Preview) map[string]any {
	samples := make([]map[string]any, 0, len(preview.Samples))
	for _, sample := range preview.Samples {
		samples = append(samples, map[string]any{
			"id": sample.ID.String(), "title": sample.Title,
			"effective_at": sample.EffectiveAt,
		})
	}
	return map[string]any{
		"matched":        preview.Matched,
		"blocked":        preview.Blocked,
		"share_of_scope": preview.ShareOfScope,
		"samples":        samples,
	}
}
