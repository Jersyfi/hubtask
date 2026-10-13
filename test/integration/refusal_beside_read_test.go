// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/port/audit"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// deniedEntries counts the DENIED entries the trail holds about one actor and one target.
func deniedEntries(ctx context.Context, t *testing.T, tenant, actor, target shared.ID) int {
	t.Helper()
	var count int
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		WHERE tenant_id = $1 AND actor_id = $2 AND target_id = $3 AND outcome = 'DENIED'`,
		tenant.String(), actor.String(), target.String()).Scan(&count); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	return count
}

// audit.md §7, UC-ID-16 check 2: a refusal is in the trail even when the reader that was refused
// was asked from inside another reader's read-only transaction. ListSuggestions reads its target
// through the catalogue inside one; the target's own reader refuses the workspace's owner, who
// holds nothing in the private hub, and its DENIED entry used to fail there
// (`cannot execute INSERT in a read-only transaction`) and be lost.
func TestARefusalInsideAReadOnlyReadIsInTheTrail(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	registry := readerRegistry(ctx, t)
	owner := reader(w.tenant, w.administrator)

	before := deniedEntries(ctx, t, w.tenant, w.administrator, w.item)
	_, err := registry.Invoke(ctx, suggestion.ListSuggestionsName, owner, usecase.Input{
		"target_type": string(suggestiondomain.TargetWorkItem), "target_id": w.item.String(),
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the owner was answered %v, want not found", err)
	}

	if got := deniedEntries(ctx, t, w.tenant, w.administrator, w.item) - before; got != 1 {
		t.Errorf("the trail gained %d DENIED entries about the entry, want one", got)
	}
}

// The port's two halves against PostgreSQL: beside a read-only transaction the write commits on its
// own, whatever the read does afterwards; inside a read-write one it joins and shares its fate -
// opening a second transaction there could wait for ever on a lock its own caller holds.
func TestWithinWritableStandsBesideAReadAndJoinsAWrite(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	sink := postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed))
	scope := persistence.Scope{TenantID: tenantA}
	readFailed := errors.New("the read failed after the write")

	entry := func(target shared.ID) audit.Entry {
		return audit.Entry{
			TenantID: tenantA, OccurredAt: created, Action: "item.read", Outcome: audit.OutcomeDenied,
			Severity: audit.SeverityNotice, ActorKind: appshared.ActorUser, ActorID: freshID(t),
			TargetType: "work_item", TargetID: target,
		}
	}
	stored := func(target shared.ID) int {
		var count int
		if err := adminPool(ctx, t).QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND target_id = $2`,
			tenantA.String(), target.String()).Scan(&count); err != nil {
			t.Fatalf("reading the trail: %v", err)
		}
		return count
	}

	beside := freshID(t)
	err := unitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		if err := unitOfWork.WithinWritable(ctx, scope, func(ctx context.Context) error {
			return sink.Append(ctx, entry(beside))
		}); err != nil {
			t.Fatalf("writing beside the read: %v", err)
		}
		return readFailed
	})
	if !errors.Is(err, readFailed) {
		t.Fatalf("the read answered %v", err)
	}
	if got := stored(beside); got != 1 {
		t.Errorf("beside a read-only transaction: %d entries stored, want one", got)
	}

	joined := freshID(t)
	err = unitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := unitOfWork.WithinWritable(ctx, scope, func(ctx context.Context) error {
			return sink.Append(ctx, entry(joined))
		}); err != nil {
			t.Fatalf("writing inside the write: %v", err)
		}
		return readFailed
	})
	if !errors.Is(err, readFailed) {
		t.Fatalf("the write answered %v", err)
	}
	if got := stored(joined); got != 0 {
		t.Errorf("inside a read-write transaction that rolled back: %d entries stored, want none", got)
	}
}
