// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PageLanguageVariantRepository keeps which page is written in another language for which, on the
// page rows themselves (migration 0314).
type PageLanguageVariantRepository struct{}

var _ repository.LanguageVariants = PageLanguageVariantRepository{}

// SourceOf answers the page a page is a variant of and its tag.
func (PageLanguageVariantRepository) SourceOf(ctx context.Context, pageID shared.ID) (shared.ID, string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", "", err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return "", "", err
	}
	row, err := queries.PageLanguageLink(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", pageFailed("reading the page's language source", err)
	}
	source, err := optionalID(row.LanguageSourceID)
	if err != nil || source.IsZero() || row.LanguageVariant == nil {
		return "", "", err
	}
	return source, *row.LanguageVariant, nil
}

// VariantsOf answers a page's variants that are not trashed.
func (PageLanguageVariantRepository) VariantsOf(ctx context.Context, sourceID shared.ID) ([]knowledge.LanguageVariant, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(sourceID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageLanguageVariants(ctx, key)
	if err != nil {
		return nil, pageFailed("reading the page's language variants", err)
	}
	return mapRows(rows, func(row sqlc.PageLanguageVariantsRow) (knowledge.LanguageVariant, error) {
		id, err := idFrom(row.ID)
		return knowledge.LanguageVariant{PageID: id, Language: row.Language}, err
	})
}

// Link links the page to its source under the tag.
func (PageLanguageVariantRepository) Link(ctx context.Context, link knowledge.LanguageLink) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(link.PageID, link.SourceID)
	if err != nil {
		return err
	}
	language := link.Language
	rows, err := queries.SetPageLanguageLink(ctx, sqlc.SetPageLanguageLinkParams{PageID: ids[0], SourceID: ids[1], Language: &language})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("pages.variant_language_taken").WithParams(map[string]string{"language": language})
	}
	if err != nil {
		return pageFailed("linking the language variant", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("pages.not_found")
	}
	return nil
}

// Unlink makes the page one of its own again.
func (r PageLanguageVariantRepository) Unlink(ctx context.Context, pageID shared.ID) (bool, error) {
	source, _, err := r.SourceOf(ctx, pageID)
	if err != nil || source.IsZero() {
		return false, err
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return false, err
	}
	rows, err := queries.SetPageLanguageLink(ctx, sqlc.SetPageLanguageLinkParams{PageID: key})
	if err != nil {
		return false, pageFailed("unlinking the language variant", err)
	}
	return rows > 0, nil
}
