// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	privacydomain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Search by meaning (UC-PRV-05 checks 5 and 7): a kept-out person's entries are not embedded, their
// vectors are forgotten in the transaction that records the objection, and they come back once the
// person lets their content in again.
func TestAKeptOutPersonsEntriesLeaveSearchByMeaning(t *testing.T) {
	ctx := context.Background()
	if !semanticStoreAvailable(ctx, t) {
		t.Skip("this database carries no pgvector")
	}
	w := seedKeptWorld(ctx, t)
	recorder := &recordingProvider{}
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	pass := work.EmbedItems{
		Embeddings: postgres.NewEmbeddingRepository(), Providers: recorder,
		Semantic: postgres.NewSemanticSearchRepository(), UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, Kept: keptOutExclusion(), BatchSize: 50,
	}
	system := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: w.tenant}
	run := func() string {
		t.Helper()
		if _, err := pass.Execute(ctx, system); err != nil {
			t.Fatalf("the pass: %v", err)
		}
		return strings.Join(recorder.take(), "\n")
	}
	embedded := func(item shared.ID) bool {
		t.Helper()
		var count int
		if err := adminPool(ctx, t).QueryRow(ctx,
			`SELECT count(*) FROM item_embedding WHERE item_id = $1`, item.String()).Scan(&count); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return count > 0
	}

	if sent := run(); !strings.Contains(sent, "KIMOWN") || !embedded(w.kimEntry) {
		t.Fatalf("before the objection Kim's entry was not embedded, so the test proves nothing:\n%s", sent)
	}

	// Kim withdraws through the use case: her vectors go in the same transaction.
	withdraw := privacyservice.WithdrawConsent{
		Consents: privacyRepo(), Authorizer: realAuthoriser(ctx, t),
		Audit: postgres.NewAuditSink(clockadapter.NewUUIDv7(clockadapter.System{})), UnitOfWork: unitOfWork,
		Clock: portclock.Fixed(time.Now().UTC()), IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
		Embeddings: postgres.NewEmbeddingRepository(),
	}
	kim := w.as(w.kim)
	kim.Scopes = []string{"privacy:manage"}
	if _, err := withdraw.Execute(ctx, kim, privacyservice.WithdrawCommand{Purpose: privacydomain.PurposeAi}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if embedded(w.kimEntry) {
		t.Error("Kim's vector outlived the transaction that recorded her objection")
	}
	if !embedded(w.adaEntry) {
		t.Error("Ada's vector was forgotten with Kim's")
	}
	// Ada's entry changes: the pass re-embeds it and sends nothing of Kim's.
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE work_item SET notes = 'changed' WHERE id = $1`, w.adaEntry.String()); err != nil {
		t.Fatalf("changing Ada's entry: %v", err)
	}
	if sent := run(); strings.Contains(sent, "KIMOWN") || strings.Contains(sent, "KIMNOTES") || embedded(w.kimEntry) {
		t.Errorf("the pass embedded Kim's entry after her objection:\n%s", sent)
	}

	// Kim lets her content back in: the pass embeds her entry again.
	w.record(ctx, t, true, time.Now().UTC().Add(time.Minute))
	if sent := run(); !strings.Contains(sent, "KIMOWN") || !embedded(w.kimEntry) {
		t.Errorf("after Kim gave consent again her entry was not embedded:\n%s", sent)
	}

	// Another workspace cannot forget anything of this one's.
	other := seedKeptWorld(ctx, t)
	if err := write(ctx, t, other.tenant, func(ctx context.Context) error {
		forgotten, err := postgres.NewEmbeddingRepository().Forget(ctx, []shared.ID{w.kim, w.ada})
		if forgotten != 0 {
			t.Errorf("another workspace forgot %d of this one's vectors", forgotten)
		}
		return err
	}); err != nil {
		t.Fatalf("forgetting next door: %v", err)
	}
	if !embedded(w.kimEntry) || !embedded(w.adaEntry) {
		t.Error("a forget next door reached this workspace's vectors")
	}
}

func semanticStoreAvailable(ctx context.Context, t *testing.T) bool {
	t.Helper()
	var available bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT to_regclass('public.item_embedding') IS NOT NULL`).Scan(&available); err != nil {
		t.Fatalf("asking for the store: %v", err)
	}
	return available
}
