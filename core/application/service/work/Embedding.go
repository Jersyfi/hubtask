// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// AiProviders answers which provider a workspace uses (J-03). An interface here rather than the
// adapter, because the application layer may not import one (ADR-0001).
type AiProviders interface {
	For(ctx context.Context, actor appshared.ActorContext) (aiprovider.Provider, error)
}

// SemanticSearch reports whether this installation has an embedding store at all (J-09, ADR-0050).
type SemanticSearch interface {
	Available(ctx context.Context) (bool, error)
}

// EmbedItems brings one workspace's vectors up to date with its entries (J-10).
//
// Not a use case, and deliberately not in the catalogue: nobody asks for their entries to be
// embedded. It is the pass behind a job, in the shape the retention sweep has - a batch, then a
// decision about whether to come straight back - because what it does is a backlog rather than a
// question.
//
// **Nothing about it is on the write path.** An embedding cannot be maintained by a trigger the way
// `search_document` is, because a trigger cannot make a network call, and a write that waited on a
// provider is precisely what ai-first.md §2 forbids. So an entry whose text just changed is found
// lexically until the pass reaches it, which is the correct degradation rather than a gap: the
// entry is searchable the whole time, by the words somebody would use to find something they just
// wrote.
type EmbedItems struct {
	Embeddings repository.Embeddings
	Providers  AiProviders
	Semantic   SemanticSearch
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// BatchSize bounds one pass. Zero takes the default below.
	BatchSize int
}

// defaultEmbedBatch is how many entries one pass embeds.
//
// Small, and for a reason that is not performance: every entry in a batch is one provider call's
// worth of somebody's content leaving the installation, and a pass that took a thousand at a time
// would make the first run after switching AI on a very large single act. Fifty is a batch a
// provider answers in one request and an operator can see going past.
const defaultEmbedBatch = 50

// EmbedOutcome is what one pass did, for the job that decides whether to come back.
type EmbedOutcome struct {
	// Embedded is how many vectors were written.
	Embedded int
	// Exhausted is true when the pass filled its batch, which means there is probably more owed.
	Exhausted bool
}

// Execute embeds one batch of what the workspace owes.
//
// Three refusals, and all three are "nothing to do" rather than failures: no store, no provider
// that can embed, and nothing owed. Semantic search is optional at every level - the extension, the
// provider, the consent - and a pass that treated any of them as an error would fill a dead letter
// queue with a feature somebody has switched off.
func (h EmbedItems) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (EmbedOutcome, error) {
	batch := h.BatchSize
	if batch <= 0 {
		batch = defaultEmbedBatch
	}

	provider, err := h.Providers.For(ctx, actor)
	if err != nil {
		return EmbedOutcome{}, err
	}
	capabilities := provider.Capabilities()
	if !capabilities.Embedding {
		return EmbedOutcome{}, nil
	}

	var (
		available bool
		owed      []repository.OwedEmbedding
	)
	if err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			var err error
			if available, err = h.Semantic.Available(ctx); err != nil || !available {
				return err
			}
			owed, err = h.Embeddings.Owed(ctx, capabilities.EmbeddingModel, batch)
			return err
		}); err != nil {
		return EmbedOutcome{}, err
	}
	if !available || len(owed) == 0 {
		return EmbedOutcome{}, nil
	}

	// The provider is called outside a transaction, for the reason every provider call in this
	// product is: it reaches somebody else's machine, and a transaction waiting on one holds a
	// connection for as long as they feel like taking (observability-reliability.md §8).
	texts := make([]string, 0, len(owed))
	for _, entry := range owed {
		texts = append(texts, embeddingText(entry))
	}
	answer, err := provider.Embed(ctx, texts)
	if err != nil {
		return EmbedOutcome{}, err
	}
	if len(answer.Vectors) != len(owed) {
		// The adapter already refuses an unalignable batch; this is the same check on the other
		// side of the port, because attaching one entry's meaning to another's row is the one
		// mistake that looks like a working search.
		return EmbedOutcome{}, shared.Internalf(
			"the provider answered %d vectors for %d entries", len(answer.Vectors), len(owed))
	}

	now := h.Clock.Now()
	if err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		for i, entry := range owed {
			if err := h.Embeddings.Store(ctx, repository.StoredEmbedding{
				ItemID: entry.ItemID,
				// The configured model, not the one the provider says answered - even though the
				// second is the more truthful description of the vector. It has to be the name
				// `Owed` asked with, or the pass never converges: `Owed` calls a row stale when
				// its model differs from the configured one, so a provider that resolves an alias
				// and answers under the resolved name would make every row it just wrote stale
				// again, and the job would embed the same fifty entries until somebody noticed the
				// bill. What the column means is therefore "the configuration this vector belongs
				// to", which is what makes a reconfiguration re-embed everything.
				Model:  capabilities.EmbeddingModel,
				Vector: answer.Vectors[i],
				// The same fingerprint the database computes when it decides what is owed, which
				// is what makes the next pass skip this entry until its text moves.
				SourceDigest: suggestion.Digest(entry.Title, entry.Notes),
				UpdatedAt:    now,
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return EmbedOutcome{}, err
	}

	return EmbedOutcome{Embedded: len(owed), Exhausted: len(owed) >= batch}, nil
}

// embeddingText is what an entry looks like to a model: the same two fields the fingerprint is
// taken over, labelled so that a title and a note are not one undifferentiated blob.
func embeddingText(entry repository.OwedEmbedding) string {
	if entry.Notes == "" {
		return entry.Title
	}
	return entry.Title + "\n\n" + entry.Notes
}
