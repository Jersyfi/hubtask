// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// What AI proposed (J-05). No method takes a tenant: row level security bounds every statement
// (ADR-0010).
type SuggestionRepository struct {
	cursors security.CursorCodec
}

func NewSuggestionRepository(cursors security.CursorCodec) SuggestionRepository {
	return SuggestionRepository{cursors: cursors}
}

var (
	_ repository.Suggestions = SuggestionRepository{}
	_ repository.Expiring    = SuggestionRepository{}
	_ repository.Requests    = SuggestionRepository{}
)

func (r SuggestionRepository) Record(ctx context.Context, proposal domain.Suggestion) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(proposal.ID)
	if err != nil {
		return err
	}
	target, err := uuidOf(proposal.TargetID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(proposal.Payload)
	if err != nil {
		return shared.Internalf("encoding a suggestion's payload: %w", err)
	}

	if err := queries.RecordSuggestion(ctx, sqlc.RecordSuggestionParams{
		ID: id, TargetType: string(proposal.TargetType), TargetID: target,
		Kind: string(proposal.Kind), Payload: payload,
		Source: string(proposal.Source), Model: proposal.Model,
		PromptID: proposal.PromptID, PromptVersion: proposal.PromptVersion,
		ProducedAt: timestampOf(proposal.ProducedAt), InputDigest: proposal.InputDigest,
		CreatedAt: timestampOf(proposal.CreatedAt),
		//nolint:gosec // G115: a count of nodes in one answer, bounded by the tree the narrowing kept
		DroppedNodes: int32(proposal.DroppedNodes),
	}); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording a suggestion: %w", err))
	}
	return nil
}

func (r SuggestionRepository) Find(
	ctx context.Context, id shared.ID,
) (domain.Suggestion, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Suggestion{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Suggestion{}, err
	}

	row, err := queries.FindSuggestion(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.Suggestion{}, shared.ErrNotFound.WithDetail("suggestions.not_found")
		}
		return domain.Suggestion{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a suggestion: %w", err))
	}
	return suggestionFrom(
		row.ID, row.TargetType, row.TargetID, row.Kind, row.Status, row.Payload,
		row.Source, row.Model, row.PromptID, row.PromptVersion, row.ProducedAt,
		row.InputDigest, row.CreatedAt, row.DecidedAt, row.DecidedBy, row.Version, row.DroppedNodes,
	)
}

func (r SuggestionRepository) List(
	ctx context.Context, query repository.Query,
) (repository.Page, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Page{}, err
	}
	target, err := uuidOf(query.TargetID)
	if err != nil {
		return repository.Page{}, err
	}
	boundary, err := suggestionCursor(r.cursors, query.Cursor)
	if err != nil {
		return repository.Page{}, err
	}

	params := sqlc.ListSuggestionsParams{
		TargetType:      string(query.TargetType),
		TargetID:        target,
		CursorCreatedAt: boundary.createdAt,
		CursorID:        boundary.id,
		PageSize:        pageProbe(query.Size),
	}
	if query.Status != "" {
		status := string(query.Status)
		params.Status = &status
	}

	rows, err := queries.ListSuggestions(ctx, params)
	if err != nil {
		return repository.Page{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the suggestions of %s: %w", query.TargetID, err))
	}

	proposals := make([]domain.Suggestion, 0, len(rows))
	for _, row := range rows {
		proposal, err := suggestionFrom(
			row.ID, row.TargetType, row.TargetID, row.Kind, row.Status, row.Payload,
			row.Source, row.Model, row.PromptID, row.PromptVersion, row.ProducedAt,
			row.InputDigest, row.CreatedAt, row.DecidedAt, row.DecidedBy, row.Version, row.DroppedNodes,
		)
		if err != nil {
			return repository.Page{}, err
		}
		proposals = append(proposals, proposal)
	}

	kept, info := pageOf(proposals, query.Size, r.cursors,
		func(proposal domain.Suggestion) security.Position {
			return security.At(proposal.CreatedAt.UTC().Format(time.RFC3339Nano), proposal.ID)
		})
	return repository.Page{Items: kept, NextCursor: info.NextCursor, HasMore: info.HasMore}, nil
}

func (r SuggestionRepository) Decide(
	ctx context.Context, decided domain.Suggestion, expectedVersion int,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(decided.ID)
	if err != nil {
		return false, err
	}
	by, err := uuidOf(decided.DecidedBy)
	if err != nil {
		return false, err
	}

	written, err := queries.DecideSuggestion(ctx, sqlc.DecideSuggestionParams{
		ID: id, Status: string(decided.Status),
		DecidedAt: timestampOf(decided.DecidedAt), DecidedBy: by,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deciding a suggestion: %w", err))
	}
	return written > 0, nil
}

func (r SuggestionRepository) DeleteExpired(
	ctx context.Context, cutoff time.Time, batch int,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	removed, err := queries.DeleteExpiredSuggestions(ctx, sqlc.DeleteExpiredSuggestionsParams{
		//nolint:gosec // G115: a retention batch size, bounded small by the purger's configuration
		Cutoff: timestampOf(cutoff), Batch: int32(batch),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing expired suggestions: %w", err))
	}
	// The requests a job left behind go with the same sweep (P-11): the job that reads one
	// deletes it when it ends, and this is the net under a job that never did. Not counted - the
	// number is the suggestions', and a request is not a proposal.
	if _, err := queries.DeleteExpiredAiRequests(ctx, sqlc.DeleteExpiredAiRequestsParams{
		//nolint:gosec // G115: the same bounded batch size
		Cutoff: timestampOf(cutoff), Batch: int32(batch),
	}); err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing expired requests: %w", err))
	}
	return int(removed), nil
}

// Put holds what a person asked a template to be drafted from (P-11), for the job that will read it.
func (r SuggestionRepository) Put(ctx context.Context, request repository.Request) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(request.ID)
	if err != nil {
		return err
	}
	askedBy, err := uuidOf(request.AskedBy)
	if err != nil {
		return err
	}
	if err := queries.PutAiRequest(ctx, sqlc.PutAiRequestParams{
		ID: id, AskedBy: askedBy, Text: request.Text, CreatedAt: timestampOf(request.CreatedAt),
	}); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("holding a request: %w", err))
	}
	return nil
}

// Get answers one request, or an error wrapping shared.ErrNotFound.
func (r SuggestionRepository) Get(ctx context.Context, id shared.ID) (repository.Request, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Request{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return repository.Request{}, err
	}
	row, err := queries.FindAiRequest(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return repository.Request{}, shared.ErrNotFound.WithDetail("suggestions.request_not_found")
		}
		return repository.Request{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a request: %w", err))
	}
	requestID, err := idFrom(row.ID)
	if err != nil {
		return repository.Request{}, err
	}
	askedBy, err := idFrom(row.AskedBy)
	if err != nil {
		return repository.Request{}, err
	}
	return repository.Request{ID: requestID, AskedBy: askedBy, Text: row.Text, CreatedAt: timeFrom(row.CreatedAt)}, nil
}

// Delete removes a request the job has read. A request already gone is not an error: the job
// that reads it may run twice.
func (r SuggestionRepository) Delete(ctx context.Context, id shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	if _, err := queries.DeleteAiRequest(ctx, key); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing a request: %w", err))
	}
	return nil
}

func (r SuggestionRepository) CountExpired(
	ctx context.Context, cutoff time.Time, ceiling int,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	due, err := queries.CountExpiredSuggestions(ctx, sqlc.CountExpiredSuggestionsParams{
		//nolint:gosec // G115: a counting ceiling, the same bounded batch size
		Cutoff: timestampOf(cutoff), Ceiling: int32(ceiling),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting expired suggestions: %w", err))
	}
	return int(due), nil
}

// suggestionBoundary is a decoded cursor, both fields absent for the first page.
type suggestionBoundary struct {
	createdAt pgtype.Timestamptz
	id        pgtype.UUID
}

func suggestionCursor(
	cursors security.CursorCodec, cursor string,
) (suggestionBoundary, error) {
	if cursor == "" {
		return suggestionBoundary{}, nil
	}
	position, err := cursors.Decode(cursor)
	if err != nil {
		return suggestionBoundary{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, position.SortKey())
	if err != nil {
		return suggestionBoundary{}, shared.ErrValidation.
			WithDetail("shared.cursor_invalid").WithCause(err)
	}
	id, err := uuidOf(position.ID)
	if err != nil {
		return suggestionBoundary{}, err
	}
	return suggestionBoundary{createdAt: timestampOf(createdAt), id: id}, nil
}

// suggestionFrom maps a stored row onto the domain's aggregate. One mapper for both selects, so
// the two cannot disagree about a field.
func suggestionFrom(
	id pgtype.UUID, targetType string, targetID pgtype.UUID, kind, status string,
	payload []byte, source, model, promptID, promptVersion string,
	producedAt pgtype.Timestamptz, digest []byte, createdAt, decidedAt pgtype.Timestamptz,
	decidedBy pgtype.UUID, version int32, droppedNodes int32,
) (domain.Suggestion, error) {
	suggestionID, err := idFrom(id)
	if err != nil {
		return domain.Suggestion{}, err
	}
	target, err := idFrom(targetID)
	if err != nil {
		return domain.Suggestion{}, err
	}

	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		return domain.Suggestion{}, shared.Internalf("decoding a suggestion's payload: %w", err)
	}

	proposal := domain.Suggestion{
		ID: suggestionID, TargetType: domain.TargetType(targetType), TargetID: target,
		Kind: domain.Kind(kind), Status: domain.Status(status), Payload: fields,
		Provenance: domain.Provenance{
			Source: domain.Source(source), Model: model,
			PromptID: promptID, PromptVersion: promptVersion,
			ProducedAt: producedAt.Time.UTC(),
		},
		InputDigest:  digest,
		DroppedNodes: int(droppedNodes),
		CreatedAt:    createdAt.Time.UTC(),
		DecidedAt:    decidedAt.Time.UTC(),
		Version:      int(version),
	}
	if decidedBy.Valid {
		decider, err := idFrom(decidedBy)
		if err != nil {
			return domain.Suggestion{}, err
		}
		proposal.DecidedBy = decider
	}
	if !decidedAt.Valid {
		proposal.DecidedAt = time.Time{}
	}
	return proposal, nil
}
