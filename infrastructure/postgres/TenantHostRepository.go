// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// TenantHostRepository is the hosts a workspace answers at (migration 0104).
//
// No method takes a tenant: row level security bounds every statement and the workspace is the
// transaction's (ADR-0010). Nothing here finds a workspace *by* host - that lookup would have to
// cross the boundary, the way `resolve_tenant` does, and it arrives with the milestone that resolves
// a request through a custom domain.
type TenantHostRepository struct{}

func NewTenantHostRepository() TenantHostRepository { return TenantHostRepository{} }

var _ repository.TenantHosts = TenantHostRepository{}

func (TenantHostRepository) Insert(ctx context.Context, host identity.TenantHost) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	verified := pgtype.Timestamptz{}
	if !host.VerifiedAt.IsZero() {
		verified = pgtype.Timestamptz{Time: host.VerifiedAt, Valid: true}
	}
	if err := queries.InsertTenantHost(ctx, sqlc.InsertTenantHostParams{
		Host:         host.Host,
		State:        string(host.State),
		Verification: host.Verification,
		VerifiedAt:   verified,
		IsCanonical:  host.Canonical,
		CreatedAt:    pgtype.Timestamptz{Time: host.CreatedAt, Valid: true},
	}); err != nil {
		if isUniqueViolation(err) {
			// One host, one workspace, installation-wide. The conflict is the answer a caller has
			// to be able to act on: the host is somebody's, and which somebody is not theirs to
			// learn.
			return shared.ErrConflict.
				WithDetail("tenant_host.host_taken").
				WithParams(map[string]string{"host": host.Host})
		}
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing a workspace host: %w", err))
	}
	return nil
}

func (TenantHostRepository) List(ctx context.Context) ([]identity.TenantHost, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListTenantHosts(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a workspace's hosts: %w", err))
	}
	scope, _ := scopeFromContext(ctx)
	hosts := make([]identity.TenantHost, 0, len(rows))
	for _, row := range rows {
		hosts = append(hosts, identity.TenantHost{
			// The tenant is the transaction's, and it is in the mapped row deliberately: three
			// mappers in this package once left it out, and every retry and correction 500'd.
			TenantID:     scope.TenantID,
			Host:         row.Host,
			State:        identity.HostState(row.State),
			Verification: row.Verification,
			VerifiedAt:   row.VerifiedAt.Time,
			Canonical:    row.IsCanonical,
			CreatedAt:    row.CreatedAt.Time,
		})
	}
	return hosts, nil
}
