// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The rule model and the two phases against a real database (E-07): one rule per kind per level,
// the marking that survives between the phases, the referential safeguard, and the boundary each
// method may not cross (gate SG-3).

func ruleRepo() postgres.RetentionRuleRepository { return postgres.NewRetentionRuleRepository() }
func markingRepo() postgres.RetentionMarkingRepository {
	return postgres.NewRetentionMarkingRepository()
}

// clearRules empties the tenant's rules before a test that cares about the unique index.
//
// One rule per kind per level is tenant-wide by design, so a rule another test left behind would
// make every later test in the package meet a conflict. A tenant per test would hide exactly the
// interference these tests are about.
func clearRules(ctx context.Context, t *testing.T, tenant shared.ID) {
	t.Helper()
	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM retention_rule WHERE tenant_id = $1`, tenant.String()); err != nil {
		t.Fatalf("clearing the retention rules: %v", err)
	}
}

func aRule(t *testing.T, tenant shared.ID, scope domain.Scope, days int) domain.Rule {
	t.Helper()
	rule, err := domain.NewRule(domain.NewRuleInput{
		ID: freshID(t), TenantID: tenant, Scope: scope,
		DataKind: domain.KindCompletedItem, RetainDays: days,
		Action: domain.ActionArchive, CreatedBy: authorA, Now: created,
	})
	if err != nil {
		t.Fatalf("building the rule: %v", err)
	}
	return rule
}

func TestARuleIsWrittenAndReadBackWhole(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	clearRules(ctx, t, tenantA)
	rule := aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 365)

	var found domain.Rule
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := ruleRepo().Insert(ctx, rule); err != nil {
			return err
		}
		var err error
		found, err = ruleRepo().Find(ctx, rule.ID)
		return err
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	switch {
	case found.DataKind != domain.KindCompletedItem:
		t.Errorf("the kind came back as %s", found.DataKind)
	case found.RetainDays != 365 || found.Action != domain.ActionArchive:
		t.Errorf("the rule came back as %d days of %s", found.RetainDays, found.Action)
	case found.GraceDays != domain.DefaultGraceDays:
		t.Errorf("the grace period came back as %d", found.GraceDays)
	// Nothing warns anybody yet, so a rule nobody asked to warn carries no warning rather than one
	// nothing sends.
	case !found.Notify.Silent():
		t.Errorf("the warning came back as %+v", found.Notify)
	case !found.Enabled:
		t.Error("a new rule came back switched off")
	}
}

// One rule per kind per level, which is the unique index. A second one at the same level is a
// conflict the caller can read rather than a second answer to "what applies here".
func TestASecondRuleForOneKindAtOneLevelIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	clearRules(ctx, t, tenantA)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return ruleRepo().Insert(ctx, aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 365))
	}); err != nil {
		t.Fatalf("writing the first rule: %v", err)
	}

	// A second transaction, because that is what a second request is: a unique violation poisons
	// the transaction it happens in, so a test that wrote both in one would be testing the commit
	// rather than the index.
	second := write(ctx, t, tenantA, func(ctx context.Context) error {
		return ruleRepo().Insert(ctx, aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 90))
	})

	if !errors.Is(second, shared.ErrConflict) {
		t.Fatalf("a second tenant-wide rule for one kind was accepted: %v", second)
	}
}

// And two at different levels are exactly what the model is for.
func TestTwoRulesForOneKindAtDifferentLevelsCoexist(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	clearRules(ctx, t, tenantA)

	var rules []domain.Rule
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := ruleRepo().Insert(ctx, aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 90)); err != nil {
			return err
		}
		scoped := domain.Scope{Kind: domain.ScopeCollection, ID: collection}
		if err := ruleRepo().Insert(ctx, aRule(t, tenantA, scoped, 365)); err != nil {
			return err
		}
		var err error
		rules, err = ruleRepo().List(ctx)
		return err
	}); err != nil {
		t.Fatalf("writing the rules: %v", err)
	}

	var forCollection, forTenant bool
	for _, rule := range rules {
		if rule.DataKind != domain.KindCompletedItem {
			continue
		}
		switch rule.Scope.Kind {
		case domain.ScopeCollection:
			forCollection = rule.Scope.ID == collection && rule.RetainDays == 365
		case domain.ScopeTenant:
			forTenant = rule.RetainDays == 90
		}
	}
	if !forCollection || !forTenant {
		t.Fatalf("the two rules did not come back: %+v", rules)
	}
}

// completedItem seeds one entry that finished at a given moment.
func completedItem(ctx context.Context, t *testing.T, tenant, collection shared.ID, at time.Time) shared.ID {
	t.Helper()
	id := freshID(t)
	item := taskIn(tenant, authorA, collection, id, freshName(t), "a0")
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, item)
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE work_item SET is_completed = true, completed_at = $3, completed_by = $4
		 WHERE tenant_id = $1 AND id = $2`,
		tenant.String(), id.String(), at, authorA.String()); err != nil {
		t.Fatalf("completing the entry: %v", err)
	}
	return id
}

func TestTheCandidatesAreWhatIsPastTheCutoffAndNotMarkedYet(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	cutoff := time.Now().UTC().Add(-24 * time.Hour)

	old := completedItem(ctx, t, tenantA, collection, cutoff.Add(-time.Hour))
	recent := completedItem(ctx, t, tenantA, collection, time.Now().UTC())

	var candidates []repository.Candidate
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		candidates, err = markingRepo().Due(ctx, domain.AnchorCompletedAt, cutoff, 100)
		return err
	}); err != nil {
		t.Fatalf("reading the candidates: %v", err)
	}

	seen := map[shared.ID]repository.Candidate{}
	for _, candidate := range candidates {
		seen[candidate.ID] = candidate
	}
	if _, due := seen[old]; !due {
		t.Error("an entry completed before the cutoff is not a candidate")
	}
	if _, due := seen[recent]; due {
		t.Error("an entry completed today is a candidate")
	}
	// The hub comes with it, because a hub-scoped rule is matched against it and an entry three
	// levels down does not know which hub it is under.
	if seen[old].HubID.IsZero() {
		t.Error("a candidate does not know which hub it is under")
	}
	if seen[old].Type != work.ItemTask {
		t.Errorf("the candidate is a %s", seen[old].Type)
	}
}

// The marking is what survives between the phases, and it is what phase two reads.
func TestAMarkedEntryIsFoundByPhaseTwoWhenItsGraceHasRunOut(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	clearRules(ctx, t, tenantA)
	rule := aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 1)
	id := completedItem(ctx, t, tenantA, collection, time.Now().UTC().Add(-48*time.Hour))

	now := time.Now().UTC()
	var marked, dueLater, dueNow int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := ruleRepo().Insert(ctx, rule); err != nil {
			return err
		}
		var err error
		marked, err = markingRepo().Mark(ctx, []shared.ID{id}, rule.ID,
			domain.ActionArchive, now.Add(time.Hour))
		if err != nil {
			return err
		}
		later, err := markingRepo().MarkedDue(ctx, now, 100)
		if err != nil {
			return err
		}
		dueLater = countOf(later, id)

		soon, err := markingRepo().MarkedDue(ctx, now.Add(2*time.Hour), 100)
		if err != nil {
			return err
		}
		dueNow = countOf(soon, id)
		return nil
	}); err != nil {
		t.Fatalf("marking: %v", err)
	}

	if marked != 1 {
		t.Fatalf("%d entries were marked", marked)
	}
	if dueLater != 0 {
		t.Error("an entry whose grace period is still running is already due")
	}
	if dueNow != 1 {
		t.Error("an entry whose grace period has run out is not due")
	}
}

func countOf(candidates []repository.Candidate, id shared.ID) int {
	count := 0
	for _, candidate := range candidates {
		if candidate.ID == id {
			count++
		}
	}
	return count
}

// RE-4's half at this level: taking an entry out ends the rule's claim on it, and the entry stops
// being a candidate for phase two.
func TestClearingAMarkingEndsTheRulesClaim(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	clearRules(ctx, t, tenantA)
	rule := aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 1)
	id := completedItem(ctx, t, tenantA, collection, time.Now().UTC().Add(-48*time.Hour))
	now := time.Now().UTC()

	var after repository.Candidate
	var stillDue int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := ruleRepo().Insert(ctx, rule); err != nil {
			return err
		}
		if _, err := markingRepo().Mark(ctx, []shared.ID{id}, rule.ID, domain.ActionArchive, now); err != nil {
			return err
		}
		if _, err := markingRepo().Clear(ctx, []shared.ID{id}, false, now); err != nil {
			return err
		}
		due, err := markingRepo().MarkedDue(ctx, now.Add(time.Hour), 100)
		if err != nil {
			return err
		}
		stillDue = countOf(due, id)
		after, err = markingRepo().Marking(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("retaining: %v", err)
	}

	if stillDue != 0 {
		t.Error("an entry taken out of its period is still due")
	}
	if !after.Pending.IsZero() || !after.Rule.IsZero() {
		t.Errorf("the marking survived being cleared: %+v", after)
	}
}

// And a stage that has acted keeps the rule's claim, because the chain's next stage is what owns
// the entry then.
func TestAStageThatActedKeepsTheRulesClaim(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	clearRules(ctx, t, tenantA)
	rule := aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 1)
	id := completedItem(ctx, t, tenantA, collection, time.Now().UTC().Add(-48*time.Hour))
	now := time.Now().UTC()

	var after repository.Candidate
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := ruleRepo().Insert(ctx, rule); err != nil {
			return err
		}
		if _, err := markingRepo().Mark(ctx, []shared.ID{id}, rule.ID, domain.ActionArchive, now); err != nil {
			return err
		}
		if _, err := markingRepo().Archive(ctx, []shared.ID{id}, now); err != nil {
			return err
		}
		if _, err := markingRepo().Clear(ctx, []shared.ID{id}, true, now); err != nil {
			return err
		}
		var err error
		after, err = markingRepo().Marking(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("acting: %v", err)
	}

	if after.Rule != rule.ID {
		t.Fatalf("the rule's claim is %s, want %s", after.Rule, rule.ID)
	}
	if !after.Pending.IsZero() {
		t.Error("the entry is still announced after the act")
	}
}

// §4.6: a parent is kept back while something below it is not going in this pass.
func TestAParentIsKeptBackWhileSomethingBelowItStays(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task, pkg, activity := trashableSubtree(ctx, t, tenantA, authorA, collection)

	var counted map[shared.ID]int
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		// The task is judged while only the activity is going: the work package between them stays,
		// so the task is kept back.
		counted, err = markingRepo().RetainedDescendants(ctx,
			[]shared.ID{task.ID}, []shared.ID{activity.ID})
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if counted[task.ID] == 0 {
		t.Fatalf("nothing below the task was counted, and %s is still there", pkg.ID)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		counted, err = markingRepo().RetainedDescendants(ctx,
			[]shared.ID{task.ID}, []shared.ID{pkg.ID, activity.ID})
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if counted[task.ID] != 0 {
		t.Errorf("%d entries below the task were counted although both are going", counted[task.ID])
	}
}

// Gate SG-3: every method, across the boundary.
func TestARetentionRuleAndItsMarkingsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantB, authorB)
	clearRules(ctx, t, tenantA)
	clearRules(ctx, t, tenantB)
	rule := aRule(t, tenantB, domain.Scope{Kind: domain.ScopeTenant}, 1)
	theirs := completedItem(ctx, t, tenantB, collection, time.Now().UTC().Add(-48*time.Hour))
	now := time.Now().UTC()

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if err := ruleRepo().Insert(ctx, rule); err != nil {
			return err
		}
		_, err := markingRepo().Mark(ctx, []shared.ID{theirs}, rule.ID, domain.ActionArchive, now)
		return err
	}); err != nil {
		t.Fatalf("seeding B: %v", err)
	}

	var find error
	var listed []domain.Rule
	var due []repository.Candidate
	var cleared, archived int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, find = ruleRepo().Find(ctx, rule.ID)
		var err error
		if listed, err = ruleRepo().List(ctx); err != nil {
			return err
		}
		if due, err = markingRepo().MarkedDue(ctx, now.Add(time.Hour), 100); err != nil {
			return err
		}
		if cleared, err = markingRepo().Clear(ctx, []shared.ID{theirs}, false, now); err != nil {
			return err
		}
		archived, err = markingRepo().Archive(ctx, []shared.ID{theirs}, now)
		return err
	}); err != nil {
		t.Fatalf("reading from A: %v", err)
	}

	if !errors.Is(find, shared.ErrNotFound) {
		t.Errorf("tenant A found tenant B's rule: %v", find)
	}
	for _, listedRule := range listed {
		if listedRule.ID == rule.ID {
			t.Errorf("tenant A listed tenant B's rule")
		}
	}
	if countOf(due, theirs) != 0 {
		t.Error("tenant A was told tenant B's entry is due")
	}
	if cleared != 0 || archived != 0 {
		t.Errorf("tenant A cleared %d and archived %d of tenant B's entries", cleared, archived)
	}
}
