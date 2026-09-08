// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// The workspace's AI provider (J-02). No method takes a tenant: row level security bounds every
// statement, and the workspace is the row's key (ADR-0010).
type AiProviderRepository struct{}

func NewAiProviderRepository() AiProviderRepository { return AiProviderRepository{} }

var _ repository.AiProviders = AiProviderRepository{}

func (AiProviderRepository) Upsert(
	ctx context.Context, configured integration.AiProvider, sealed *crypto.Sealed, now time.Time,
) (integration.AiProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiProvider{}, err
	}

	params := sqlc.UpsertAiProviderParams{
		Kind:              string(configured.Kind),
		BaseUrl:           configured.BaseURL,
		CompletionModel:   configured.CompletionModel,
		EmbeddingModel:    configured.EmbeddingModel,
		Jurisdiction:      string(configured.Jurisdiction),
		ProcessingAllowed: configured.ProcessingAllowed,
		Now:               pgtype.Timestamptz{Time: now, Valid: true},
	}
	if sealed != nil {
		keyID := sealed.KeyID
		params.ApiKeyEnc, params.ApiKeyKeyID = sealed.Ciphertext, &keyID
	}

	row, err := queries.UpsertAiProvider(ctx, params)
	if err != nil {
		return integration.AiProvider{}, writeFailed(err)
	}
	return integration.AiProvider{
		TenantID:          configured.TenantID,
		Kind:              integration.AiProviderKind(row.Kind),
		BaseURL:           row.BaseUrl,
		CompletionModel:   row.CompletionModel,
		EmbeddingModel:    row.EmbeddingModel,
		HasAPIKey:         row.HasApiKey,
		Jurisdiction:      integration.AiJurisdiction(row.Jurisdiction),
		ProcessingAllowed: row.ProcessingAllowed,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
		Version:           int(row.Version),
	}, nil
}

func (AiProviderRepository) UpsertKeepingKey(
	ctx context.Context, configured integration.AiProvider, now time.Time,
) (integration.AiProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiProvider{}, err
	}
	row, err := queries.UpsertAiProviderKeepingKey(ctx, sqlc.UpsertAiProviderKeepingKeyParams{
		Kind:              string(configured.Kind),
		BaseUrl:           configured.BaseURL,
		CompletionModel:   configured.CompletionModel,
		EmbeddingModel:    configured.EmbeddingModel,
		Jurisdiction:      string(configured.Jurisdiction),
		ProcessingAllowed: configured.ProcessingAllowed,
		Now:               pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return integration.AiProvider{}, writeFailed(err)
	}
	return integration.AiProvider{
		TenantID:          configured.TenantID,
		Kind:              integration.AiProviderKind(row.Kind),
		BaseURL:           row.BaseUrl,
		CompletionModel:   row.CompletionModel,
		EmbeddingModel:    row.EmbeddingModel,
		HasAPIKey:         row.HasApiKey,
		Jurisdiction:      integration.AiJurisdiction(row.Jurisdiction),
		ProcessingAllowed: row.ProcessingAllowed,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
		Version:           int(row.Version),
	}, nil
}

func (AiProviderRepository) Find(ctx context.Context) (integration.AiProvider, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiProvider{}, err
	}
	row, err := queries.FindAiProvider(ctx)
	if err != nil {
		if IsNoRows(err) {
			return integration.AiProvider{}, shared.ErrNotFound.WithDetail("ai.not_configured")
		}
		return integration.AiProvider{}, readFailed(err)
	}
	return integration.AiProvider{
		Kind:              integration.AiProviderKind(row.Kind),
		BaseURL:           row.BaseUrl,
		CompletionModel:   row.CompletionModel,
		EmbeddingModel:    row.EmbeddingModel,
		HasAPIKey:         row.HasApiKey,
		Jurisdiction:      integration.AiJurisdiction(row.Jurisdiction),
		ProcessingAllowed: row.ProcessingAllowed,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
		Version:           int(row.Version),
	}, nil
}

func (AiProviderRepository) FindWithKey(
	ctx context.Context,
) (integration.AiProvider, *crypto.Sealed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.AiProvider{}, nil, err
	}
	row, err := queries.FindAiProviderKey(ctx)
	if err != nil {
		if IsNoRows(err) {
			return integration.AiProvider{}, nil, shared.ErrNotFound.WithDetail("ai.not_configured")
		}
		return integration.AiProvider{}, nil, readFailed(err)
	}

	configured := integration.AiProvider{
		Kind:              integration.AiProviderKind(row.Kind),
		BaseURL:           row.BaseUrl,
		CompletionModel:   row.CompletionModel,
		EmbeddingModel:    row.EmbeddingModel,
		HasAPIKey:         row.ApiKeyKeyID != nil,
		Jurisdiction:      integration.AiJurisdiction(row.Jurisdiction),
		ProcessingAllowed: row.ProcessingAllowed,
	}
	// The column pair is constrained all-or-nothing by the migration, so one nil check answers
	// for both: a provider that needs no key is a configuration rather than a half-written row.
	if row.ApiKeyKeyID == nil {
		return configured, nil, nil
	}
	return configured, &crypto.Sealed{KeyID: *row.ApiKeyKeyID, Ciphertext: row.ApiKeyEnc}, nil
}

func (AiProviderRepository) Delete(ctx context.Context) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	removed, err := queries.DeleteAiProvider(ctx)
	if err != nil {
		return false, writeFailed(err)
	}
	return removed > 0, nil
}

func (AiProviderRepository) RewrapKey(
	ctx context.Context, sealed crypto.Sealed, expectedKeyID string,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	moved, err := queries.RewrapAiProviderKey(ctx, sqlc.RewrapAiProviderKeyParams{
		ApiKeyEnc: sealed.Ciphertext, ApiKeyKeyID: &sealed.KeyID, ExpectedKeyID: &expectedKeyID,
	})
	if err != nil {
		return false, writeFailed(err)
	}
	return moved > 0, nil
}

// The two failure shapes, spelled once each: a query that could not run is an unavailable
// dependency and never a raw driver message, which is T-18's rule.
func writeFailed(err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("writing the AI provider: %w", err))
}

func readFailed(err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("reading the AI provider: %w", err))
}
