// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// OfferedAiRepository is the models the installation offers (ADR-0072 §1). No method takes a
// tenant: the read policy admits these rows everywhere and the write policy only in the
// installation's own scope, so which transaction a call runs in decides what it may do.
type OfferedAiRepository struct{}

func NewOfferedAiRepository() OfferedAiRepository { return OfferedAiRepository{} }

var _ repository.OfferedAiProviders = OfferedAiRepository{}

func offeredNotFound() error {
	return shared.ErrNotFound.WithDetail("ai.offered_model_unknown")
}

// offeredFrom maps the one row shape every offered statement answers; the generated types are
// identical structs, converted at the call sites.
func offeredFrom(row sqlc.ListOfferedAiProvidersRow) (integration.AiProvider, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return integration.AiProvider{}, err
	}
	return integration.AiProvider{
		ID:              id,
		DisplayName:     row.DisplayName,
		Processor:       row.Processor,
		Kind:            integration.AiProviderKind(row.Kind),
		BaseURL:         row.BaseUrl,
		CompletionModel: row.CompletionModel,
		EmbeddingModel:  row.EmbeddingModel,
		HasAPIKey:       row.HasApiKey,
		Jurisdiction:    integration.AiJurisdiction(row.Jurisdiction),
		CreatedAt:       timeFrom(row.CreatedAt),
		UpdatedAt:       timeFrom(row.UpdatedAt),
		Version:         int(row.Version),
	}, nil
}

func (OfferedAiRepository) List(ctx context.Context) ([]integration.AiProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListOfferedAiProviders(ctx)
	if err != nil {
		return nil, readFailed(err)
	}
	offered := make([]integration.AiProvider, 0, len(rows))
	for _, row := range rows {
		mapped, err := offeredFrom(row)
		if err != nil {
			return nil, err
		}
		offered = append(offered, mapped)
	}
	return offered, nil
}

func (OfferedAiRepository) Find(ctx context.Context, id shared.ID) (integration.AiProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiProvider{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return integration.AiProvider{}, offeredNotFound()
	}
	row, err := queries.FindOfferedAiProvider(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return integration.AiProvider{}, offeredNotFound()
		}
		return integration.AiProvider{}, readFailed(err)
	}
	return offeredFrom(sqlc.ListOfferedAiProvidersRow(row))
}

func (OfferedAiRepository) FindWithKey(
	ctx context.Context, id shared.ID,
) (integration.AiProvider, *crypto.Sealed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiProvider{}, nil, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return integration.AiProvider{}, nil, offeredNotFound()
	}
	row, err := queries.FindOfferedAiProviderKey(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return integration.AiProvider{}, nil, offeredNotFound()
		}
		return integration.AiProvider{}, nil, readFailed(err)
	}
	found, err := idFrom(row.ID)
	if err != nil {
		return integration.AiProvider{}, nil, err
	}
	offered := integration.AiProvider{
		ID: found, DisplayName: row.DisplayName, Processor: row.Processor,
		Kind: integration.AiProviderKind(row.Kind), BaseURL: row.BaseUrl,
		CompletionModel: row.CompletionModel, EmbeddingModel: row.EmbeddingModel,
		HasAPIKey: row.ApiKeyKeyID != nil, Jurisdiction: integration.AiJurisdiction(row.Jurisdiction),
	}
	if row.ApiKeyKeyID == nil {
		return offered, nil, nil
	}
	return offered, &crypto.Sealed{KeyID: *row.ApiKeyKeyID, Ciphertext: row.ApiKeyEnc}, nil
}

func (OfferedAiRepository) Insert(
	ctx context.Context, offered integration.AiProvider, sealed *crypto.Sealed, now time.Time,
) (integration.AiProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiProvider{}, err
	}
	id, err := uuidOf(offered.ID)
	if err != nil {
		return integration.AiProvider{}, err
	}
	params := sqlc.InsertOfferedAiProviderParams{
		ID: id, DisplayName: offered.DisplayName, Processor: offered.Processor,
		Kind: string(offered.Kind), BaseUrl: offered.BaseURL,
		CompletionModel: offered.CompletionModel, EmbeddingModel: offered.EmbeddingModel,
		Jurisdiction: string(offered.Jurisdiction),
		Now:          pgtype.Timestamptz{Time: now, Valid: true},
	}
	if sealed != nil {
		keyID := sealed.KeyID
		params.ApiKeyEnc, params.ApiKeyKeyID = sealed.Ciphertext, &keyID
	}
	row, err := queries.InsertOfferedAiProvider(ctx, params)
	if err != nil {
		if isUniqueViolation(err) {
			return integration.AiProvider{}, shared.ErrConflict.WithDetail("ai.display_name_taken").
				WithFields(shared.FieldError{Path: "/display_name", Code: "ai.display_name_taken"})
		}
		return integration.AiProvider{}, writeFailed(err)
	}
	return offeredFrom(sqlc.ListOfferedAiProvidersRow(row))
}

func (OfferedAiRepository) Update(
	ctx context.Context, offered integration.AiProvider, sealed *crypto.Sealed, now time.Time,
) (integration.AiProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiProvider{}, err
	}
	id, err := uuidOf(offered.ID)
	if err != nil {
		return integration.AiProvider{}, offeredNotFound()
	}
	params := sqlc.UpdateOfferedAiProviderParams{
		ID: id, DisplayName: offered.DisplayName, Processor: offered.Processor,
		Kind: string(offered.Kind), BaseUrl: offered.BaseURL,
		CompletionModel: offered.CompletionModel, EmbeddingModel: offered.EmbeddingModel,
		Jurisdiction: string(offered.Jurisdiction),
		Now:          pgtype.Timestamptz{Time: now, Valid: true},
	}
	if sealed != nil {
		keyID := sealed.KeyID
		params.ApiKeyEnc, params.ApiKeyKeyID = sealed.Ciphertext, &keyID
	}
	row, err := queries.UpdateOfferedAiProvider(ctx, params)
	return offeredWritten(sqlc.ListOfferedAiProvidersRow(row), err)
}

func (OfferedAiRepository) UpdateKeepingKey(
	ctx context.Context, offered integration.AiProvider, now time.Time,
) (integration.AiProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiProvider{}, err
	}
	id, err := uuidOf(offered.ID)
	if err != nil {
		return integration.AiProvider{}, offeredNotFound()
	}
	row, err := queries.UpdateOfferedAiProviderKeepingKey(ctx, sqlc.UpdateOfferedAiProviderKeepingKeyParams{
		ID: id, DisplayName: offered.DisplayName, Processor: offered.Processor,
		Kind: string(offered.Kind), BaseUrl: offered.BaseURL,
		CompletionModel: offered.CompletionModel, EmbeddingModel: offered.EmbeddingModel,
		Jurisdiction: string(offered.Jurisdiction),
		Now:          pgtype.Timestamptz{Time: now, Valid: true},
	})
	return offeredWritten(sqlc.ListOfferedAiProvidersRow(row), err)
}

func offeredWritten(row sqlc.ListOfferedAiProvidersRow, err error) (integration.AiProvider, error) {
	switch {
	case IsNoRows(err):
		return integration.AiProvider{}, offeredNotFound()
	case isUniqueViolation(err):
		return integration.AiProvider{}, shared.ErrConflict.WithDetail("ai.display_name_taken").
			WithFields(shared.FieldError{Path: "/display_name", Code: "ai.display_name_taken"})
	case err != nil:
		return integration.AiProvider{}, writeFailed(err)
	}
	return offeredFrom(row)
}

func (OfferedAiRepository) Delete(ctx context.Context, id shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, nil
	}
	removed, err := queries.DeleteOfferedAiProvider(ctx, key)
	if err != nil {
		return false, writeFailed(err)
	}
	return removed > 0, nil
}

// AiChoiceRepository is the workspace's choice of AI source, under row level security.
type AiChoiceRepository struct{}

func NewAiChoiceRepository() AiChoiceRepository { return AiChoiceRepository{} }

var _ repository.AiChoices = AiChoiceRepository{}

func (AiChoiceRepository) Find(ctx context.Context, tenantID shared.ID) (integration.AiChoice, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiChoice{}, err
	}
	row, err := queries.FindAiSource(ctx)
	if err != nil {
		if IsNoRows(err) {
			return integration.DefaultAiChoice(tenantID), nil
		}
		return integration.AiChoice{}, readFailed(err)
	}
	return choiceFrom(row)
}

func (AiChoiceRepository) Save(ctx context.Context, choice integration.AiChoice) (integration.AiChoice, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiChoice{}, err
	}
	params := sqlc.UpsertAiSourceParams{
		Source: string(choice.Source),
		Now:    pgtype.Timestamptz{Time: choice.UpdatedAt, Valid: true},
	}
	if !choice.OfferedProviderID.IsZero() {
		if params.OfferedProviderID, err = uuidOf(choice.OfferedProviderID); err != nil {
			return integration.AiChoice{}, err
		}
	}
	row, err := queries.UpsertAiSource(ctx, params)
	if err != nil {
		if isForeignKeyViolation(err) {
			// The operator withdrew the model between the check and the write.
			return integration.AiChoice{}, shared.ErrValidation.WithDetail("ai.offered_model_unknown").
				WithFields(shared.FieldError{Path: "/offered_model_id", Code: "ai.offered_model_unknown"})
		}
		return integration.AiChoice{}, writeFailed(err)
	}
	return choiceFrom(row)
}

func choiceFrom(row sqlc.AiSource) (integration.AiChoice, error) {
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return integration.AiChoice{}, err
	}
	choice := integration.AiChoice{
		TenantID: tenantID, Source: integration.AiSourceKind(row.Source),
		UpdatedAt: timeFrom(row.UpdatedAt), Version: int(row.Version),
	}
	if row.OfferedProviderID.Valid {
		if choice.OfferedProviderID, err = idFrom(row.OfferedProviderID); err != nil {
			return integration.AiChoice{}, err
		}
	}
	return choice, nil
}

// isForeignKeyViolation reports a reference to a row that is gone - here an offered model the
// operator withdrew while a workspace was choosing it.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
