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
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Search by meaning under a restriction (UC-PRV-04 check 5, UC-PRV-03 check 10): restricting
// forgets, in its own transaction, the vectors of what the person created and of what they are on;
// the pass embeds neither while the restriction stands, and both again once it is lifted.
func TestARestrictedPersonsEntriesLeaveSearchByMeaning(t *testing.T) {
	ctx := context.Background()
	if !semanticStoreAvailable(ctx, t) {
		t.Skip("this database carries no pgvector")
	}
	w := seedKeptWorld(ctx, t)
	assigned := w.assignedToKim(ctx, t, "ADAFORKIM plan the drinks")
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
		return countIn(ctx, t, `SELECT count(*) FROM item_embedding WHERE item_id = $1`, item.String()) > 0
	}

	if sent := run(); !strings.Contains(sent, "KIMOWN") || !embedded(w.kimEntry) || !embedded(assigned) {
		t.Fatalf("before the restriction Kim's entries were not embedded, so the test proves nothing:\n%s", sent)
	}

	restrict := func(restricted bool) {
		t.Helper()
		restriction := privacyservice.RestrictProcessing{
			Subjects: privacyRepo(), Kept: privacyRepo(), Authorizer: permissive{},
			Audit:      postgres.NewAuditSink(clockadapter.NewUUIDv7(clockadapter.System{})),
			UnitOfWork: unitOfWork, Clock: portclock.Fixed(time.Now().UTC()),
			Embeddings: postgres.NewEmbeddingRepository(),
		}
		if err := restriction.Execute(ctx, w.as(w.ada), privacyservice.RestrictCommand{
			AccountID: w.kim, Restricted: restricted, Reason: "Art. 18",
		}); err != nil {
			t.Fatalf("restricted=%v: %v", restricted, err)
		}
	}

	restrict(true)
	if embedded(w.kimEntry) || embedded(assigned) {
		t.Error("Kim's vectors outlived the transaction that restricted her")
	}
	if !embedded(w.adaEntry) {
		t.Error("Ada's vector was forgotten with Kim's")
	}
	if sent := run(); strings.Contains(sent, "KIMOWN") || strings.Contains(sent, "ADAFORKIM") ||
		embedded(w.kimEntry) || embedded(assigned) {
		t.Errorf("the pass embedded Kim's entries while she is restricted:\n%s", sent)
	}

	restrict(false)
	if sent := run(); !strings.Contains(sent, "KIMOWN") || !embedded(w.kimEntry) || !embedded(assigned) {
		t.Errorf("after the restriction was lifted Kim's entries were not embedded again:\n%s", sent)
	}

	// Another workspace cannot forget anything of this one's.
	other := seedKeptWorld(ctx, t)
	if err := write(ctx, t, other.tenant, func(ctx context.Context) error {
		forgotten, err := postgres.NewEmbeddingRepository().ForgetAssigned(ctx, []shared.ID{w.kim})
		if forgotten != 0 {
			t.Errorf("another workspace forgot %d of this one's vectors", forgotten)
		}
		return err
	}); err != nil {
		t.Fatalf("forgetting next door: %v", err)
	}
	if !embedded(assigned) {
		t.Error("a forget next door reached this workspace's vectors")
	}
}
