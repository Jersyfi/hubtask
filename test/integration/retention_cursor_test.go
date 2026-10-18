// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Where each read of the rule sweep stopped (data-retention.md §5): one place per reader, kept per
// workspace, and the boundary each method may not cross (gate SG-3).

func cursorRepo() postgres.RetentionCursorRepository { return postgres.NewRetentionCursorRepository() }

func cursorOf(ctx context.Context, t *testing.T, tenant shared.ID, reader repository.Reader) (repository.Position, bool) {
	t.Helper()
	var at repository.Position
	var found bool
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		at, found, err = cursorRepo().Find(ctx, reader)
		return err
	}); err != nil {
		t.Fatalf("reading the cursor: %v", err)
	}
	return at, found
}

func setCursor(ctx context.Context, t *testing.T, tenant shared.ID, reader repository.Reader, at repository.Position) {
	t.Helper()
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return cursorRepo().Set(ctx, reader, at)
	}); err != nil {
		t.Fatalf("writing the cursor: %v", err)
	}
}

// One place per reader: a second write replaces the first, and the three phases keep three places.
func TestARetentionCursorKeepsOnePlacePerReader(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	announce := repository.Reader{Phase: repository.ReaderAnnounce, DataKind: domain.KindArchivedItem}
	act := repository.Reader{Phase: repository.ReaderAct}

	first := repository.Position{At: created.Add(-48 * time.Hour), ID: freshID(t)}
	second := repository.Position{At: created.Add(-24 * time.Hour), ID: freshID(t)}
	setCursor(ctx, t, tenantA, announce, first)
	setCursor(ctx, t, tenantA, announce, second)
	setCursor(ctx, t, tenantA, act, first)

	if at, found := cursorOf(ctx, t, tenantA, announce); !found || at.ID != second.ID || !at.At.Equal(second.At) {
		t.Errorf("phase one's place is %+v (found %v), want the second write %+v", at, found, second)
	}
	if at, found := cursorOf(ctx, t, tenantA, act); !found || at.ID != first.ID {
		t.Errorf("phase two's place is %+v (found %v), want its own %+v", at, found, first)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return cursorRepo().Clear(ctx, announce)
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if _, found := cursorOf(ctx, t, tenantA, announce); found {
		t.Error("a cleared reader still has a place")
	}
	if _, found := cursorOf(ctx, t, tenantA, act); !found {
		t.Error("clearing one reader cleared another")
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return cursorRepo().Clear(ctx, act)
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
}

// The cross-tenant negative test for Find, Set and Clear: the same reader keeps a place in each
// workspace, B never reads A's, B's write leaves A's alone, and B's clear removes only B's.
func TestARetentionCursorIsPerTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	reader := repository.Reader{Phase: repository.ReaderAnnounce, DataKind: domain.KindCompletedItem}

	inA := repository.Position{At: created.Add(-72 * time.Hour), ID: freshID(t)}
	setCursor(ctx, t, tenantA, reader, inA)
	t.Cleanup(func() {
		_ = write(ctx, t, tenantA, func(ctx context.Context) error { return cursorRepo().Clear(ctx, reader) })
		_ = write(ctx, t, tenantB, func(ctx context.Context) error { return cursorRepo().Clear(ctx, reader) })
	})

	if _, found := cursorOf(ctx, t, tenantB, reader); found {
		t.Fatal("tenant B read tenant A's place")
	}

	inB := repository.Position{At: created.Add(-1 * time.Hour), ID: freshID(t)}
	setCursor(ctx, t, tenantB, reader, inB)
	if at, _ := cursorOf(ctx, t, tenantA, reader); at.ID != inA.ID {
		t.Errorf("tenant B's write moved tenant A's place to %+v", at)
	}

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return cursorRepo().Clear(ctx, reader)
	}); err != nil {
		t.Fatalf("B clearing: %v", err)
	}
	if _, found := cursorOf(ctx, t, tenantA, reader); !found {
		t.Error("tenant B's clear removed tenant A's place")
	}
}

// A chain's place goes with its rule: a withdrawn rule leaves no position behind.
func TestAChainsCursorGoesWithItsRule(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	clearRules(ctx, t, tenantA)
	rule := aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 365)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return ruleRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	chain := repository.Reader{Phase: repository.ReaderChain, RuleID: rule.ID}
	setCursor(ctx, t, tenantA, chain, repository.Position{At: created, ID: freshID(t)})

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := ruleRepo().Delete(ctx, rule.ID)
		return err
	}); err != nil {
		t.Fatalf("withdrawing the rule: %v", err)
	}
	if _, found := cursorOf(ctx, t, tenantA, chain); found {
		t.Error("the chain's place outlived its rule")
	}
}

// A reader of a shape the table refuses is refused before the statement, with a code.
func TestARetentionCursorRefusesAReaderOfTheWrongShape(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return cursorRepo().Set(ctx, repository.Reader{Phase: repository.ReaderAnnounce},
			repository.Position{At: created, ID: freshID(t)})
	})
	if shared.AsError(err).DetailCode != "retention.cursor_reader_invalid" {
		t.Fatalf("a phase-one reader without its kind: %v", err)
	}
}
