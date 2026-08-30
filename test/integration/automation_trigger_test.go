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
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The other four triggers against a real database (G-08). What only PostgreSQL can answer: that
// the due index answers by moment, that the occurrence upsert really is one statement, that a
// rotation replaces the address rather than adding one, and that every one of the new methods is
// bounded by the tenant of the transaction (gate SG-3).

func automationInbound() postgres.AutomationInboundRepository {
	// A fixed installation secret rather than a random one, so a hash printed in a failing test is
	// the same value on a rerun.
	return postgres.NewAutomationInboundRepository(
		security.NewInboundTokenHasher(secret.New("integration test installation secret")))
}

// scheduledFixture is a rule the schedule pass can find: enabled, of the right kind, and owing a
// moment.
func scheduledFixture(
	ctx context.Context, t *testing.T, tenant, runAs, author shared.ID, due time.Time,
) domain.Rule {
	t.Helper()

	rule := ruleFixture(t, tenant, runAs, author)
	rule.Trigger = domain.Trigger{
		Kind: domain.TriggerSchedule, RRule: "FREQ=DAILY;BYHOUR=3", Timezone: "Europe/Berlin",
	}
	rule.Enabled, rule.NextRunAt = true, due.UTC().Truncate(time.Microsecond)

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the scheduled rule: %v", err)
	}
	return rule
}

// The due read is the whole of the poller's question, and the index behind it is the one E-05's
// backup schedules read the same way.
func TestTheDueScheduleReadAnswersByMoment(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	past := time.Now().UTC().Add(-time.Hour)
	soon := time.Now().UTC().Add(time.Hour)
	overdue := scheduledFixture(ctx, t, tenantA, runAs, authorA, past)
	later := scheduledFixture(ctx, t, tenantA, runAs, authorA, soon)

	var due []domain.Rule
	var next time.Time
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if due, err = automationRules().Due(ctx, time.Now().UTC(), 50); err != nil {
			return err
		}
		next, err = automationRules().NextDue(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading what is due: %v", err)
	}

	if len(due) != 1 || due[0].ID != overdue.ID {
		t.Fatalf("due answered %d rules, want only the one whose moment has passed", len(due))
	}
	if !next.Equal(overdue.NextRunAt) {
		t.Errorf("next due %v, want the earliest moment owed", next)
	}

	// Advancing is not an edit: the version stays where it was, so a client holding an optimistic
	// lock does not see every occurrence as somebody else's change.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().SetNextRun(ctx, overdue.ID, soon)
	}); err != nil {
		t.Fatalf("advancing: %v", err)
	}

	var moved domain.Rule
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		moved, err = automationRules().Find(ctx, overdue.ID)
		return err
	}); err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if moved.Version != overdue.Version {
		t.Errorf("version %d after an advance, want %d unchanged", moved.Version, overdue.Version)
	}
	if !moved.NextRunAt.Equal(soon.Truncate(time.Microsecond)) {
		t.Errorf("moved to %v, want the new moment", moved.NextRunAt)
	}

	// A rule switched off owes nothing, whatever its column says. The index is partial on exactly
	// that, so this is the predicate rather than a filter in Go.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().SetEnabled(ctx, later.ID, false, later.Version, time.Now().UTC())
	}); err != nil {
		t.Fatalf("switching off: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		next, err = automationRules().NextDue(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the next moment: %v", err)
	}
	if !next.Equal(soon.Truncate(time.Microsecond)) {
		t.Errorf("next due %v, want only the rule that is still on", next)
	}
}

// The upsert is one statement, which is what stops "the due date moved" from leaving a window in
// which the tenant owed nothing; the claim removes what it reads, because the row *is* the debt.
func TestAnOccurrenceMovesInOneStatementAndIsClaimedOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)
	rule := ruleFixture(t, tenantA, runAs, authorA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	item := seedWorkItemForRules(ctx, t, tenantA)

	first := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond)
	moved := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := automationRules().Upsert(ctx, domain.Occurrence{
			ID: freshID(t), TenantID: tenantA, RuleID: rule.ID, ItemID: item, FireAt: first,
		}); err != nil {
			return err
		}
		// The same (rule, entry) again: the unique index makes this a move rather than a second
		// debt, which is what stops one deadline firing twice.
		return automationRules().Upsert(ctx, domain.Occurrence{
			ID: freshID(t), TenantID: tenantA, RuleID: rule.ID, ItemID: item, FireAt: moved,
		})
	}); err != nil {
		t.Fatalf("writing the occurrence: %v", err)
	}

	var claimed []domain.Occurrence
	var again []domain.Occurrence
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if claimed, err = automationRules().ClaimDue(ctx, time.Now().UTC(), 50); err != nil {
			return err
		}
		again, err = automationRules().ClaimDue(ctx, time.Now().UTC(), 50)
		return err
	}); err != nil {
		t.Fatalf("claiming: %v", err)
	}

	if len(claimed) != 1 {
		t.Fatalf("claimed %d occurrences, want the one moved moment", len(claimed))
	}
	if !claimed[0].FireAt.Equal(moved) || claimed[0].ItemID != item {
		t.Errorf("claimed %+v, want the moved moment for this entry", claimed[0])
	}
	if len(again) != 0 {
		t.Errorf("claimed %d occurrences a second time - the row is the debt", len(again))
	}

	// Forgetting is what a cleared anchor does, and it is idempotent: the second call is somebody
	// making sure.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := automationRules().Forget(ctx, rule.ID, item); err != nil {
			return err
		}
		return automationRules().ForgetItem(ctx, item)
	}); err != nil {
		t.Fatalf("forgetting: %v", err)
	}
}

// Rotating replaces the address in one statement: the old token and the new one never both open
// the rule, which is what "revocable by rotating" has to mean.
func TestRotatingAnInboundAddressReplacesIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	rule := ruleFixture(t, tenantA, runAs, authorA)
	rule.Trigger = domain.Trigger{Kind: domain.TriggerInboundWebhook}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	first := inboundTokenFor(t, tenantA, 1)
	second := inboundTokenFor(t, tenantA, 2)
	at := time.Now().UTC().Truncate(time.Microsecond)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		changed, err := automationInbound().SetToken(ctx, rule.ID, first, at)
		if err != nil || !changed {
			t.Fatalf("minting: changed=%v err=%v", changed, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("minting: %v", err)
	}

	var found domain.Rule
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = automationInbound().FindByToken(ctx, first)
		return err
	}); err != nil {
		t.Fatalf("the address does not open the rule: %v", err)
	}
	if found.ID != rule.ID || !found.InboundRotatedAt.Equal(at) {
		t.Errorf("found %+v, want the rule and when its address was minted", found)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := automationInbound().SetToken(ctx, rule.ID, second, at.Add(time.Minute))
		return err
	}); err != nil {
		t.Fatalf("rotating: %v", err)
	}

	err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, findErr := automationInbound().FindByToken(ctx, first)
		return findErr
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the previous address still opens the rule: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, findErr := automationInbound().FindByToken(ctx, second)
		return findErr
	}); err != nil {
		t.Errorf("the new address does not open the rule: %v", err)
	}
}

// The cross-tenant negative every new repository method owes (gate SG-3, security.md §6).
//
// The inbound lookup is the sharpest of them: it is reached from a route with no authentication,
// and the only thing between tenant B's token and tenant A's rule is row level security plus the
// hash covering the tenant half of the credential.
func TestTheNewTriggerMethodsCannotReachAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	due := time.Now().UTC().Add(-time.Hour)
	rule := scheduledFixture(ctx, t, tenantA, runAs, authorA, due)
	item := seedWorkItemForRules(ctx, t, tenantA)

	token := inboundTokenFor(t, tenantA, 9)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := automationInbound().SetToken(ctx, rule.ID, token, time.Now().UTC()); err != nil {
			return err
		}
		return automationRules().Upsert(ctx, domain.Occurrence{
			ID: freshID(t), TenantID: tenantA, RuleID: rule.ID, ItemID: item, FireAt: due,
		})
	}); err != nil {
		t.Fatalf("seeding tenant A: %v", err)
	}

	t.Run("due", func(t *testing.T) {
		var due []domain.Rule
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			due, err = automationRules().Due(ctx, time.Now().UTC(), 50)
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		if len(due) != 0 {
			t.Errorf("tenant B was answered %d of tenant A's due rules", len(due))
		}
	})

	t.Run("next due", func(t *testing.T) {
		var next time.Time
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			next, err = automationRules().NextDue(ctx)
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		if !next.IsZero() {
			t.Errorf("tenant B owes %v, which is tenant A's moment", next)
		}
	})

	t.Run("set next run", func(t *testing.T) {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return automationRules().SetNextRun(ctx, rule.ID, time.Now().UTC().Add(time.Hour))
		}); err != nil {
			t.Fatalf("advancing: %v", err)
		}
		var still domain.Rule
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			still, err = automationRules().Find(ctx, rule.ID)
			return err
		}); err != nil {
			t.Fatalf("reading it back: %v", err)
		}
		if !still.NextRunAt.Equal(rule.NextRunAt) {
			t.Errorf("tenant B moved tenant A's rule to %v", still.NextRunAt)
		}
	})

	t.Run("by trigger kind", func(t *testing.T) {
		var rules []domain.Rule
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			rules, err = automationRuns().ByTriggerKind(ctx, domain.TriggerSchedule)
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		if len(rules) != 0 {
			t.Errorf("tenant B was answered %d of tenant A's rules", len(rules))
		}
	})

	t.Run("claim due occurrences", func(t *testing.T) {
		var claimed []domain.Occurrence
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			claimed, err = automationRules().ClaimDue(ctx, time.Now().UTC(), 50)
			return err
		}); err != nil {
			t.Fatalf("claiming: %v", err)
		}
		if len(claimed) != 0 {
			t.Errorf("tenant B claimed %d of tenant A's occurrences", len(claimed))
		}
	})

	t.Run("next occurrence", func(t *testing.T) {
		var next time.Time
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			next, err = automationRules().NextOccurrence(ctx)
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		if !next.IsZero() {
			t.Errorf("tenant B owes %v, which is tenant A's occurrence", next)
		}
	})

	t.Run("upsert an occurrence", func(t *testing.T) {
		// The composite foreign key is on (tenant_id, rule_id): tenant B writing `current_tenant_id()`
		// into the row cannot reference tenant A's rule, so the write is refused rather than
		// silently landing in the wrong workspace.
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return automationRules().Upsert(ctx, domain.Occurrence{
				ID: freshID(t), TenantID: tenantB, RuleID: rule.ID, ItemID: item,
				FireAt: time.Now().UTC(),
			})
		})
		if err == nil {
			t.Error("tenant B wrote an occurrence against tenant A's rule")
		}
	})

	t.Run("forget an occurrence", func(t *testing.T) {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			if err := automationRules().Forget(ctx, rule.ID, item); err != nil {
				return err
			}
			return automationRules().ForgetItem(ctx, item)
		}); err != nil {
			t.Fatalf("forgetting: %v", err)
		}
		var next time.Time
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			next, err = automationRules().NextOccurrence(ctx)
			return err
		}); err != nil {
			t.Fatalf("reading it back: %v", err)
		}
		if next.IsZero() {
			t.Error("tenant B forgot what tenant A owed")
		}
	})

	t.Run("find by inbound token", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, findErr := automationInbound().FindByToken(ctx, token)
			return findErr
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("error %v, want ErrNotFound - tenant B opened tenant A's address", err)
		}
	})

	t.Run("set an inbound token", func(t *testing.T) {
		theirs := inboundTokenFor(t, tenantB, 4)
		var changed bool
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			changed, err = automationInbound().SetToken(ctx, rule.ID, theirs, time.Now().UTC())
			return err
		}); err != nil {
			t.Fatalf("minting: %v", err)
		}
		if changed {
			t.Error("tenant B minted an address on tenant A's rule")
		}
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			_, findErr := automationInbound().FindByToken(ctx, token)
			return findErr
		}); err != nil {
			t.Errorf("tenant A's own address stopped working: %v", err)
		}
	})
}

// inboundTokenFor mints a token whose secret is fixed by a seed, so that a failing test prints the
// same value on a rerun.
func inboundTokenFor(t *testing.T, tenant shared.ID, seed byte) integration.InboundToken {
	t.Helper()

	entropy := make([]byte, integration.InboundTokenSecretBytes)
	for i := range entropy {
		entropy[i] = seed + byte(i)
	}
	token, err := integration.NewInboundToken(tenant, entropy)
	if err != nil {
		t.Fatalf("minting a token: %v", err)
	}
	return token
}

// seedWorkItemForRules writes one entry an occurrence can point at. The foreign key is composite,
// so the entry has to be the tenant's own.
func seedWorkItemForRules(ctx context.Context, t *testing.T, tenant shared.ID) shared.ID {
	t.Helper()
	admin := adminPool(ctx, t)

	// A hub first: `container`'s check constraint ties `type = 'HUB'` to `parent_id IS NULL`, so a
	// collection without one is refused by the schema rather than by anything this test asks.
	hub := freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		VALUES ($1, $2, 'HUB', $3, 'a0', $4)`,
		hub.String(), tenant.String(), freshName(t), authorA.String()); err != nil {
		t.Fatalf("seeding the hub: %v", err)
	}

	collection := freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		VALUES ($1, $2, 'COLLECTION', $3, $4, 'a0', $5)`,
		collection.String(), tenant.String(), hub.String(), freshName(t),
		authorA.String()); err != nil {
		t.Fatalf("seeding the collection: %v", err)
	}

	item := freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key,
		                       content_language, created_by)
		VALUES ($1, $2, $3, 'TASK', '/' || $1 || '/', 1, 'Something due', 'a0', 'en', $4)`,
		item.String(), tenant.String(), collection.String(), authorA.String()); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	return item
}

var _ repository.InboundTriggers = postgres.AutomationInboundRepository{}
