// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
)

// ScopeKind is how wide a rule reaches (data-retention.md §2).
type ScopeKind string

const (
	ScopeTenant     ScopeKind = "TENANT"
	ScopeHub        ScopeKind = "HUB"
	ScopeCollection ScopeKind = "COLLECTION"
)

// depth is how narrow a scope is, and it is the whole of "the narrower rule wins over the wider
// one". A number rather than a comparison written at each call site: three places deciding which of
// two rules applies would be three chances for two of them to disagree.
func (s ScopeKind) depth() int {
	switch s {
	case ScopeCollection:
		return 3
	case ScopeHub:
		return 2
	case ScopeTenant:
		return 1
	}
	return 0
}

func (s ScopeKind) Valid() bool { return s.depth() > 0 }

// Scope is what a rule covers.
type Scope struct {
	Kind ScopeKind
	// ID is the hub or the collection, and empty for a tenant-wide rule.
	ID shared.ID
}

// Recipient is who an advance warning goes to (data-retention.md §2).
type Recipient string

const (
	RecipientItemMembers      Recipient = "ITEM_MEMBERS"
	RecipientCollectionAdmins Recipient = "COLLECTION_ADMINS"
	RecipientTenantAdmins     Recipient = "TENANT_ADMINS"
)

var recipients = [...]Recipient{
	RecipientItemMembers, RecipientCollectionAdmins, RecipientTenantAdmins,
}

func (r Recipient) Valid() bool { return slices.Contains(recipients[:], r) }

// Notify is the advance warning, and an empty one means it is switched off.
type Notify struct {
	BeforeDays int
	Recipients []Recipient
}

// Silent reports a warning nobody gets.
func (n Notify) Silent() bool { return len(n.Recipients) == 0 }

// DefaultNotifyBeforeDays is the seven days §6 gives as the default warning.
const DefaultNotifyBeforeDays = 7

// DefaultGraceDays is §2's fourteen: the gap between the announcement and the act.
const DefaultGraceDays = 14

// Rule is one retention rule, as data-retention.md §2 writes it.
type Rule struct {
	ID       shared.ID
	TenantID shared.ID
	Scope    Scope
	DataKind DataKind
	// Condition is an optional CEL expression, evaluated per candidate by the sweep
	// (data-retention.md §2, ADR-0009). Empty is the ordinary case: a rule with no condition is
	// decided by its scope and its period alone.
	//
	// Whether the text is an expression is the compiler's question and is asked in the application
	// layer, because core/domain may not hold an evaluator (ADR-0001). What is checked here is that
	// it is not longer than an expression may be - the same bound the engine applies, restated so
	// that a rule cannot be stored with one the engine would refuse to compile.
	Condition  string
	RetainDays int
	Action     Action
	// ThenAfterDays and ThenAction are the second stage of a chain, counted from what the first
	// stage did rather than from the original anchor. Both or neither.
	ThenAfterDays int
	ThenAction    Action
	GraceDays     int
	Notify        Notify
	// Justification is why the period exceeds the kind's upper bound, and is required exactly then.
	Justification string
	Enabled       bool
	// ExportTargetID is where EXPORT_THEN_DELETE writes its archive.
	ExportTargetID shared.ID
	CreatedBy      shared.ID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int
}

// Chained reports a rule with a second stage.
func (r Rule) Chained() bool { return r.ThenAction != "" }

// NewRuleInput is one rule as somebody asked for it.
type NewRuleInput struct {
	ID             shared.ID
	TenantID       shared.ID
	Scope          Scope
	DataKind       DataKind
	Condition      string
	RetainDays     int
	Action         Action
	ThenAfterDays  int
	ThenAction     Action
	GraceDays      *int
	Notify         *Notify
	Justification  string
	Enabled        *bool
	ExportTargetID shared.ID
	CreatedBy      shared.ID
	Now            time.Time
	// Ceiling is the upper bound in force for this kind, and zero when there is none.
	//
	// A parameter rather than a field of the catalogue, because §4.4 makes it the operator's:
	// "prevent effectively unlimited storage **where the operator has set a maximum period**". The
	// catalogue's kinds carry no maximum, and `retention_policy.max_days` is where an operator puts
	// one - so the caller reads it and hands it in, and the domain decides what it means.
	Ceiling int
}

// NewRule builds a rule and refuses what the model cannot mean.
//
// Everything decided here is decided from the request and the catalogue, which between them are the
// whole of §2 and §3. What is *not* decided here is anything that needs the world: whether the
// export target exists, whether the container is this tenant's, and whether the rule would touch
// more than five per cent of the holdings are questions for the application layer, which has the
// ports to ask them.
func NewRule(in NewRuleInput) (Rule, error) {
	kind, known := FindKind(in.DataKind)
	switch {
	case in.ID.IsZero() || in.TenantID.IsZero():
		return Rule{}, invalidRule(CodeRuleIncomplete, "/data_kind")
	case !known:
		return Rule{}, invalidRule(CodeKindUnknown, "/data_kind")
	// A kind the document names and nothing here removes. Refused rather than accepted silently,
	// which is the reasoning `lifecycle.history_not_wired` already carries: a period configured
	// against nothing would look like a working installation until somebody checked.
	case !kind.Swept():
		return Rule{}, shared.ErrConflict.WithDetail(CodeKindNotSwept).
			WithParams(map[string]string{"data_kind": string(in.DataKind)}).
			WithFields(shared.FieldError{Path: "/data_kind", Code: CodeKindNotSwept})
	case !in.Scope.Kind.Valid():
		return Rule{}, invalidRule(CodeScopeInvalid, "/scope")
	case (in.Scope.Kind == ScopeTenant) != in.Scope.ID.IsZero():
		return Rule{}, invalidRule(CodeScopeIDMismatch, "/scope")
	case !in.Action.Valid():
		return Rule{}, invalidRule(CodeActionInvalid, "/action")
	case !kind.Performs(in.Action):
		return Rule{}, shared.ErrConflict.WithDetail(CodeActionNotPerformed).
			WithParams(map[string]string{
				"data_kind": string(in.DataKind), "action": string(in.Action),
			}).
			WithFields(shared.FieldError{Path: "/action", Code: CodeActionNotPerformed})
	case in.RetainDays < 0:
		return Rule{}, invalidRule(CodeRetainDaysInvalid, "/retain_days")
	}

	if len(strings.TrimSpace(in.Condition)) > MaxConditionLength {
		return Rule{}, invalidRule(CodeConditionTooLong, "/condition")
	}
	if err := validateChain(in, kind); err != nil {
		return Rule{}, err
	}
	if err := validateBounds(in, kind); err != nil {
		return Rule{}, err
	}

	grace, err := graceOf(in, kind)
	if err != nil {
		return Rule{}, err
	}
	notify, err := notifyOf(in, grace)
	if err != nil {
		return Rule{}, err
	}
	if in.Action == ActionExportThenDelete && in.ExportTargetID.IsZero() {
		return Rule{}, invalidRule(CodeExportTargetRequired, "/export_target_id")
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return Rule{
		ID: in.ID, TenantID: in.TenantID, Scope: in.Scope, DataKind: in.DataKind,
		// Trimmed, so that what the sweep compiles is what somebody wrote rather than what they
		// happened to leave whitespace around.
		Condition:  strings.TrimSpace(in.Condition),
		RetainDays: in.RetainDays, Action: in.Action,
		ThenAfterDays: in.ThenAfterDays, ThenAction: in.ThenAction,
		GraceDays: grace, Notify: notify, Justification: strings.TrimSpace(in.Justification),
		Enabled: enabled, ExportTargetID: in.ExportTargetID,
		CreatedBy: in.CreatedBy, CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}, nil
}

// validateChain is §2's "completed → archive after 1 year → delete after 2 more".
//
// A second stage needs something left to act on, which is what makes ARCHIVE and TRASH the only
// first stages a chain can have: after a hard delete there is no object, and after an anonymisation
// there is nothing left that a period is about.
func validateChain(in NewRuleInput, kind Kind) error {
	if in.ThenAction == "" && in.ThenAfterDays == 0 {
		return nil
	}
	switch {
	case in.ThenAction == "" || in.ThenAfterDays <= 0:
		return invalidRule(CodeChainIncomplete, "/then_action")
	case !in.ThenAction.Valid() || in.ThenAction == ActionNotifyOnly:
		return invalidRule(CodeActionInvalid, "/then_action")
	case !kind.Performs(in.ThenAction):
		return shared.ErrConflict.WithDetail(CodeActionNotPerformed).
			WithParams(map[string]string{
				"data_kind": string(in.DataKind), "action": string(in.ThenAction),
			}).
			WithFields(shared.FieldError{Path: "/then_action", Code: CodeActionNotPerformed})
	}
	if _, leaves := in.Action.Leaves(); !leaves {
		return shared.ErrConflict.WithDetail(CodeChainHasNoSecondStage).
			WithParams(map[string]string{"action": string(in.Action)}).
			WithFields(shared.FieldError{Path: "/then_action", Code: CodeChainHasNoSecondStage})
	}
	return nil
}

// validateBounds is §4.3 and §4.4.
//
// The lower bound is a refusal here rather than a silent clamp, and that is the difference between
// this and the old Policy: a period a tenant *configures* is a decision they are making, and
// answering it with a different number they never asked for is worse than telling them the number
// they may not go below. The old path clamped because it was reading a row nobody had reviewed.
func validateBounds(in NewRuleInput, kind Kind) error {
	if in.Action == ActionNotifyOnly {
		// A rule that only announces removes nothing, so neither bound is about it: the lower
		// bound exists to stop an accidental immediate deletion and the upper one to stop
		// effectively unlimited storage, and an announcement does neither.
		return nil
	}
	if in.RetainDays < kind.MinDays {
		return shared.ErrConflict.WithDetail(CodeBelowLowerBound).
			WithParams(map[string]string{
				"data_kind": string(in.DataKind), "min_days": strconv.Itoa(kind.MinDays),
			}).
			WithFields(shared.FieldError{Path: "/retain_days", Code: CodeBelowLowerBound})
	}
	total := in.RetainDays + in.ThenAfterDays
	if ceiling := ceilingOf(in, kind); ceiling > 0 && total > ceiling &&
		strings.TrimSpace(in.Justification) == "" {
		return shared.ErrValidation.WithDetail(CodeJustificationRequired).
			WithParams(map[string]string{
				"data_kind": string(in.DataKind), "max_days": strconv.Itoa(ceilingOf(in, kind)),
			}).
			WithFields(shared.FieldError{Path: "/justification", Code: CodeJustificationRequired})
	}
	return nil
}

// ceilingOf is the upper bound in force: the operator's where they have set one, and the
// catalogue's otherwise. Today no kind carries one, so the operator's is the only one there is.
func ceilingOf(in NewRuleInput, kind Kind) int {
	if in.Ceiling > 0 {
		return in.Ceiling
	}
	return kind.Ceiling()
}

// ExceedsCeiling reports a rule whose whole chain runs past an upper bound, which is what §4.4
// makes auditable. Read from the rule rather than remembered from the request, so that the audit
// entry and the refusal are decided by the same reading.
func (r Rule) ExceedsCeiling(ceiling int) bool {
	return ceiling > 0 && r.RetainDays+r.ThenAfterDays > ceiling
}

// graceOf is the gap between the announcement and the act.
//
// A kind that does not go through the marking phase may not have one, and that is a refusal rather
// than a value quietly set to zero. The trash is its own grace period - the object is visible, it
// can be taken out and it has a date - so a rule asking for fourteen days on top would be asking
// for an announcement of something already announced, and somebody would be waiting for a warning
// that never comes.
func graceOf(in NewRuleInput, kind Kind) (int, error) {
	if !kind.Marks {
		if in.GraceDays != nil && *in.GraceDays > 0 {
			return 0, shared.ErrConflict.WithDetail(CodeGraceNotApplicable).
				WithParams(map[string]string{"data_kind": string(in.DataKind)}).
				WithFields(shared.FieldError{Path: "/grace_days", Code: CodeGraceNotApplicable})
		}
		return 0, nil
	}
	if in.GraceDays == nil {
		return DefaultGraceDays, nil
	}
	if *in.GraceDays < 0 {
		return 0, invalidRule(CodeGraceInvalid, "/grace_days")
	}
	return *in.GraceDays, nil
}

// notifyOf is the advance warning (§6), stored since R-1 was answered in G-12.
//
// §6 asks for two kinds of visibility: the object carries what is coming, and those affected get a
// message. Both exist now - the marking and `retention` on the entry for the first, the RETENTION
// notification category and the resolution of COLLECTION_ADMINS and TENANT_ADMINS for the second -
// so a rule that asks to warn somebody is stored and honoured rather than refused.
//
// BeforeDays is bounded by the grace period, and the bound is the promise the engine keeps: the
// warning goes out when the entry is marked, which is the earliest moment there is anything true
// to say and therefore at least this many days ahead. The field says how *late* a warning may be,
// and this build is never later than the marking - one before it would be about something nobody
// has decided yet, and one after the act would be a condolence.
func notifyOf(in NewRuleInput, grace int) (Notify, error) {
	if in.Notify == nil || in.Notify.Silent() {
		return Notify{}, nil
	}

	notify := *in.Notify
	for _, recipient := range notify.Recipients {
		if !recipient.Valid() {
			return Notify{}, invalidRule(CodeRecipientInvalid, "/notify/recipients")
		}
	}
	if notify.BeforeDays < 0 || notify.BeforeDays > grace {
		return Notify{}, shared.ErrValidation.WithDetail(CodeNotifyBeyondGrace).
			WithParams(map[string]string{"grace_days": strconv.Itoa(grace)}).
			WithFields(shared.FieldError{Path: "/notify/before_days", Code: CodeNotifyBeyondGrace})
	}
	if notify.BeforeDays == 0 {
		// The documented default rather than "immediately". A rule that names recipients and no
		// number is asking to warn them, and §6's seven days is what it is asking for.
		notify.BeforeDays = DefaultNotifyBeforeDays
	}
	return notify, nil
}

// Applies reports whether a rule covers an object in this hub and this collection.
func (r Rule) Applies(hubID, collectionID shared.ID) bool {
	switch r.Scope.Kind {
	case ScopeTenant:
		return true
	case ScopeHub:
		return r.Scope.ID == hubID
	case ScopeCollection:
		return r.Scope.ID == collectionID
	}
	return false
}

// Narrower reports whether this rule wins over the other one (§2: the narrower rule wins).
//
// Ties are impossible within one kind, because one rule per kind per scope is what the schema's
// unique index makes true - two rules that both said COLLECTION would be two rules for the same
// collection, which cannot be stored.
func (r Rule) Narrower(other Rule) bool {
	return r.Scope.Kind.depth() > other.Scope.Kind.depth()
}

// Effective answers the rule in force for an object of this kind in this place, and false when
// none is.
//
// Disabled rules are not candidates, and that is worth being explicit about: a disabled collection
// rule does not fall back to the tenant's, it *shadows* nothing - the tenant's rule applies because
// it is the narrowest one that is on. Treating a disabled rule as a blocker would make "switch this
// off here" mean "and let the wider one through", which is the opposite of what somebody switching
// a rule off in a collection means.
func Effective(rules []Rule, kind DataKind, hubID, collectionID shared.ID) (Rule, bool) {
	var winner Rule
	var found bool
	for _, rule := range rules {
		if rule.DataKind != kind || !rule.Enabled || !rule.Applies(hubID, collectionID) {
			continue
		}
		if !found || rule.Narrower(winner) {
			winner, found = rule, true
		}
	}
	return winner, found
}

// Cutoff is the instant an object's anchor has to lie before for the first stage to be due.
func (r Rule) Cutoff(now time.Time) time.Time {
	return now.AddDate(0, 0, -r.RetainDays)
}

// ThenCutoff is the instant for the second stage, counted from what the first stage did.
func (r Rule) ThenCutoff(now time.Time) time.Time {
	return now.AddDate(0, 0, -r.ThenAfterDays)
}

// StageAnchor is the column each stage counts from: the kind's for the first, and the column the
// first stage's action wrote for the second.
func (r Rule) StageAnchor(kind Kind, second bool) (Anchor, bool) {
	if !second {
		return kind.Anchor, true
	}
	return r.Action.Leaves()
}

// EffectiveAt is when the act falls due for an object whose anchor is at that instant: the period,
// and then the grace the announcement bought.
func (r Rule) EffectiveAt(anchoredAt time.Time) time.Time {
	return anchoredAt.AddDate(0, 0, r.RetainDays+r.GraceDays)
}

func invalidRule(code, field string) error {
	return shared.ErrValidation.WithDetail(code).
		WithFields(shared.FieldError{Path: field, Code: code}).
		WithCause(errors.New(code))
}

// The refusals of the rule model, as codes rather than as prose.
const (
	CodeRuleIncomplete     = "lifecycle.rule_incomplete"
	CodeKindUnknown        = "lifecycle.data_kind_unknown"
	CodeKindNotSwept       = "lifecycle.data_kind_not_swept"
	CodeScopeInvalid       = "lifecycle.scope_invalid"
	CodeScopeIDMismatch    = "lifecycle.scope_id_mismatch"
	CodeActionInvalid      = "lifecycle.action_invalid"
	CodeActionNotPerformed = "lifecycle.action_not_performed"
	CodeRetainDaysInvalid  = "lifecycle.retain_days_invalid"
	// CodeConditionTooLong is an expression past what the engine will compile. Restated here so
	// that a rule cannot be stored with one the sweep would refuse every time it ran.
	CodeConditionTooLong = "lifecycle.condition_too_long"

	// MaxConditionLength is the engine's own bound, restated. The number lives in
	// core/port/expression; core/domain may import a port, and repeating the value rather than the
	// import would be two numbers that could drift.
	MaxConditionLength        = expression.MaxLength
	CodeChainIncomplete       = "lifecycle.chain_incomplete"
	CodeChainHasNoSecondStage = "lifecycle.chain_has_no_second_stage"
	CodeBelowLowerBound       = "lifecycle.below_lower_bound"
	CodeJustificationRequired = "lifecycle.justification_required"
	CodeGraceNotApplicable    = "lifecycle.grace_not_applicable"
	CodeGraceInvalid          = "lifecycle.grace_invalid"
	CodeRecipientInvalid      = "lifecycle.recipient_invalid"
	CodeNotifyBeyondGrace     = "lifecycle.notify_beyond_grace"
	CodeExportTargetRequired  = "lifecycle.export_target_required"
	CodeExportUnavailable     = "lifecycle.export_unavailable"
	CodeRuleNotFound          = "lifecycle.rule_not_found"
	CodeRuleAlreadyExists     = "lifecycle.rule_already_exists"
	CodeNotMarked             = "lifecycle.not_marked"
)
