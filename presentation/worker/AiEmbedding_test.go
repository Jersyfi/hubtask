// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The handler's own job is small and worth pinning down: the tenant it runs for, and when it comes
// back. Everything else is the application layer's, and is tested there.

var embedTenant = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")

// owedStore hands the pass a fixed number of entries and swallows what comes back, which is enough
// for the handler's decision about when to return to be measured.
type owedStore struct{ rows int }

func (s owedStore) Owed(_ context.Context, _ string, batch int) ([]repository.OwedEmbedding, error) {
	owed := make([]repository.OwedEmbedding, 0, s.rows)
	for i := range min(s.rows, batch) {
		owed = append(owed, repository.OwedEmbedding{
			ItemID: shared.MustParseID("0192f000-0000-7000-8000-0000000004" + string("0123456789ab"[i%12]) + "1"),
			Title:  "an entry",
		})
	}
	return owed, nil
}

func (s owedStore) CountMissing(context.Context, int) (int, error) { return s.rows, nil }
func (s owedStore) Store(context.Context, repository.StoredEmbedding) error {
	return nil
}

type presentStore struct{}

func (presentStore) Available(context.Context) (bool, error) { return true, nil }

type passthroughUnitOfWork struct{}

func (passthroughUnitOfWork) Within(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	return fn(ctx)
}

func (passthroughUnitOfWork) WithinReadOnly(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	return fn(ctx)
}

type batchProvider struct{}

func (batchProvider) Capabilities() aiprovider.ProviderCapabilities {
	return aiprovider.ProviderCapabilities{
		Kind: "stub", Embedding: true, EmbeddingModel: "embed-3", EmbeddingDimensions: 3,
	}
}

func (batchProvider) Complete(
	context.Context, aiprovider.CompletionRequest,
) (aiprovider.CompletionResult, error) {
	return aiprovider.CompletionResult{}, aiprovider.ErrUnavailable
}

func (batchProvider) Embed(
	_ context.Context, texts []string,
) (aiprovider.EmbeddingResult, error) {
	vectors := make([][]float32, 0, len(texts))
	for range texts {
		vectors = append(vectors, []float32{0.1, 0.2, 0.3})
	}
	return aiprovider.EmbeddingResult{Vectors: vectors, Model: "embed-3", Dimensions: 3}, nil
}

type oneProvider struct{}

func (oneProvider) For(context.Context, appshared.ActorContext) (aiprovider.Provider, error) {
	return batchProvider{}, nil
}

func embeddingFor(rows, batch int) AiEmbedding {
	return AiEmbedding{
		Embed: work.EmbedItems{
			Embeddings: owedStore{rows: rows}, Providers: oneProvider{}, Semantic: presentStore{},
			UnitOfWork: passthroughUnitOfWork{}, Clock: clock.Fixed(time.Now()), BatchSize: batch,
		},
		Interval: time.Hour, Continuation: time.Second,
	}
}

func TestAnEmbeddingPassWithoutATenantIsRefused(t *testing.T) {
	_, err := embeddingFor(0, 10).Run(t.Context(), queue.Job{Kind: queue.KindAiEmbed})

	if !errors.Is(err, shared.ErrInternal) {
		t.Errorf("a tenantless pass reported %v, want an internal error", err)
	}
}

// The job is never finished for good, and that is what "catches up after an outage without a
// restart" means concretely: a pass that filled its batch comes straight back, so a workspace
// that fell a thousand entries behind while a provider was down works through them by itself -
// and a quiet one waits out the long interval rather than removing its own row.
func TestTheEmbeddingPassAlwaysComesBack(t *testing.T) {
	for _, c := range []struct {
		name  string
		rows  int
		batch int
		after time.Duration
	}{
		{"nothing is owed", 0, 10, time.Hour},
		{"the workspace is up to date bar one entry", 1, 10, time.Hour},
		{"the batch was full, so there is known work left", 10, 10, time.Second},
	} {
		t.Run(c.name, func(t *testing.T) {
			result, err := embeddingFor(c.rows, c.batch).Run(
				t.Context(), queue.Job{Kind: queue.KindAiEmbed, TenantID: embedTenant})
			if err != nil {
				t.Fatalf("the pass failed: %v", err)
			}

			if !result.Repeat {
				t.Fatal("the embedding pass finished for good")
			}
			if result.RepeatAfter != c.after {
				t.Errorf("it comes back after %v, want %v", result.RepeatAfter, c.after)
			}
		})
	}
}

// The pass acts for the workspace rather than for a person: an embedding is the installation's own
// bookkeeping about entries that already exist, which is also why the read it performs is not
// narrowed to anybody's visibility.
func TestTheEmbeddingPassActsForTheWorkspaceAndNobodyElse(t *testing.T) {
	seen := &actorRecorder{}
	handler := embeddingFor(1, 10)
	handler.Embed.Providers = seen

	if _, err := handler.Run(
		t.Context(), queue.Job{Kind: queue.KindAiEmbed, TenantID: embedTenant}); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if seen.actor.Kind != appshared.ActorSystem {
		t.Errorf("the pass acted as %q, want the system", seen.actor.Kind)
	}
	if seen.actor.TenantID != embedTenant {
		t.Errorf("the pass acted for %s, want the workspace the job named", seen.actor.TenantID)
	}
	if !seen.actor.AccountID.IsZero() {
		t.Errorf("the pass acted as account %s, want nobody", seen.actor.AccountID)
	}
}

type actorRecorder struct{ actor appshared.ActorContext }

func (r *actorRecorder) For(
	_ context.Context, actor appshared.ActorContext,
) (aiprovider.Provider, error) {
	r.actor = actor
	return batchProvider{}, nil
}
