// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The assignment policy against a real database (C-02): one row per scope, the rotation's cursor
// advanced under its lock, and a cross-tenant negative for every method (gate SG-3).

func policyRepo() postgres.AutoAssignPolicyRepository {
	return postgres.AutoAssignPolicyRepository{}
}

func accountCandidates(accounts ...shared.ID) []work.AutoAssignCandidate {
	candidates := make([]work.AutoAssignCandidate, 0, len(accounts))
	for _, account := range accounts {
		candidates = append(candidates, work.AutoAssignCandidate{
			Kind: work.CandidateAccount, ID: account,
		})
	}
	return candidates
}

// storedPolicy writes a policy for the collection and reads it back, so every test starts from
// what the row actually says rather than from what it handed the upsert.
func storedPolicy(
	ctx context.Context, t *testing.T, tenant, collection shared.ID,
	strategy work.AutoAssignStrategy, candidates []work.AutoAssignCandidate,
) work.AutoAssignPolicy {
	t.Helper()

	draft := work.AutoAssignPolicy{
		ID: freshID(t), ScopeType: work.AutoAssignScopeCollection, ScopeID: collection,
		Strategy: strategy, Candidates: candidates, Enabled: true,
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return policyRepo().Upsert(ctx, draft)
	}); err != nil {
		t.Fatalf("writing the policy: %v", err)
	}

	var stored work.AutoAssignPolicy
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		stored, err = policyRepo().FindForScope(ctx, work.AutoAssignScopeCollection, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the policy back: %v", err)
	}
	return stored
}

func TestAPolicyIsWrittenReadBackAndReplaced(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	ada, ben := seedAccount(ctx, t, tenantA), seedAccount(ctx, t, tenantA)

	stored := storedPolicy(ctx, t, tenantA, collection,
		work.AssignRoundRobin, accountCandidates(ada, ben))

	if stored.Strategy != work.AssignRoundRobin || stored.Version != 1 || !stored.Enabled {
		t.Errorf("stored %+v, want an enabled ROUND_ROBIN at version 1", stored)
	}
	if len(stored.Candidates) != 2 || stored.Candidates[0].ID != ada ||
		stored.Candidates[0].Kind != work.CandidateAccount {
		t.Errorf("candidates came back as %+v", stored.Candidates)
	}
	if stored.TenantID != tenantA || stored.State.Cursor != 0 {
		t.Errorf("tenant %s cursor %d, want the seeding tenant and a rotation at the head",
			stored.TenantID, stored.State.Cursor)
	}

	// The same scope written again is the same row replaced, not a second policy: the version
	// moves on and the definition is the new one.
	replaced := storedPolicy(ctx, t, tenantA, collection,
		work.AssignFixed, accountCandidates(ada))
	if replaced.ID != stored.ID {
		// The row's identity survives a replacement - the upsert conflicts on the scope, and a
		// fresh identifier would sever anything that recorded the old one.
		t.Errorf("the replacement changed the row's identity from %s to %s", stored.ID, replaced.ID)
	}
	if replaced.Strategy != work.AssignFixed || replaced.Version != 2 {
		t.Errorf("replaced %+v, want FIXED at version 2", replaced)
	}
}

func TestAReplacementResetsTheRotation(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	ada, ben := seedAccount(ctx, t, tenantA), seedAccount(ctx, t, tenantA)

	stored := storedPolicy(ctx, t, tenantA, collection,
		work.AssignRoundRobin, accountCandidates(ada, ben))

	advanced := stored
	advanced.State.Cursor = 1
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := policyRepo().Lock(ctx, work.AutoAssignScopeCollection, collection); err != nil {
			return err
		}
		return policyRepo().SaveState(ctx, advanced)
	}); err != nil {
		t.Fatalf("advancing the rotation: %v", err)
	}

	moved := storedPolicy(ctx, t, tenantA, collection,
		work.AssignRoundRobin, accountCandidates(ben, ada))
	if moved.State.Cursor != 0 {
		t.Errorf("cursor %d after a replacement, want the head: the state belonged to the old pool",
			moved.State.Cursor)
	}

	// And the state write alone spends no version: the rotation's bookkeeping must not look like
	// a configuration change (see the query).
	if moved.Version != 2 {
		t.Errorf("version %d, want 2 - one for each upsert and none for the state", moved.Version)
	}
}

// The acceptance criterion of C-02, at the row: two rotations arriving together queue on the
// lock, advance the state twice, and hand out two different candidates. A real concurrency test -
// the second transaction provably waits on the first - not two sequential calls.
func TestTwoConcurrentRotationsQueueOnTheLockedRow(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	ada, ben, chris := seedAccount(ctx, t, tenantA),
		seedAccount(ctx, t, tenantA), seedAccount(ctx, t, tenantA)
	first := seedTask(ctx, t, tenantA, authorA, collection)
	second := seedTask(ctx, t, tenantA, authorA, collection)

	storedPolicy(ctx, t, tenantA, collection,
		work.AssignRoundRobin, accountCandidates(ada, ben, chris))

	strategy, err := service.StrategyFor(work.AssignRoundRobin)
	if err != nil {
		t.Fatal(err)
	}

	// One rotation inside one transaction: lock the row, choose from the cursor, assign the
	// task, save the advanced cursor. What the auto-assign use case does, reduced to the rows.
	rotate := func(ctx context.Context, task shared.ID) error {
		policy, err := policyRepo().Lock(ctx, work.AutoAssignScopeCollection, collection)
		if err != nil {
			return err
		}
		positions := make([]int, len(policy.Candidates))
		accounts := make([]shared.ID, len(policy.Candidates))
		for i, candidate := range policy.Candidates {
			positions[i], accounts[i] = i, candidate.ID
		}
		choice, ok := strategy.Choose(service.AssignmentSelection{
			Accounts: accounts, Positions: positions,
			CandidateCount: len(accounts), Cursor: policy.State.Cursor,
		})
		if !ok {
			return errors.New("the rotation found nobody")
		}
		item, err := itemRepo().Find(ctx, task)
		if err != nil {
			return err
		}
		if err := itemRepo().SetAssignee(ctx,
			item.Assigned(choice.AccountID, changedAt), item.Version); err != nil {
			return err
		}
		policy.State.Cursor = choice.NextCursor
		return policyRepo().SaveState(ctx, policy)
	}

	// The handshake that makes the contention real: the first transaction signals once it holds
	// the lock and then dawdles; the second starts only on that signal, so its FOR UPDATE
	// provably queues behind an open transaction rather than luckily running after it.
	locked := make(chan struct{})
	outcome := make(chan error, 2)

	concurrency.Go(ctx, "test.rotation.first", func(ctx context.Context) {
		outcome <- write(ctx, t, tenantA, func(ctx context.Context) error {
			policy, err := policyRepo().Lock(ctx, work.AutoAssignScopeCollection, collection)
			if err != nil {
				return err
			}
			close(locked)
			time.Sleep(150 * time.Millisecond)

			positions := []int{0, 1, 2}
			accounts := []shared.ID{
				policy.Candidates[0].ID, policy.Candidates[1].ID, policy.Candidates[2].ID,
			}
			choice, ok := strategy.Choose(service.AssignmentSelection{
				Accounts: accounts, Positions: positions,
				CandidateCount: 3, Cursor: policy.State.Cursor,
			})
			if !ok {
				return errors.New("the rotation found nobody")
			}
			item, err := itemRepo().Find(ctx, first)
			if err != nil {
				return err
			}
			if err := itemRepo().SetAssignee(ctx,
				item.Assigned(choice.AccountID, changedAt), item.Version); err != nil {
				return err
			}
			policy.State.Cursor = choice.NextCursor
			return policyRepo().SaveState(ctx, policy)
		})
	})
	concurrency.Go(ctx, "test.rotation.second", func(ctx context.Context) {
		<-locked
		outcome <- write(ctx, t, tenantA, func(ctx context.Context) error {
			return rotate(ctx, second)
		})
	})

	for range 2 {
		if err := <-outcome; err != nil {
			t.Fatalf("rotating: %v", err)
		}
	}

	one := findItem(ctx, t, tenantA, first).AssigneeID
	two := findItem(ctx, t, tenantA, second).AssigneeID
	if one.IsZero() || two.IsZero() {
		t.Fatalf("assignees %q and %q, want both set", one, two)
	}
	if one == two {
		t.Errorf("both rotations handed the turn to %s - the cursor was read hopefully", one)
	}

	var after work.AutoAssignPolicy
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		after, err = policyRepo().FindForScope(ctx, work.AutoAssignScopeCollection, collection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if after.State.Cursor != 2 {
		t.Errorf("cursor %d after two rotations from the head, want 2", after.State.Cursor)
	}
}

// The policies document travels with the container: the LEFT JOIN in FindContainer and
// ListContainers is what puts the row's definition onto every container read (C-02).
func TestTheContainerCarriesItsPolicyDocument(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)
	ada := seedAccount(ctx, t, tenantA)

	storedPolicy(ctx, t, tenantA, collection,
		work.AssignFixed, accountCandidates(ada))

	var found work.Container
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = containerRepo().Find(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the collection: %v", err)
	}
	if found.AutoAssign == nil || found.AutoAssign.Strategy != work.AssignFixed ||
		len(found.AutoAssign.Candidates) != 1 || found.AutoAssign.Candidates[0].ID != ada ||
		!found.AutoAssign.Enabled {
		t.Fatalf("the collection carries %+v", found.AutoAssign)
	}

	// And one without a policy carries the absent key, through the same join.
	var bare work.Container
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		bare, err = containerRepo().Find(ctx, hub)
		return err
	}); err != nil {
		t.Fatalf("reading the hub: %v", err)
	}
	if bare.AutoAssign != nil {
		t.Fatalf("the hub carries %+v, want no policy", bare.AutoAssign)
	}
}

func TestThePolicyIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	ada := seedAccount(ctx, t, tenantA)

	stored := storedPolicy(ctx, t, tenantA, collection,
		work.AssignFixed, accountCandidates(ada))

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := policyRepo().FindForScope(ctx, work.AutoAssignScopeCollection, collection)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B read tenant A's policy: %v", err)
		}
	})

	t.Run("lock", func(t *testing.T) {
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := policyRepo().Lock(ctx, work.AutoAssignScopeCollection, collection)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B locked tenant A's policy: %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return policyRepo().Delete(ctx, work.AutoAssignScopeCollection, collection)
		}); err != nil {
			t.Fatalf("deleting: %v", err)
		}
		still := storedPolicy(ctx, t, tenantA, collection,
			work.AssignFixed, accountCandidates(ada))
		if still.ID != stored.ID {
			t.Errorf("tenant B's delete reached tenant A's row")
		}
	})

	t.Run("save state", func(t *testing.T) {
		moved := stored
		moved.State.Cursor = 7
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return policyRepo().SaveState(ctx, moved)
		})
		// Row level security removed the row from the update's reach; the adapter reports the
		// impossibility rather than pretending the state was written.
		if err == nil {
			t.Fatal("tenant B wrote state onto tenant A's policy")
		}
		var after work.AutoAssignPolicy
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			after, err = policyRepo().FindForScope(ctx, work.AutoAssignScopeCollection, collection)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if after.State.Cursor != 0 {
			t.Errorf("cursor %d, want tenant A's rotation untouched", after.State.Cursor)
		}
	})

	t.Run("upsert", func(t *testing.T) {
		// Tenant B writing the same scope creates a row of its own - the identifier space is
		// shared, the data is not - and tenant A's definition stays what it was.
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			foreign := work.AutoAssignPolicy{
				ID: freshID(t), ScopeType: work.AutoAssignScopeCollection, ScopeID: collection,
				Strategy:   work.AssignRandomMember,
				Candidates: accountCandidates(seedAccount(ctx, t, tenantB)),
				Enabled:    true,
			}
			return policyRepo().Upsert(ctx, foreign)
		}); err != nil {
			t.Fatalf("tenant B writing its own policy: %v", err)
		}

		var mine work.AutoAssignPolicy
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			mine, err = policyRepo().FindForScope(ctx, work.AutoAssignScopeCollection, collection)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if mine.ID != stored.ID || mine.Strategy != work.AssignFixed {
			t.Errorf("tenant B's upsert replaced tenant A's policy: %+v", mine)
		}
	})
}

func TestOpenItemsAreCountedPerAssignee(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	ada, ben := seedAccount(ctx, t, tenantA), seedAccount(ctx, t, tenantA)

	assign := func(task, account shared.ID) {
		t.Helper()
		item := findItem(ctx, t, tenantA, task)
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return itemRepo().SetAssignee(ctx, item.Assigned(account, changedAt), item.Version)
		}); err != nil {
			t.Fatalf("assigning: %v", err)
		}
	}

	open, alsoOpen := seedTask(ctx, t, tenantA, authorA, collection),
		seedTask(ctx, t, tenantA, authorA, collection)
	completed := seedTask(ctx, t, tenantA, authorA, collection)
	archived := seedTask(ctx, t, tenantA, authorA, collection)
	assign(open, ada)
	assign(alsoOpen, ada)
	assign(completed, ada)
	assign(archived, ben)

	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		"UPDATE work_item SET is_completed = true, completed_at = $2, completed_by = $3 WHERE id = $1",
		completed.String(), changedAt, authorA.String()); err != nil {
		t.Fatalf("completing: %v", err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE work_item SET archived_at = $2 WHERE id = $1",
		archived.String(), changedAt); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	var load map[shared.ID]int
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		load, err = itemRepo().CountOpenByAssignee(ctx, []shared.ID{ada, ben})
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}

	if load[ada] != 2 {
		t.Errorf("ada carries %d, want 2 - the completed entry is not load", load[ada])
	}
	if carries, counted := load[ben]; counted {
		t.Errorf("ben carries %d, want no key - the archived entry is not load", carries)
	}

	t.Run("cross-tenant", func(t *testing.T) {
		var foreign map[shared.ID]int
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			foreign, err = itemRepo().CountOpenByAssignee(ctx, []shared.ID{ada, ben})
			return err
		}); err != nil {
			t.Fatalf("counting: %v", err)
		}
		if len(foreign) != 0 {
			t.Errorf("tenant B counted tenant A's entries: %v", foreign)
		}
	})
}
