// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/view"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// SavedViewRepository stores the bookmark shelf (D-07). The query documents pass through it as
// they were sent - validated by the application, interpreted by nobody here.
//
// Nothing here names a tenant: the transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010).
type SavedViewRepository struct{}

func NewSavedViewRepository() SavedViewRepository { return SavedViewRepository{} }

var _ repository.SavedViews = SavedViewRepository{}

// Find answers one view.
func (r SavedViewRepository) Find(ctx context.Context, id shared.ID) (view.SavedView, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return view.SavedView{}, err
	}
	rowID, err := uuidOf(id)
	if err != nil {
		return view.SavedView{}, err
	}

	row, err := queries.FindSavedView(ctx, rowID)
	if err != nil {
		if IsNoRows(err) {
			// Also the answer when the row belongs to another tenant (multi-tenancy.md §2).
			return view.SavedView{}, shared.ErrNotFound.
				WithDetail("views.not_found").
				WithParams(map[string]string{"view_id": id.String()})
		}
		return view.SavedView{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the view %s: %w", id, err))
	}
	return savedViewFrom(row)
}

// ListOwned answers the account's own views.
func (r SavedViewRepository) ListOwned(
	ctx context.Context, ownerID shared.ID,
) ([]view.SavedView, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	owner, err := uuidOf(ownerID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListSavedViewsOwned(ctx, owner)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the views of %s: %w", ownerID, err))
	}
	return savedViewsFrom(rows)
}

// ListReachable answers the account's own views plus what is shared into the given scopes.
func (r SavedViewRepository) ListReachable(
	ctx context.Context, ownerID shared.ID, scopeIDs []shared.ID,
) ([]view.SavedView, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	owner, err := uuidOf(ownerID)
	if err != nil {
		return nil, err
	}
	scopes, err := uuidsOf(scopeIDs)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListSavedViewsReachable(ctx, sqlc.ListSavedViewsReachableParams{
		OwnerID: owner, ScopeIds: scopes,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the reachable views of %s: %w", ownerID, err))
	}
	return savedViewsFrom(rows)
}

// Insert writes a new view.
func (r SavedViewRepository) Insert(ctx context.Context, saved view.SavedView) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(saved.ID)
	if err != nil {
		return err
	}
	scopeID, err := optionalUUID(saved.ScopeID)
	if err != nil {
		return err
	}
	owner, err := uuidOf(saved.OwnerID)
	if err != nil {
		return err
	}
	query, grouping, err := viewDocumentsOf(saved)
	if err != nil {
		return err
	}

	err = queries.InsertSavedView(ctx, sqlc.InsertSavedViewParams{
		ID: id, ScopeType: string(saved.ScopeType), ScopeID: scopeID, OwnerID: owner,
		Name: saved.Name, Layout: string(saved.Layout), Query: query, Grouping: grouping,
		VisibleFields: saved.VisibleFields, Sharing: string(saved.Sharing),
		CreatedAt: timestampOf(saved.CreatedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the view: %w", err))
	}
	return nil
}

// SetAttributes writes the view's own fields whole, or reports a version conflict.
func (r SavedViewRepository) SetAttributes(
	ctx context.Context, saved view.SavedView, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(saved.ID)
	if err != nil {
		return err
	}
	query, grouping, err := viewDocumentsOf(saved)
	if err != nil {
		return err
	}

	affected, err := queries.SetSavedViewAttributes(ctx, sqlc.SetSavedViewAttributesParams{
		Name: saved.Name, Layout: string(saved.Layout), Query: query, Grouping: grouping,
		VisibleFields: saved.VisibleFields, ID: id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		// No name in the message: it is the owner's free text, and the error text reaches the log
		// (rule 10, ADR-0017).
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the attributes of the view %s: %w", saved.ID, err))
	}
	return savedViewConflictIfUntouched(affected, saved.ID, expectedVersion)
}

// SetSharing writes who sees the view, or reports a version conflict.
func (r SavedViewRepository) SetSharing(
	ctx context.Context, saved view.SavedView, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(saved.ID)
	if err != nil {
		return err
	}

	affected, err := queries.SetSavedViewSharing(ctx, sqlc.SetSavedViewSharingParams{
		Sharing: string(saved.Sharing), ID: id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the sharing of the view %s: %w", saved.ID, err))
	}
	return savedViewConflictIfUntouched(affected, saved.ID, expectedVersion)
}

// Delete removes the view, or reports a version conflict.
func (r SavedViewRepository) Delete(
	ctx context.Context, saved view.SavedView, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(saved.ID)
	if err != nil {
		return err
	}

	affected, err := queries.DeleteSavedView(ctx, sqlc.DeleteSavedViewParams{
		ID: id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting the view %s: %w", saved.ID, err))
	}
	return savedViewConflictIfUntouched(affected, saved.ID, expectedVersion)
}

// savedViewConflictIfUntouched is versionConflictIfUntouched's twin for this table: either the
// row is gone, somebody else moved it on, or it belongs to another tenant - and a caller must not
// be able to tell the last apart from the first two (multi-tenancy.md §2).
func savedViewConflictIfUntouched(affected int64, id shared.ID, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("views.version_conflict").
		WithParams(map[string]string{
			"view_id": id.String(), "expected_version": strconv.Itoa(expectedVersion),
		})
}

// viewDocumentsOf renders the two stored documents. The marshalling lives here rather than in the
// domain, because how a map becomes bytes is the adapter's dialect (ADR-0001).
func viewDocumentsOf(saved view.SavedView) (query, grouping []byte, err error) {
	if query, err = json.Marshal(saved.Query); err != nil {
		return nil, nil, shared.ErrInternal.
			WithDetail("views.query_unencodable").
			WithCause(fmt.Errorf("encoding the query of %s: %w", saved.ID, err))
	}
	if grouping, err = json.Marshal(saved.Grouping); err != nil {
		return nil, nil, shared.ErrInternal.
			WithDetail("views.query_unencodable").
			WithCause(fmt.Errorf("encoding the grouping of %s: %w", saved.ID, err))
	}
	return query, grouping, nil
}

func savedViewsFrom(rows []sqlc.SavedView) ([]view.SavedView, error) {
	views := make([]view.SavedView, 0, len(rows))
	for _, row := range rows {
		saved, err := savedViewFrom(row)
		if err != nil {
			return nil, err
		}
		views = append(views, saved)
	}
	return views, nil
}

func savedViewFrom(row sqlc.SavedView) (view.SavedView, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return view.SavedView{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return view.SavedView{}, err
	}
	scopeID, err := optionalID(row.ScopeID)
	if err != nil {
		return view.SavedView{}, err
	}
	ownerID, err := optionalID(row.OwnerID)
	if err != nil {
		return view.SavedView{}, err
	}

	var query map[string]any
	if err := json.Unmarshal(row.Query, &query); err != nil {
		return view.SavedView{}, shared.ErrInternal.
			WithDetail("views.query_undecodable").
			WithCause(fmt.Errorf("decoding the query of %s: %w", id, err))
	}
	var grouping map[string]any
	if err := json.Unmarshal(row.Grouping, &grouping); err != nil {
		return view.SavedView{}, shared.ErrInternal.
			WithDetail("views.query_undecodable").
			WithCause(fmt.Errorf("decoding the grouping of %s: %w", id, err))
	}

	fields := row.VisibleFields
	if fields == nil {
		fields = []string{}
	}

	return view.SavedView{
		ID:            id,
		TenantID:      tenantID,
		ScopeType:     view.ViewScope(row.ScopeType),
		ScopeID:       scopeID,
		OwnerID:       ownerID,
		Name:          row.Name,
		Layout:        view.Layout(row.Layout),
		Query:         query,
		Grouping:      grouping,
		VisibleFields: fields,
		Sharing:       view.Sharing(row.Sharing),
		CreatedAt:     timeFrom(row.CreatedAt),
		Version:       int(row.Version),
	}, nil
}
