// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// EmbeddingWidth is the geometry of the index: the number of dimensions the store holds, and the
// most a model may produce (ADR-0054).
//
// A vector narrower than this is zero-padded to it before it is stored or compared - exact for
// cosine, which is the only distance the product uses, since padding changes neither the dot
// product nor either norm. A vector wider than this is refused, because truncating one is exact
// only for models trained for it and silently wrong for every other.
//
// One number rather than a question the store answers, because it is a contract rather than a
// measurement: the column is built for it, and an integration test holds the column to it. That
// test is what makes a constant that mirrors a migration honest.
const EmbeddingWidth = 1536

// embeddingTooWide is the detail code for a vector the index cannot hold.
const embeddingTooWide = "ai.embedding_too_wide"

// EmbeddingTooWide is the refusal for a vector wider than the index, naming the model, its width
// and the index's. A validation error rather than an internal one: the model is a configuration
// somebody chose, and the answer tells them what to choose instead.
func EmbeddingTooWide(model string, dimensions int) error {
	return shared.ErrValidation.WithDetail(embeddingTooWide).WithParams(map[string]string{
		"model":      model,
		"dimensions": strconv.Itoa(dimensions),
		"width":      strconv.Itoa(EmbeddingWidth),
	})
}

// EmbeddingEmpty is the refusal for a vector with no dimensions at all. Refused before the store
// rather than by it, because padded to the index it would be a vector of zeros, and a zero vector's
// cosine distance to everything is NaN - a value that sorts first. A provider defect rather than a
// configuration, so an internal error.
func EmbeddingEmpty(model string) error {
	return shared.ErrInternal.WithDetail("ai.embedding_empty").
		WithParams(map[string]string{"model": model})
}

// IsEmbeddingTooWide reports whether an error is that refusal, so a caller can finish rather than
// retry: a model's width does not change on the next attempt. By detail code, because `Is` on the
// typed error compares the category and would match every validation error.
func IsEmbeddingTooWide(err error) bool {
	return errors.Is(err, shared.ErrValidation) && shared.AsError(err).DetailCode == embeddingTooWide
}

// OwedEmbedding is one entry that has no vector, or whose vector was made from text that has
// since changed.
type OwedEmbedding struct {
	ItemID shared.ID
	Title  string
	Notes  string
}

// StoredEmbedding is one vector on its way into the store.
type StoredEmbedding struct {
	ItemID shared.ID
	// Model is what produced it. Vectors are only comparable within one model's space, so a row
	// that does not name its model is one nothing can decide about after a reconfiguration.
	Model  string
	Vector []float32
	// SourceDigest fingerprints the text the vector was made from, so the next pass can tell an
	// entry whose text moved from one whose text did not.
	SourceDigest []byte
	UpdatedAt    time.Time
}

// Embeddings is semantic search's store (J-09, J-10, ADR-0050).
//
// **Every method may be called against a table that does not exist.** pgvector is detected rather
// than demanded, so an installation without it has no store at all - and the thing that stops these
// being called there is the capability, asked once and answered in one place, rather than each
// method guessing. An adapter that reached the missing table answers an ordinary database error,
// which is the honest failure for a call that should not have happened.
type Embeddings interface {
	// Owed answers a batch of entries whose vectors are missing or stale, newest first. `digestOf`
	// is the fingerprint the caller would compute for each entry's current text, and `model` the
	// one it would use - both passed in, because what makes a vector stale is a comparison between
	// what is stored and what the caller would store now.
	Owed(ctx context.Context, model string, batch int) ([]OwedEmbedding, error)

	// CountMissing reports how many entries have no vector at all, counted no higher than the
	// ceiling. For the pass's own log rather than for a decision.
	CountMissing(ctx context.Context, ceiling int) (int, error)

	// Store writes one entry's vector, replacing whatever was there.
	Store(ctx context.Context, embedding StoredEmbedding) error

	// Near answers the entries closest to one entry in the embedding space, nearest first (K-04).
	//
	// `floor` is the similarity below which two entries are not near each other, and `limit`
	// bounds the answer. What comes back is candidates rather than results: the rows come from
	// everywhere in the workspace at once, so *which of them the caller may see* is a question the
	// application layer asks afterwards, exactly as it does for a search (rule 2, SearchItems).
	//
	// An entry with no vector answers `Embedded: false` and no candidates, which is not an error:
	// the pass has not reached it, it is found by nobody and finds nobody until it has, and that
	// is J-10's degradation rather than a gap.
	Near(ctx context.Context, itemID shared.ID, floor float64, limit int) (Nearby, error)
}

// Nearby is what one entry's neighbourhood looks like.
type Nearby struct {
	// Embedded says the entry itself has a vector. False means there is nothing to compare, and
	// the answer is no suggestion rather than an empty one.
	Embedded bool
	// Model is the embedding model that produced the entry's own vector. It travels because it is
	// the proposal's provenance: a similarity means nothing outside one model's space, and only
	// rows produced by the same model are compared at all.
	Model string
	// Candidates are the neighbours above the floor, nearest first, before the caller narrows them
	// to what the actor may read.
	Candidates []Neighbour
}

// Neighbour is one entry near another, with the path the permission question is asked against.
type Neighbour struct {
	ItemID       shared.ID
	CollectionID shared.ID
	// HubID is the collection's parent, or zero for a collection that sits at the top level. It
	// travels for the reason a search hit carries one: a membership held at a hub applies
	// downwards, and a path that named only the collection could not show an entry to somebody
	// whose right sits above it.
	HubID shared.ID
	// Similarity is 1 minus the cosine distance: 1 is the same direction, 0 is unrelated. It is
	// the number the floor is compared against and the one a person reads as "how alike".
	Similarity float64
}
