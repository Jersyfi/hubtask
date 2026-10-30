// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

var _ repository.Organisations = ServiceDeskOrganisations{}

// organisationDomainConstraint is the name the trigger of migration 0287 refuses an overlap under.
const organisationDomainConstraint = "requester_organisation_domain_uq"

// ServiceDeskOrganisations stores a workspace's portal organisations (migration 0287).
type ServiceDeskOrganisations struct{}

func (ServiceDeskRepository) Organisations() ServiceDeskOrganisations {
	return ServiceDeskOrganisations{}
}

// organisationConflict names which uniqueness refused the write.
func organisationConflict(err error, key string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == organisationDomainConstraint {
		return shared.ErrConflict.WithDetail("organisations.domain_taken")
	}
	return shared.ErrConflict.WithDetail("organisations.key_taken").WithParams(map[string]string{"key": key})
}

func (ServiceDeskOrganisations) Insert(ctx context.Context, o servicedesk.Organisation) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(o.ID)
	if err != nil {
		return err
	}
	tenant, err := uuidOf(o.TenantID)
	if err != nil {
		return err
	}
	if err := queries.InsertRequesterOrganisation(ctx, sqlc.InsertRequesterOrganisationParams{
		ID: id, TenantID: tenant, Key: o.Key, Name: o.Name, Domains: o.Domains,
		SharesRequests: o.SharesRequests, Now: timestampOf(o.CreatedAt),
	}); err != nil {
		if isUniqueViolation(err) {
			return organisationConflict(err, o.Key)
		}
		return queryFailed("writing an organisation", err)
	}
	return nil
}

func (ServiceDeskOrganisations) Update(ctx context.Context, o servicedesk.Organisation, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.UpdateRequesterOrganisation(ctx, sqlc.UpdateRequesterOrganisationParams{
		Name: o.Name, Domains: o.Domains, SharesRequests: o.SharesRequests,
		Now: timestampOf(o.UpdatedAt), Key: o.Key,
		ExpectedVersion: int32(expectedVersion), //nolint:gosec // a version is far below 2^31
	})
	if err != nil {
		if isUniqueViolation(err) {
			return organisationConflict(err, o.Key)
		}
		return queryFailed("updating an organisation", err)
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("organisations.version_conflict")
	}
	return nil
}

func organisationFrom(row sqlc.RequesterOrganisation) (servicedesk.Organisation, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return servicedesk.Organisation{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return servicedesk.Organisation{}, err
	}
	return servicedesk.Organisation{
		ID: id, TenantID: tenant, Key: row.Key, Name: row.Name, Domains: append([]string{}, row.Domains...),
		SharesRequests: row.SharesRequests, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		Version: int(row.Version),
	}, nil
}

func (ServiceDeskOrganisations) Find(ctx context.Context, key string) (servicedesk.Organisation, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.Organisation{}, err
	}
	row, err := queries.FindRequesterOrganisation(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return servicedesk.Organisation{}, shared.ErrNotFound.WithDetail("organisations.not_found")
		}
		return servicedesk.Organisation{}, queryFailed("reading an organisation", err)
	}
	return organisationFrom(row)
}

func (ServiceDeskOrganisations) List(ctx context.Context) ([]servicedesk.Organisation, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListRequesterOrganisations(ctx, servicedesk.MaxOrganisations)
	if err != nil {
		return nil, queryFailed("listing organisations", err)
	}
	out := make([]servicedesk.Organisation, 0, len(rows))
	for _, row := range rows {
		o, err := organisationFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func (ServiceDeskOrganisations) Count(ctx context.Context) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	held, err := queries.CountRequesterOrganisations(ctx)
	if err != nil {
		return 0, queryFailed("counting organisations", err)
	}
	return int(held), nil
}

func (ServiceDeskOrganisations) Delete(ctx context.Context, key string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteRequesterOrganisation(ctx, key)
	if err != nil {
		return queryFailed("deleting an organisation", err)
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("organisations.not_found")
	}
	return nil
}

func (ServiceDeskOrganisations) OfDomain(ctx context.Context, domain string) (servicedesk.Organisation, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.Organisation{}, err
	}
	row, err := queries.RequesterOrganisationOfDomain(ctx, domain)
	if err != nil {
		if IsNoRows(err) {
			return servicedesk.Organisation{}, shared.ErrNotFound.WithDetail("organisations.not_found")
		}
		return servicedesk.Organisation{}, queryFailed("reading the organisation of a domain", err)
	}
	return organisationFrom(row)
}

func (ServiceDeskOrganisations) Members(ctx context.Context, domains []string, limit int) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.AccountsAtDomains(ctx, sqlc.AccountsAtDomainsParams{
		Domains: domains, Max: int32(limit), //nolint:gosec // the caller bounds it far below 2^31
	})
	if err != nil {
		return nil, queryFailed("reading an organisation's people", err)
	}
	out := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
