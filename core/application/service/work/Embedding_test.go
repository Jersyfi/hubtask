// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The embedding pass, at the level this layer owns: which entries it asks the provider about, what
// it writes back, and - the half that matters most - the three ways of having nothing to do, none
// of which is a failure. Whether the statement behind `Owed` picks the right rows is the database's
// to answer, and is asked of a real one in test/integration.

// embeddingWorld is the store, the provider and the capability in one place, because a pass is only
// interesting as the conversation between the three.
type embeddingWorld struct {
	owed      []repository.OwedEmbedding
	stored    []repository.StoredEmbedding
	available bool
	embedding bool
	model     string
	// answeredModel is what the provider says answered, which is deliberately not what is stored.
	answeredModel string
	vectors       [][]float32
	embedded      [][]string
	askedModel    []string
	embedErr      error
	storeErr      error
	// watcher, where one is set, says whether a transaction is open while the provider is called.
	watcher        *transactionWatcher
	embeddedInside bool
}

// transactionWatcher is the unit of work with one extra fact: whether a transaction is open right
// now. What it exists to prove is that the provider call is not inside one.
type transactionWatcher struct {
	unitOfWork
	open int
}

func (u *transactionWatcher) Within(
	ctx context.Context, s persistence.Scope, fn func(context.Context) error,
) error {
	u.open++
	defer func() { u.open-- }()
	return u.unitOfWork.Within(ctx, s, fn)
}

func (u *transactionWatcher) WithinReadOnly(
	ctx context.Context, s persistence.Scope, fn func(context.Context) error,
) error {
	u.open++
	defer func() { u.open-- }()
	return u.unitOfWork.WithinReadOnly(ctx, s, fn)
}

func embeddingHarness(world *embeddingWorld) EmbedItems {
	return EmbedItems{
		Embeddings: world, Providers: world, Semantic: world,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

func (w *embeddingWorld) Owed(_ context.Context, model string, batch int) ([]repository.OwedEmbedding, error) {
	w.askedModel = append(w.askedModel, model)
	if len(w.owed) > batch {
		return w.owed[:batch], nil
	}
	return w.owed, nil
}

func (w *embeddingWorld) CountMissing(context.Context, int) (int, error) { return len(w.owed), nil }

func (w *embeddingWorld) Store(_ context.Context, embedding repository.StoredEmbedding) error {
	if w.storeErr != nil {
		return w.storeErr
	}
	w.stored = append(w.stored, embedding)
	return nil
}

func (w *embeddingWorld) Available(context.Context) (bool, error) { return w.available, nil }

func (w *embeddingWorld) For(context.Context, appshared.ActorContext) (aiprovider.Provider, error) {
	return embeddingProvider{world: w}, nil
}

type embeddingProvider struct{ world *embeddingWorld }

func (p embeddingProvider) Capabilities() aiprovider.ProviderCapabilities {
	return aiprovider.ProviderCapabilities{
		Kind: "stub", Embedding: p.world.embedding, EmbeddingModel: p.world.model,
		EmbeddingDimensions: 3,
	}
}

func (p embeddingProvider) Complete(
	context.Context, aiprovider.CompletionRequest,
) (aiprovider.CompletionResult, error) {
	return aiprovider.CompletionResult{}, aiprovider.ErrUnavailable
}

func (p embeddingProvider) Embed(
	_ context.Context, texts []string,
) (aiprovider.EmbeddingResult, error) {
	p.world.embedded = append(p.world.embedded, texts)
	if p.world.watcher != nil && p.world.watcher.open > 0 {
		p.world.embeddedInside = true
	}
	if p.world.embedErr != nil {
		return aiprovider.EmbeddingResult{}, p.world.embedErr
	}
	return aiprovider.EmbeddingResult{
		Vectors: p.world.vectors, Model: p.world.answeredModel, Dimensions: 3,
	}, nil
}

func owedFixture(id shared.ID, title, notes string) repository.OwedEmbedding {
	return repository.OwedEmbedding{ItemID: id, Title: title, Notes: notes}
}

func embeddingActor() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantID}
}

// The ordinary pass: what is owed goes to the provider, what comes back is stored against the
// fingerprint of the text it was made from - which is what makes the next pass skip it.
func TestAnEmbeddingPassStoresWhatTheProviderAnswered(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3",
		owed:    []repository.OwedEmbedding{owedFixture(taskID, "Quarterly report", "due friday")},
		vectors: [][]float32{{0.1, 0.2, 0.3}},
	}

	outcome, err := embeddingHarness(world).Execute(t.Context(), embeddingActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if outcome.Embedded != 1 || outcome.Exhausted {
		t.Errorf("outcome = %+v, want one embedded and no continuation", outcome)
	}
	if len(world.embedded) != 1 || len(world.embedded[0]) != 1 {
		t.Fatalf("the provider was asked %v", world.embedded)
	}
	if want := "Quarterly report\n\ndue friday"; world.embedded[0][0] != want {
		t.Errorf("the provider was sent %q, want %q", world.embedded[0][0], want)
	}
	if len(world.stored) != 1 {
		t.Fatalf("%d vectors were stored, want one", len(world.stored))
	}
	stored := world.stored[0]
	if stored.ItemID != taskID || len(stored.Vector) != 3 || !stored.UpdatedAt.Equal(now) {
		t.Errorf("the stored vector is %+v", stored)
	}
	digest := suggestion.Digest("Quarterly report", "due friday")
	if string(stored.SourceDigest) != string(digest) {
		t.Error("the vector was stored against a fingerprint of something other than its own text")
	}
}

// The model a vector is stored under is the one `Owed` asked with, not the one the provider says
// answered. A provider that resolves an alias would otherwise make every row it just wrote stale
// again, and the pass would embed the same batch until somebody noticed the bill.
func TestTheStoredModelIsTheOneStalenessIsJudgedAgainst(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3", answeredModel: "embed-3-2026-04-01",
		owed:    []repository.OwedEmbedding{owedFixture(taskID, "Quarterly report", "")},
		vectors: [][]float32{{0.1, 0.2, 0.3}},
	}

	if _, err := embeddingHarness(world).Execute(t.Context(), embeddingActor()); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if len(world.askedModel) != 1 || world.askedModel[0] != "embed-3" {
		t.Fatalf("staleness was judged against %v", world.askedModel)
	}
	if world.stored[0].Model != world.askedModel[0] {
		t.Errorf("the vector was stored under %q but staleness is judged against %q - the pass "+
			"would embed this entry again for ever", world.stored[0].Model, world.askedModel[0])
	}
}

// An entry with no notes is sent as its title alone, rather than as a title with two empty lines
// after it: the fingerprint is taken over the same two fields, and a separator that appeared in one
// and not the other would make every noteless entry permanently stale.
func TestAnEntryWithoutNotesIsSentAsItsTitle(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3",
		owed:    []repository.OwedEmbedding{owedFixture(taskID, "Quarterly report", "")},
		vectors: [][]float32{{0.1, 0.2, 0.3}},
	}

	if _, err := embeddingHarness(world).Execute(t.Context(), embeddingActor()); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if world.embedded[0][0] != "Quarterly report" {
		t.Errorf("the provider was sent %q", world.embedded[0][0])
	}
}

// The three ways of having nothing to do. All three are a pass that did nothing and said so, not a
// failure: semantic search is optional at every one of those levels, and a handler that treated any
// of them as an error would fill a dead letter queue with a feature somebody switched off.
func TestAnEmbeddingPassWithNothingToDoIsNotAFailure(t *testing.T) {
	for _, c := range []struct {
		name  string
		world *embeddingWorld
		asked bool
	}{
		{
			name:  "no embedding store",
			world: &embeddingWorld{embedding: true, model: "embed-3", owed: []repository.OwedEmbedding{owedFixture(taskID, "Quarterly report", "")}},
		},
		{
			name:  "a provider that cannot embed",
			world: &embeddingWorld{available: true, owed: []repository.OwedEmbedding{owedFixture(taskID, "Quarterly report", "")}},
		},
		{
			name:  "nothing owed",
			world: &embeddingWorld{available: true, embedding: true, model: "embed-3"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			outcome, err := embeddingHarness(c.world).Execute(t.Context(), embeddingActor())
			if err != nil {
				t.Fatalf("a pass with nothing to do failed: %v", err)
			}
			if outcome.Embedded != 0 || outcome.Exhausted {
				t.Errorf("outcome = %+v, want nothing done", outcome)
			}
			if len(c.world.embedded) != 0 {
				t.Errorf("a provider was called anyway: %v", c.world.embedded)
			}
			if len(c.world.stored) != 0 {
				t.Errorf("%d vectors were stored anyway", len(c.world.stored))
			}
		})
	}
}

// A full batch says so, which is what makes the job come straight back rather than wait out its
// interval - that is how an installation that has just switched AI on catches up without a restart.
func TestAFullBatchAsksToBeCalledAgain(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3",
		owed: []repository.OwedEmbedding{
			owedFixture(taskID, "Quarterly report", ""),
			owedFixture(packageID, "Shopping", ""),
		},
		vectors: [][]float32{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}},
	}

	handler := embeddingHarness(world)
	handler.BatchSize = 2

	outcome, err := handler.Execute(t.Context(), embeddingActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if outcome.Embedded != 2 || !outcome.Exhausted {
		t.Errorf("outcome = %+v, want a full batch that asks to continue", outcome)
	}
}

// A provider that answers a different number of vectors than it was sent texts is refused rather
// than aligned by position: attaching one entry's meaning to another entry's row is the one mistake
// that looks like a working search.
func TestAMisalignedBatchIsRefusedRatherThanStored(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3",
		owed: []repository.OwedEmbedding{
			owedFixture(taskID, "Quarterly report", ""),
			owedFixture(packageID, "Shopping", ""),
		},
		vectors: [][]float32{{0.1, 0.2, 0.3}},
	}

	if _, err := embeddingHarness(world).Execute(t.Context(), embeddingActor()); err == nil {
		t.Fatal("a batch of two entries and one vector was stored")
	}
	if len(world.stored) != 0 {
		t.Errorf("%d vectors were stored from a misaligned batch", len(world.stored))
	}
}

// A provider that refuses stops the pass, and the job retries it. It is deliberately not swallowed
// the way the *search's* provider call is: a search has a complete lexical answer to fall back on,
// and a pass that reported success while embedding nothing would leave the workspace permanently
// behind with nothing saying so.
func TestAProviderThatRefusesFailsThePass(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3",
		owed:     []repository.OwedEmbedding{owedFixture(taskID, "Quarterly report", "")},
		embedErr: aiprovider.ErrUnavailable,
	}

	_, err := embeddingHarness(world).Execute(t.Context(), embeddingActor())
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("a cut-off provider answered %v", err)
	}
	if len(world.stored) != 0 {
		t.Errorf("%d vectors were stored after the provider refused", len(world.stored))
	}
}

// The provider is called outside the transaction, because it reaches somebody else's machine and a
// transaction waiting on one holds a connection for as long as they feel like taking.
func TestTheProviderIsNotCalledInsideATransaction(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3",
		owed:    []repository.OwedEmbedding{owedFixture(taskID, "Quarterly report", "")},
		vectors: [][]float32{{0.1, 0.2, 0.3}},
	}
	handler := embeddingHarness(world)
	uow := &transactionWatcher{}
	handler.UnitOfWork = uow
	world.watcher = uow

	if _, err := handler.Execute(t.Context(), embeddingActor()); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if world.embeddedInside {
		t.Error("the provider was called with a transaction open")
	}
	if uow.writes != 1 || uow.reads != 1 {
		t.Errorf("the pass opened %d writes and %d reads, want one of each", uow.writes, uow.reads)
	}
}
