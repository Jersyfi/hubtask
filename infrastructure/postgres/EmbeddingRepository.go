// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Semantic search's store (J-09, J-10, ADR-0050). No method takes a tenant: row level security
// bounds every statement (ADR-0010).
//
// **The one repository in this package whose SQL is not generated**, and ADR-0050 decision 5 says
// why: sqlc reads `db/migrations`, and `item_embedding` is created inside a `DO` block because
// pgvector is detected rather than demanded - so it is invisible to the generator. That is a
// consequence of the capability being conditional, not a shortcut.
//
// What rule 9 still holds here, and what the statements below are written to keep: every value is
// **bound**, and nothing a caller sent is ever concatenated into SQL text. The one string built at
// run time is the vector literal, which is assembled from float32s this process produced - never
// from anything that arrived in a request.
type EmbeddingRepository struct{}

func NewEmbeddingRepository() EmbeddingRepository { return EmbeddingRepository{} }

var _ repository.Embeddings = EmbeddingRepository{}

// The three statements, as constants. Constants rather than builders, because there is nothing
// about them to vary: what changes between calls is the parameters.
const (
	owedEmbeddings = `
		SELECT wi.id, wi.title, coalesce(wi.notes, '')
		FROM work_item wi
		LEFT JOIN item_embedding e ON e.tenant_id = wi.tenant_id AND e.item_id = wi.id
		WHERE wi.deleted_at IS NULL
		  AND (e.item_id IS NULL OR e.model <> $1 OR e.source_digest <> digest_of(wi.title, coalesce(wi.notes, '')))
		ORDER BY wi.updated_at DESC NULLS LAST, wi.id
		LIMIT $2`

	countMissingEmbeddings = `
		SELECT count(*)::bigint FROM (
		  SELECT 1 FROM work_item wi
		  LEFT JOIN item_embedding e ON e.tenant_id = wi.tenant_id AND e.item_id = wi.id
		  WHERE wi.deleted_at IS NULL AND e.item_id IS NULL
		  LIMIT $1
		) AS owed`

	// The neighbourhood of one entry (K-04). A query and a threshold: nothing is asked of a
	// provider, because the vectors are the ones J-10's pass already wrote.
	//
	// Four things it refuses to call a duplicate, and each is a line rather than a comment
	// afterwards. The entry itself, which is trivially its own nearest neighbour. A row from
	// another *model*, because vectors are comparable only inside one model's space and ranking
	// the mixture would rank by nothing. Anything on the entry's own branch - its ancestors and
	// its descendants - because a work package is not a duplicate of the task it sits in. And a
	// deleted entry, which is not a duplicate of anything.
	//
	// The tenant is nobody's parameter here either: row level security bounds both sides of the
	// comparison (ADR-0010), so `me` and the candidates are one workspace's by construction.
	nearEmbeddings = `
		WITH me AS (
		  SELECT e.embedding, e.model, wi.path
		  FROM item_embedding e
		  JOIN work_item wi ON wi.tenant_id = e.tenant_id AND wi.id = e.item_id
		  WHERE e.item_id = $1
		)
		SELECT wi.id, wi.collection_id, c.parent_id,
		       (1 - (e.embedding <=> me.embedding))::float8 AS similarity
		FROM item_embedding e
		JOIN work_item wi ON wi.tenant_id = e.tenant_id AND wi.id = e.item_id
		JOIN container c ON c.tenant_id = wi.tenant_id AND c.id = wi.collection_id
		CROSS JOIN me
		WHERE e.item_id <> $1
		  AND e.model = me.model
		  AND wi.deleted_at IS NULL
		  AND NOT starts_with(wi.path, me.path)
		  AND NOT starts_with(me.path, wi.path)
		  AND (1 - (e.embedding <=> me.embedding)) >= $2
		ORDER BY e.embedding <=> me.embedding, wi.id
		LIMIT $3`

	// What the entry's own vector was made with, which is the proposal's provenance and the answer
	// to "has the pass reached this entry at all".
	embeddingModelOf = `SELECT model FROM item_embedding WHERE item_id = $1`

	storeEmbedding = `
		INSERT INTO item_embedding (tenant_id, item_id, model, embedding, source_digest, updated_at)
		VALUES (current_tenant_id(), $1, $2, $3::vector, $4, $5)
		ON CONFLICT (tenant_id, item_id) DO UPDATE SET
		  model         = excluded.model,
		  embedding     = excluded.embedding,
		  source_digest = excluded.source_digest,
		  updated_at    = excluded.updated_at`
)

// Owed answers a batch of entries whose vectors are missing or stale.
//
// Staleness is decided in the database rather than in Go, and that is what the `digest_of` function
// is for: the alternative is reading every entry's text into the process to fingerprint it, which
// is the whole table crossing the wire to find the handful of rows that moved.
func (EmbeddingRepository) Owed(
	ctx context.Context, model string, batch int,
) ([]repository.OwedEmbedding, error) {
	tx, err := FromContext(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, owedEmbeddings, model, batch)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading what needs embedding: %w", err))
	}
	defer rows.Close()

	var owed []repository.OwedEmbedding
	for rows.Next() {
		var (
			id           string
			title, notes string
		)
		if err := rows.Scan(&id, &title, &notes); err != nil {
			return nil, shared.Internalf("reading an owed embedding: %w", err)
		}
		itemID, err := shared.ParseID(id)
		if err != nil {
			return nil, err
		}
		owed = append(owed, repository.OwedEmbedding{ItemID: itemID, Title: title, Notes: notes})
	}
	if err := rows.Err(); err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading what needs embedding: %w", err))
	}
	return owed, nil
}

// CountMissing reports how many entries have no vector at all.
func (EmbeddingRepository) CountMissing(ctx context.Context, ceiling int) (int, error) {
	tx, err := FromContext(ctx)
	if err != nil {
		return 0, err
	}

	var missing int64
	if err := tx.QueryRow(ctx, countMissingEmbeddings, ceiling).Scan(&missing); err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting entries without an embedding: %w", err))
	}
	return int(missing), nil
}

// Near answers the entries closest to one entry, nearest first.
//
// Two statements rather than one, and the first is the cheap one: an entry the embedding pass has
// not reached has no model, and asking for its neighbours would be asking the index to compare
// nothing. It is also what tells a proposal which model to record.
func (EmbeddingRepository) Near(
	ctx context.Context, itemID shared.ID, floor float64, limit int,
) (repository.Nearby, error) {
	tx, err := FromContext(ctx)
	if err != nil {
		return repository.Nearby{}, err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return repository.Nearby{}, err
	}

	var model string
	switch err := tx.QueryRow(ctx, embeddingModelOf, id).Scan(&model); {
	case errors.Is(err, pgx.ErrNoRows):
		// Not embedded yet, or not embedded at all. Not an error: the entry is found by nobody and
		// finds nobody until the pass reaches it (J-10).
		return repository.Nearby{}, nil
	case err != nil:
		return repository.Nearby{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading an entry's embedding model: %w", err))
	}

	rows, err := tx.Query(ctx, nearEmbeddings, id, floor, limit)
	if err != nil {
		return repository.Nearby{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading an entry's neighbours: %w", err))
	}
	defer rows.Close()

	near := repository.Nearby{Embedded: true, Model: model}
	for rows.Next() {
		var (
			candidate, collection string
			hub                   *string
			similarity            float64
		)
		if err := rows.Scan(&candidate, &collection, &hub, &similarity); err != nil {
			return repository.Nearby{}, shared.Internalf("reading a neighbour: %w", err)
		}

		neighbour := repository.Neighbour{Similarity: similarity}
		if neighbour.ItemID, err = shared.ParseID(candidate); err != nil {
			return repository.Nearby{}, err
		}
		if neighbour.CollectionID, err = shared.ParseID(collection); err != nil {
			return repository.Nearby{}, err
		}
		if hub != nil {
			if neighbour.HubID, err = shared.ParseID(*hub); err != nil {
				return repository.Nearby{}, err
			}
		}
		near.Candidates = append(near.Candidates, neighbour)
	}
	if err := rows.Err(); err != nil {
		return repository.Nearby{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading an entry's neighbours: %w", err))
	}
	return near, nil
}

// Store writes one entry's vector.
func (EmbeddingRepository) Store(
	ctx context.Context, embedding repository.StoredEmbedding,
) error {
	tx, err := FromContext(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(embedding.ItemID)
	if err != nil {
		return err
	}
	// The application layer refused this already; refused a second time here so a caller that
	// bypassed it could not reach the column either (ADR-0054).
	if len(embedding.Vector) > repository.EmbeddingWidth {
		return repository.EmbeddingTooWide(embedding.Model, len(embedding.Vector))
	}
	// And an empty one, which the column used to refuse for us: padded, it would be a vector of
	// zeros, whose cosine distance to everything is NaN - and NaN sorts first under `ORDER BY
	// rank DESC`, so one such row would head every search it lexically matched.
	if len(embedding.Vector) == 0 {
		return repository.EmbeddingEmpty(embedding.Model)
	}

	if _, err := tx.Exec(ctx, storeEmbedding,
		id, embedding.Model, vectorLiteral(embedding.Vector),
		embedding.SourceDigest, embedding.UpdatedAt,
	); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("storing an embedding: %w", err))
	}
	return nil
}

// vectorLiteral renders a vector the way pgvector's text input expects it, at the index's width.
//
// Built at run time and bound as a parameter, which is the distinction that matters: it is
// assembled from float32s this process produced - never from anything that arrived in a request -
// and it reaches the statement through `$3` rather than through the SQL text (rule 9, T-06).
//
// A vector narrower than the index is padded with zeros to its width (ADR-0054). Exact for cosine,
// the only distance the product uses: the dot product and both norms are unchanged by trailing
// zeros, so every distance the index is built on and every similarity a floor is compared against
// is the number the model produced. Here rather than at either caller, because the stored vector
// and the query vector both come through this function, and padding one side and not the other
// would be a search comparing two geometries.
//
// A wider one is not truncated - that is exact only for models trained for it - and does not
// reach this function: the store and the search both refuse it first.
func vectorLiteral(values []float32) string {
	var b strings.Builder
	b.Grow(repository.EmbeddingWidth*12 + 2)
	b.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	for i := len(values); i < repository.EmbeddingWidth; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('0')
	}
	b.WriteByte(']')
	return b.String()
}

// vectorLiteralOrEmpty renders a query vector, or nothing where the search is lexical.
func vectorLiteralOrEmpty(values []float32) string {
	if len(values) == 0 {
		return ""
	}
	return vectorLiteral(values)
}
