// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

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
}
