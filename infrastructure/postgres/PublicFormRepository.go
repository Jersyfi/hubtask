// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// PublicFormRepository stores a collection's public form (migration 0302) and finds the one a
// token names. The token is hashed here and nowhere else: the pepper is this layer's secret.
type PublicFormRepository struct {
	hasher security.InboundTokenHasher
}

// NewPublicFormRepository takes the hasher under the public form's own purpose label.
func NewPublicFormRepository(hasher security.InboundTokenHasher) PublicFormRepository {
	return PublicFormRepository{hasher: hasher}
}

var _ repository.PublicForms = PublicFormRepository{}

func publicFormFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// Put writes the link under the token's hash, replacing the collection's previous one.
func (r PublicFormRepository) Put(ctx context.Context, form servicedesk.PublicForm, token integration.FormLinkToken) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(form.CollectionID, form.CreatedBy)
	if err != nil {
		return err
	}
	if err := queries.PutPublicForm(ctx, sqlc.PutPublicFormParams{
		CollectionID: ids[0], TokenHash: r.hasher.Hash(token.Secret()), CreatedBy: ids[1], At: timestampOf(form.CreatedAt),
	}); err != nil {
		// No token in the message: it is the credential (rule 10, T-21).
		return publicFormFailed("writing the public form", err)
	}
	return nil
}

// Find answers the collection's link.
func (r PublicFormRepository) Find(ctx context.Context, collectionID shared.ID) (servicedesk.PublicForm, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.PublicForm{}, false, err
	}
	key, err := uuidOf(collectionID)
	if err != nil {
		return servicedesk.PublicForm{}, false, err
	}
	row, err := queries.FindPublicForm(ctx, key)
	if IsNoRows(err) {
		return servicedesk.PublicForm{}, false, nil
	}
	if err != nil {
		return servicedesk.PublicForm{}, false, publicFormFailed("reading the public form", err)
	}
	form, err := publicFormOf("", row.CollectionID, row.CreatedBy, row.CreatedAt)
	return form, err == nil, err
}

// FindByToken is the unauthenticated route's lookup, bounded to the token's tenant by the
// caller's transaction and row level security (ADR-0010).
func (r PublicFormRepository) FindByToken(ctx context.Context, token integration.FormLinkToken) (servicedesk.PublicForm, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.PublicForm{}, err
	}
	row, err := queries.FindPublicFormByHash(ctx, r.hasher.Hash(token.Secret()))
	if IsNoRows(err) {
		return servicedesk.PublicForm{}, shared.ErrNotFound.WithDetail("public_forms.link_not_found")
	}
	if err != nil {
		return servicedesk.PublicForm{}, publicFormFailed("reading the public form by its token", err)
	}
	return publicFormOf(token.TenantID(), row.CollectionID, row.CreatedBy, row.CreatedAt)
}

// Delete takes the collection's link away.
func (r PublicFormRepository) Delete(ctx context.Context, collectionID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(collectionID)
	if err != nil {
		return false, err
	}
	rows, err := queries.DeletePublicForm(ctx, key)
	if err != nil {
		return false, publicFormFailed("removing the public form", err)
	}
	return rows > 0, nil
}

func publicFormOf(tenant shared.ID, collection, by pgtype.UUID, at pgtype.Timestamptz) (servicedesk.PublicForm, error) {
	ids, err := pageIDs(collection, by)
	if err != nil {
		return servicedesk.PublicForm{}, err
	}
	return servicedesk.PublicForm{TenantID: tenant, CollectionID: ids[0], CreatedBy: ids[1], CreatedAt: timeFrom(at)}, nil
}
