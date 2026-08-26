// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle_test

import (
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	ruleID     = shared.MustParseID("0198f0a0-0000-7000-8000-000000000a01")
	ruleTenant = shared.MustParseID("0198f0a0-0000-7000-8000-000000000b01")
	ruleHub    = shared.MustParseID("0198f0a0-0000-7000-8000-000000000c01")
	ruleColl   = shared.MustParseID("0198f0a0-0000-7000-8000-000000000d01")
	ruleNow    = time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
)

func ruleInput(change func(*domain.NewRuleInput)) domain.NewRuleInput {
	in := domain.NewRuleInput{
		ID: ruleID, TenantID: ruleTenant,
		Scope:    domain.Scope{Kind: domain.ScopeTenant},
		DataKind: domain.KindCompletedItem, RetainDays: 365,
		Action: domain.ActionArchive, Now: ruleNow,
	}
	change(&in)
	return in
}

func ruleCode(t *testing.T, err error) string {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	return domainErr.DetailCode
}

func TestAValidRuleIsBuilt(t *testing.T) {
	rule, err := domain.NewRule(ruleInput(func(*domain.NewRuleInput) {}))
	if err != nil {
		t.Fatalf("a valid rule was refused: %v", err)
	}
	if rule.GraceDays != domain.DefaultGraceDays {
		t.Errorf("the grace period is %d, want the documented %d", rule.GraceDays, domain.DefaultGraceDays)
	}
	if rule.Notify.Silent() {
		t.Error("a rule with a grace period warns nobody by default")
	}
	if rule.Notify.BeforeDays != domain.DefaultNotifyBeforeDays {
		t.Errorf("the warning is %d days before, want %d",
			rule.Notify.BeforeDays, domain.DefaultNotifyBeforeDays)
	}
	if !rule.Enabled || rule.Version != 1 {
		t.Errorf("a new rule is %+v", rule)
	}
}

func TestWhatARuleCannotMean(t *testing.T) {
	cases := map[string]struct {
		change func(*domain.NewRuleInput)
		code   string
	}{
		"a kind nobody defined": {
			func(in *domain.NewRuleInput) { in.DataKind = "EVERYTHING" },
			domain.CodeKindUnknown,
		},
		"a kind the document names and nothing removes": {
			func(in *domain.NewRuleInput) {
				in.DataKind, in.Action = domain.KindComment, domain.ActionHardDelete
			},
			domain.CodeKindNotSwept,
		},
		"a tenant-wide rule that names a collection": {
			func(in *domain.NewRuleInput) { in.Scope.ID = ruleColl },
			domain.CodeScopeIDMismatch,
		},
		"a collection rule that names none": {
			func(in *domain.NewRuleInput) { in.Scope = domain.Scope{Kind: domain.ScopeCollection} },
			domain.CodeScopeIDMismatch,
		},
		"an action nobody defined": {
			func(in *domain.NewRuleInput) { in.Action = "SHRED" },
			domain.CodeActionInvalid,
		},
		"an action this build cannot perform on the kind": {
			func(in *domain.NewRuleInput) { in.Action = domain.ActionAnonymize },
			domain.CodeActionNotPerformed,
		},
		"a condition, until the language exists": {
			func(in *domain.NewRuleInput) { in.Condition = "item.completed_at != null" },
			domain.CodeConditionNotAvailable,
		},
		"half a chain": {
			func(in *domain.NewRuleInput) { in.ThenAfterDays = 730 },
			domain.CodeChainIncomplete,
		},
		"a second stage after there is nothing left": {
			func(in *domain.NewRuleInput) {
				in.Action = domain.ActionHardDelete
				in.ThenAfterDays, in.ThenAction = 30, domain.ActionTrash
			},
			domain.CodeChainHasNoSecondStage,
		},
		"a period below the kind's floor": {
			func(in *domain.NewRuleInput) {
				in.DataKind, in.Action, in.RetainDays = domain.KindTrash, domain.ActionHardDelete, 1
				in.Scope = domain.Scope{Kind: domain.ScopeTenant}
			},
			domain.CodeBelowLowerBound,
		},
		"a grace period on a kind that is its own": {
			func(in *domain.NewRuleInput) {
				in.DataKind, in.Action, in.RetainDays = domain.KindTrash, domain.ActionHardDelete, 30
				days := 14
				in.GraceDays = &days
			},
			domain.CodeGraceNotApplicable,
		},
		"a warning after the act": {
			func(in *domain.NewRuleInput) {
				grace := 3
				in.GraceDays = &grace
				in.Notify = &domain.Notify{
					BeforeDays: 7, Recipients: []domain.Recipient{domain.RecipientItemMembers},
				}
			},
			domain.CodeNotifyBeyondGrace,
		},
		"an export with nowhere to write it": {
			func(in *domain.NewRuleInput) { in.Action = domain.ActionExportThenDelete },
			domain.CodeExportTargetRequired,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewRule(ruleInput(test.change))
			if err == nil {
				t.Fatal("the rule was accepted")
			}
			if code := ruleCode(t, err); code != test.code {
				t.Fatalf("refused with %s, want %s", code, test.code)
			}
		})
	}
}

// §4.4: exceeding the upper bound is allowed and costs a justification, which then reaches the
// audit. The trash is the one kind with a bound today.
func TestTheUpperBoundIsAJustificationRatherThanARefusal(t *testing.T) {
	kind, _ := domain.FindKind(domain.KindTrash)
	if kind.Ceiling() == 0 {
		t.Skip("no kind has an upper bound yet, so there is nothing to justify")
	}

	beyond := func(in *domain.NewRuleInput) {
		in.DataKind, in.Action = domain.KindTrash, domain.ActionHardDelete
		in.RetainDays = kind.Ceiling() + 1
	}

	_, err := domain.NewRule(ruleInput(beyond))
	if code := ruleCode(t, err); code != domain.CodeJustificationRequired {
		t.Fatalf("refused with %s, want %s", code, domain.CodeJustificationRequired)
	}

	rule, err := domain.NewRule(ruleInput(func(in *domain.NewRuleInput) {
		beyond(in)
		in.Justification = "The works council agreed a longer period"
	}))
	if err != nil {
		t.Fatalf("a justified rule was refused: %v", err)
	}
	if !rule.ExceedsCeiling(kind) {
		t.Error("the rule does not know it is past the bound, so nothing will audit it")
	}
}

// A rule that only announces removes nothing, so neither bound is about it.
func TestAnAnnouncementIsBoundedByNeitherLimit(t *testing.T) {
	rule, err := domain.NewRule(ruleInput(func(in *domain.NewRuleInput) {
		in.Action, in.RetainDays = domain.ActionNotifyOnly, 0
	}))
	if err != nil {
		t.Fatalf("an announcement was refused: %v", err)
	}
	if rule.Action != domain.ActionNotifyOnly {
		t.Fatalf("the rule is %s", rule.Action)
	}
}

// §2: the narrower rule wins over the wider one.
func TestTheNarrowerRuleWins(t *testing.T) {
	tenantWide := ruleAt(t, domain.Scope{Kind: domain.ScopeTenant}, 90)
	hubWide := ruleAt(t, domain.Scope{Kind: domain.ScopeHub, ID: ruleHub}, 180)
	collectionWide := ruleAt(t, domain.Scope{Kind: domain.ScopeCollection, ID: ruleColl}, 365)

	rules := []domain.Rule{tenantWide, hubWide, collectionWide}

	effective, found := domain.Effective(rules, domain.KindCompletedItem, ruleHub, ruleColl)
	if !found || effective.RetainDays != 365 {
		t.Fatalf("the collection's rule did not win: %+v", effective)
	}

	// A collection in the same hub that no collection rule names falls to the hub's.
	other := shared.MustParseID("0198f0a0-0000-7000-8000-000000000d02")
	effective, found = domain.Effective(rules, domain.KindCompletedItem, ruleHub, other)
	if !found || effective.RetainDays != 180 {
		t.Fatalf("the hub's rule did not win: %+v", effective)
	}

	// And a hub nothing names falls to the tenant's.
	otherHub := shared.MustParseID("0198f0a0-0000-7000-8000-000000000c02")
	effective, found = domain.Effective(rules, domain.KindCompletedItem, otherHub, other)
	if !found || effective.RetainDays != 90 {
		t.Fatalf("the tenant's rule did not win: %+v", effective)
	}
}

// Switching a rule off in a collection means the wider one applies there, not that nothing does.
func TestADisabledRuleShadowsNothing(t *testing.T) {
	tenantWide := ruleAt(t, domain.Scope{Kind: domain.ScopeTenant}, 90)
	collectionWide := ruleAt(t, domain.Scope{Kind: domain.ScopeCollection, ID: ruleColl}, 365)
	collectionWide.Enabled = false

	effective, found := domain.Effective(
		[]domain.Rule{tenantWide, collectionWide}, domain.KindCompletedItem, ruleHub, ruleColl)

	if !found || effective.RetainDays != 90 {
		t.Fatalf("a disabled rule did not fall through to the wider one: %+v", effective)
	}
}

func ruleAt(t *testing.T, scope domain.Scope, days int) domain.Rule {
	t.Helper()
	rule, err := domain.NewRule(ruleInput(func(in *domain.NewRuleInput) {
		in.Scope, in.RetainDays = scope, days
	}))
	if err != nil {
		t.Fatalf("building a rule at %s: %v", scope.Kind, err)
	}
	return rule
}

// RE-9's arithmetic: the second stage counts from what the first one did, not from the original
// anchor. Reading the original would collapse a three-year chain into a two-year one.
func TestTheSecondStageCountsFromWhatTheFirstOneDid(t *testing.T) {
	rule, err := domain.NewRule(ruleInput(func(in *domain.NewRuleInput) {
		in.RetainDays, in.Action = 365, domain.ActionArchive
		in.ThenAfterDays, in.ThenAction = 730, domain.ActionHardDelete
	}))
	if err != nil {
		t.Fatalf("building the chain: %v", err)
	}

	kind, _ := domain.FindKind(domain.KindCompletedItem)
	first, ok := rule.StageAnchor(kind, false)
	if !ok || first != domain.AnchorCompletedAt {
		t.Fatalf("the first stage counts from %q", first)
	}
	second, ok := rule.StageAnchor(kind, true)
	if !ok || second != domain.AnchorArchivedAt {
		t.Fatalf("the second stage counts from %q, want the column the archiving wrote", second)
	}

	// And the cutoffs are the two periods rather than their sum applied to one column.
	if got := rule.Cutoff(ruleNow); !got.Equal(ruleNow.AddDate(0, 0, -365)) {
		t.Errorf("the first cutoff is %s", got)
	}
	if got := rule.ThenCutoff(ruleNow); !got.Equal(ruleNow.AddDate(0, 0, -730)) {
		t.Errorf("the second cutoff is %s", got)
	}
}

// §4: the reasons are a precedence order, and the first match wins. An object under a hold and past
// its tombstone window is blocked by the hold - reporting the window would send an operator to look
// at the wrong thing.
func TestTheFirstBlockingReasonWins(t *testing.T) {
	both := map[string]bool{
		domain.BlockedByTombstoneWindow: true,
		domain.BlockedByLegalHold:       true,
	}
	if reason, blocked := domain.FirstBlock(both); !blocked || reason != domain.BlockedByLegalHold {
		t.Fatalf("reported %q, want the legal hold", reason)
	}

	if reason, blocked := domain.FirstBlock(map[string]bool{domain.BlockedByDescendant: true}); !blocked ||
		reason != domain.BlockedByDescendant {
		t.Fatalf("reported %q", reason)
	}
	if _, blocked := domain.FirstBlock(map[string]bool{}); blocked {
		t.Error("nothing found and something reported")
	}

	// The order is the document's, and the test says so rather than the comment.
	want := []string{
		domain.BlockedByLegalHold, domain.BlockedByRestriction,
		domain.BlockedByTombstoneWindow, domain.BlockedByDescendant,
	}
	got := domain.BlockOrder()
	if len(got) != len(want) {
		t.Fatalf("the order has %d reasons, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reason %d is %q, want %q", i, got[i], want[i])
		}
	}
}

// The catalogue is the document's, and every kind it names is here - with the ones nothing sweeps
// marked rather than absent.
func TestTheCatalogueNamesEveryKindTheDocumentDoes(t *testing.T) {
	const documented = 15
	if len(domain.Catalogue()) != documented {
		t.Fatalf("the catalogue has %d kinds, and data-retention.md §3 lists %d",
			len(domain.Catalogue()), documented)
	}
	for _, kind := range domain.Catalogue() {
		if kind.Anchor == "" {
			t.Errorf("%s says nothing about what its period runs from", kind.Name)
		}
		if kind.Swept() && len(kind.Blockable) == 0 && kind.Name != domain.KindNotification {
			t.Errorf("%s is swept and nothing can block it", kind.Name)
		}
		for _, action := range kind.Actions {
			if !action.Valid() {
				t.Errorf("%s can be %s, which is not an action", kind.Name, action)
			}
		}
	}
	if len(domain.SweptKinds()) == 0 {
		t.Fatal("nothing is swept at all")
	}
}
