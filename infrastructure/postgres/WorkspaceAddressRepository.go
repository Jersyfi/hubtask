// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// WorkspaceAddressRepository is a change of a workspace's address (multi-tenancy.md §3.1): the short
// name and the main row in the change's one transaction, the sessions it ends, who is told, and the
// notice a member without an address reads at sign-in (migration 0209). No method takes a tenant: row
// level security and current_tenant_id() bound every statement to the transaction's workspace.
type WorkspaceAddressRepository struct{}

func NewWorkspaceAddressRepository() WorkspaceAddressRepository { return WorkspaceAddressRepository{} }

var (
	_ adminrepo.TenantAddresses       = WorkspaceAddressRepository{}
	_ adminrepo.AddressAudience       = WorkspaceAddressRepository{}
	_ repository.AddressChangeNotices = WorkspaceAddressRepository{}
)

// tenantSlugIndex is the tenant table's unique short name.
const tenantSlugIndex = "tenant_slug_key"

func (WorkspaceAddressRepository) ChangeAddress(
	ctx context.Context, slug string, host identity.TenantHost, now time.Time,
) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	at := pgtype.Timestamptz{Time: now, Valid: true}
	changed, err := queries.ChangeTenantSlug(ctx, sqlc.ChangeTenantSlugParams{Slug: slug, Now: at})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == tenantSlugIndex {
			params := map[string]string{"slug": slug}
			return "", shared.ErrConflict.WithDetail("admin.slug_taken").WithParams(params).
				WithFields(shared.FieldError{Path: "/slug", Code: "admin.slug_taken", Params: params})
		}
		return "", shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("changing the short name: %w", err))
	}
	if changed == 0 {
		return "", shared.ErrConflict.WithDetail("admin.tenant_leaving")
	}
	dropped, err := queries.DropMainTenantHost(ctx)
	if err != nil {
		return "", shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("dropping the main host: %w", err))
	}
	if err := (TenantHostRepository{}).Insert(ctx, host); err != nil {
		return "", err
	}
	if len(dropped) == 0 {
		return "", nil
	}
	return dropped[0], nil
}

func (WorkspaceAddressRepository) EndSessions(ctx context.Context, now time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	ended, err := queries.RevokeWorkspaceSessions(ctx, pgtype.Timestamptz{Time: now, Valid: true})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("ending the workspace's sessions: %w", err))
	}
	return int(ended), nil
}

func (WorkspaceAddressRepository) PeopleToTell(ctx context.Context) ([]shared.ID, []shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := queries.PeopleToTellOfAddress(ctx)
	if err != nil {
		return nil, nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading who is told of a change of address: %w", err))
	}
	mailed, unmailed := []shared.ID{}, []shared.ID{}
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, nil, err
		}
		if row.HasEmail {
			mailed = append(mailed, id)
		} else {
			unmailed = append(unmailed, id)
		}
	}
	return mailed, unmailed, nil
}

func (WorkspaceAddressRepository) Record(
	ctx context.Context, accountID shared.ID, from, to string, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	if err := queries.RecordAddressChangeNotice(ctx, sqlc.RecordAddressChangeNoticeParams{
		AccountID: account, FromHost: from, ToHost: to,
		ChangedAt: pgtype.Timestamptz{Time: at, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording an address notice: %w", err))
	}
	return nil
}

func (WorkspaceAddressRepository) Of(
	ctx context.Context, accountID shared.ID,
) ([]identity.AddressChangeNotice, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.AddressChangeNoticesOf(ctx, account)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading an address notice: %w", err))
	}
	notices := make([]identity.AddressChangeNotice, 0, len(rows))
	for _, row := range rows {
		notices = append(notices, identity.AddressChangeNotice{
			From: row.FromHost, To: row.ToHost, ChangedAt: row.ChangedAt.Time,
		})
	}
	return notices, nil
}

func (WorkspaceAddressRepository) Acknowledge(ctx context.Context, accountID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	if _, err := queries.AcknowledgeAddressChangeNotice(ctx, account); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("acknowledging an address notice: %w", err))
	}
	return nil
}
