// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

	if _, err := tx.Exec(ctx, storeEmbedding,
		id, embedding.Model, vectorLiteral(embedding.Vector),
		embedding.SourceDigest, embedding.UpdatedAt,
	); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("storing an embedding: %w", err))
	}
	return nil
}

// vectorLiteral renders a vector the way pgvector's text input expects it.
//
// Built at run time and bound as a parameter, which is the distinction that matters: it is
// assembled from float32s this process produced - never from anything that arrived in a request -
// and it reaches the statement through `$3` rather than through the SQL text (rule 9, T-06).
func vectorLiteral(values []float32) string {
	var b strings.Builder
	b.Grow(len(values)*12 + 2)
	b.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
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
