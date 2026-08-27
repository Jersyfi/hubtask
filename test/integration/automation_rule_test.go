// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The rules against a real database (G-05). What only PostgreSQL can answer: that the whole
// definition survives the four jsonb columns, that the version guard in the WHERE is a guard, that
// the soft delete hides a rule from every read, and that row level security keeps a tenant's rules
// to itself.

func automationRules() postgres.AutomationRuleRepository {
	return postgres.NewAutomationRuleRepository(pageCursors())
}

// seedServiceAccount writes an account a rule can act as.
func seedServiceAccount(ctx context.Context, t *testing.T, tenant shared.ID) shared.ID {
	t.Helper()

	id := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, display_name, status)
		VALUES ($1, $2, 'SERVICE_ACCOUNT', 'Automation', 'ACTIVE')`,
		id.String(), tenant.String()); err != nil {
		t.Fatalf("seeding the service account: %v", err)
	}
	return id
}

func ruleFixture(t *testing.T, tenant, runAs, author shared.ID) domain.Rule {
	t.Helper()

	rule, err := domain.NewRule(domain.NewRuleInput{
		ID: freshID(t), TenantID: tenant, Name: freshName(t),
		Scope: domain.Scope{Type: domain.ScopeTenant}, RunAs: runAs,
		Trigger: domain.Trigger{Kind: domain.TriggerEvent, EventType: event.ItemOverdue},
		Actions: []domain.Action{
			{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x", "count": float64(2)}},
		},
		Throttle:  domain.Throttle{MaxRunsPerHour: 100},
		OnError:   domain.OnErrorContinue,
		CreatedBy: author, Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatalf("building the rule: %v", err)
	}
	return rule
}

// The whole definition through the four jsonb columns and back: what is stored is what is read.
func TestARuleSurvivesTheColumnsItIsStoredIn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	written := ruleFixture(t, tenantA, runAs, authorA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Insert(ctx, written)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	var stored domain.Rule
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		stored, findErr = automationRules().Find(ctx, written.ID)
		return findErr
	}); err != nil {
		t.Fatalf("reading the rule: %v", err)
	}

	if stored.Name != written.Name || stored.RunAs != runAs || stored.CreatedBy != authorA {
		t.Errorf("read back %+v", stored)
	}
	if stored.Enabled {
		t.Error("a rule was stored switched on")
	}
	if stored.Scope != written.Scope {
		t.Errorf("scope %+v, want %+v", stored.Scope, written.Scope)
	}
	if stored.Trigger.Kind != domain.TriggerEvent || stored.Trigger.EventType != event.ItemOverdue {
		t.Errorf("trigger %+v", stored.Trigger)
	}
	if stored.OnError != domain.OnErrorContinue {
		t.Errorf("on_error %q", stored.OnError)
	}
	if stored.Throttle.MaxRunsPerHour != 100 {
		t.Errorf("throttle %+v", stored.Throttle)
	}
	if len(stored.Actions) != 1 || stored.Actions[0].Kind != "ADD_LABEL" {
		t.Fatalf("actions %+v", stored.Actions)
	}
	// A parameter is a document a rule carries, and jsonb has to give it back unchanged.
	if got := stored.Actions[0].Params["label_id"]; got != "x" {
		t.Errorf("label_id came back %v", got)
	}
	if got := stored.Actions[0].Params["count"]; got != float64(2) {
		t.Errorf("count came back %v (%T), want 2", got, got)
	}
	// Empty rather than null, so a caller is answered a list either way.
	if stored.Conditions == nil {
		t.Error("conditions came back null")
	}
	if !stored.CreatedAt.Equal(written.CreatedAt) {
		t.Errorf("created at %v, want %v", stored.CreatedAt, written.CreatedAt)
	}
}

// The guard is in the WHERE rather than in a read-then-write, because a check in the application
// layer is a check something else can commit between.
func TestAStaleVersionCannotOverwriteARule(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	rule := ruleFixture(t, tenantA, runAs, authorA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	rule.Name = freshName(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Update(ctx, rule, 1)
	}); err != nil {
		t.Fatalf("the first edit: %v", err)
	}

	// The second writer read version 1 as well.
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Update(ctx, rule, 1)
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error %v, want ErrConflict", err)
	}
}

// A guarded update matches nothing when the version moved *or* when the row is gone, and the two
// are different answers to the caller.
func TestAnEditOfADeletedRuleIsNotFound(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	rule := ruleFixture(t, tenantA, runAs, authorA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := automationRules().Insert(ctx, rule); err != nil {
			return err
		}
		_, err := automationRules().Delete(ctx, rule.ID, time.Now().UTC())
		return err
	}); err != nil {
		t.Fatalf("writing and deleting the rule: %v", err)
	}

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Update(ctx, rule, 1)
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want ErrNotFound", err)
	}
}

// Switching a rule on clears the failure count; switching it off leaves it, because a rule somebody
// stopped by hand has not been fixed.
func TestSwitchingARuleOnClearsItsFailureCount(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	rule := ruleFixture(t, tenantA, runAs, authorA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE automation_rule SET failure_count = 3 WHERE id = $1`, rule.ID.String()); err != nil {
		t.Fatalf("seeding the failure count: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().SetEnabled(ctx, rule.ID, true, 1, time.Now().UTC())
	}); err != nil {
		t.Fatalf("switching on: %v", err)
	}

	var on domain.Rule
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		on, findErr = automationRules().Find(ctx, rule.ID)
		return findErr
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if !on.Enabled || on.FailureCount != 0 {
		t.Fatalf("enabled=%v failures=%d, want on and zero", on.Enabled, on.FailureCount)
	}

	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE automation_rule SET failure_count = 2 WHERE id = $1`, rule.ID.String()); err != nil {
		t.Fatalf("seeding the failure count: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().SetEnabled(ctx, rule.ID, false, on.Version, time.Now().UTC())
	}); err != nil {
		t.Fatalf("switching off: %v", err)
	}

	var off domain.Rule
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		off, findErr = automationRules().Find(ctx, rule.ID)
		return findErr
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if off.Enabled {
		t.Error("the rule is still on")
	}
	if off.FailureCount != 2 {
		t.Errorf("failures %d, want 2 - switching off by hand has not fixed anything", off.FailureCount)
	}
}

// The deletion is soft, and the row stops answering every read there is.
func TestADeletedRuleIsGoneFromEveryRead(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	rule := ruleFixture(t, tenantA, runAs, authorA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	var first, again bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if first, err = automationRules().Delete(ctx, rule.ID, time.Now().UTC()); err != nil {
			return err
		}
		// The second call is somebody making sure, not an error.
		again, err = automationRules().Delete(ctx, rule.ID, time.Now().UTC())
		return err
	}); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if !first || again {
		t.Errorf("delete answered %v then %v, want true then false", first, again)
	}

	err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, findErr := automationRules().Find(ctx, rule.ID)
		return findErr
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a deleted rule is still found: %v", err)
	}

	var page repository.Page
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var listErr error
		page, listErr = automationRules().List(ctx, repository.Query{Size: 200})
		return listErr
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	for _, listed := range page.Rules {
		if listed.ID == rule.ID {
			t.Error("a deleted rule is still listed")
		}
	}

	// The tombstone also switches it off: nothing would find it either way, and a row saying it is
	// enabled and never enabled again reads as a lie to whoever opens the table next.
	var enabled bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT enabled FROM automation_rule WHERE id = $1`, rule.ID.String()).
		Scan(&enabled); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if enabled {
		t.Error("the deleted row still says it is enabled")
	}
}

// The listing pages by identifier, which is the creation order, and the filter is a nullable
// argument rather than a second statement.
func TestTheListingPagesAndFilters(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantB)

	written := make([]shared.ID, 0, 3)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		for range 3 {
			rule := ruleFixture(t, tenantB, runAs, authorB)
			if err := automationRules().Insert(ctx, rule); err != nil {
				return err
			}
			written = append(written, rule.ID)
		}
		return automationRules().SetEnabled(ctx, written[0], true, 1, time.Now().UTC())
	}); err != nil {
		t.Fatalf("writing the rules: %v", err)
	}

	// Two pages of one, walked with the cursor, cover the three without repeating any.
	seen := map[shared.ID]int{}
	cursor := ""
	for round := range 5 {
		var page repository.Page
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var listErr error
			page, listErr = automationRules().List(ctx, repository.Query{Cursor: cursor, Size: 1})
			return listErr
		}); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		for _, rule := range page.Rules {
			seen[rule.ID]++
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	for _, id := range written {
		if seen[id] != 1 {
			t.Errorf("rule %s was answered %d times over the walk", id, seen[id])
		}
	}

	enabled := true
	var only repository.Page
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var listErr error
		only, listErr = automationRules().List(ctx, repository.Query{Enabled: &enabled, Size: 200})
		return listErr
	}); err != nil {
		t.Fatalf("filtering: %v", err)
	}
	for _, rule := range only.Rules {
		if !rule.Enabled {
			t.Errorf("rule %s is off and was answered to a filter for the ones that are on", rule.ID)
		}
	}
}

// The cross-tenant negative test for every method on the repository. Row level security narrows the
// table to the transaction's tenant, so another workspace's rule is invisible rather than forbidden.
func TestARuleCannotBeReachedFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	rule := ruleFixture(t, tenantA, runAs, authorA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, findErr := automationRules().Find(ctx, rule.ID)
			return findErr
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("error %v, want ErrNotFound", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		var page repository.Page
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var listErr error
			page, listErr = automationRules().List(ctx, repository.Query{Size: 200})
			return listErr
		}); err != nil {
			t.Fatalf("listing: %v", err)
		}
		for _, listed := range page.Rules {
			if listed.ID == rule.ID {
				t.Error("tenant B was answered a rule of tenant A")
			}
		}
	})

	t.Run("update", func(t *testing.T) {
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return automationRules().Update(ctx, rule, 1)
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("error %v, want ErrNotFound", err)
		}
	})

	t.Run("set enabled", func(t *testing.T) {
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return automationRules().SetEnabled(ctx, rule.ID, true, 1, time.Now().UTC())
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("error %v, want ErrNotFound", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		var changed bool
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			var deleteErr error
			changed, deleteErr = automationRules().Delete(ctx, rule.ID, time.Now().UTC())
			return deleteErr
		}); err != nil {
			t.Fatalf("deleting: %v", err)
		}
		if changed {
			t.Error("tenant B deleted a rule of tenant A")
		}
	})

	// And the rule is untouched afterwards.
	var still domain.Rule
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		still, findErr = automationRules().Find(ctx, rule.ID)
		return findErr
	}); err != nil {
		t.Fatalf("reading it back in its own tenant: %v", err)
	}
	if still.Name != rule.Name || still.Version != 1 || still.Enabled {
		t.Errorf("the rule was changed from outside its tenant: %+v", still)
	}
}

// The composite foreign key on (tenant_id, run_as) is what stops a rule acting as an account of
// another workspace. The column pair has carried it since 0001_init and nothing has tested it.
func TestARuleCannotRunAsAnAccountOfAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	foreign := seedServiceAccount(ctx, t, tenantB)

	rule := ruleFixture(t, tenantA, foreign, authorA)
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	})
	if err == nil {
		t.Fatal("a rule was written naming an account of another tenant")
	}
	if !errors.Is(err, shared.ErrUnavailable) && !errors.Is(err, shared.ErrConflict) {
		t.Logf("refused as %v", err)
	}
}
