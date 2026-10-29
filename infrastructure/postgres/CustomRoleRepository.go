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

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

const customRoleKeyIndex = "custom_role_key_uq"

// CustomRoleRepository stores the workspace's own roles and their grants (migration 0219), through
// the unit of work's transaction (rule 3).
type CustomRoleRepository struct{}

func NewCustomRoleRepository() CustomRoleRepository { return CustomRoleRepository{} }

var _ repository.CustomRoles = CustomRoleRepository{}

func (r CustomRoleRepository) List(ctx context.Context) ([]repository.CustomRoleRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListCustomRoles(ctx)
	if err != nil {
		return nil, roleQueryError(err, "listing the custom roles")
	}
	out := make([]repository.CustomRoleRow, 0, len(rows))
	for _, row := range rows {
		role, err := customRoleFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, nil
}

func (r CustomRoleRepository) Find(ctx context.Context, key string) (repository.CustomRoleRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.CustomRoleRow{}, err
	}
	row, err := queries.FindCustomRole(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return repository.CustomRoleRow{}, shared.ErrNotFound.WithDetail("roles.not_found").
				WithParams(map[string]string{"key": key})
		}
		return repository.CustomRoleRow{}, roleQueryError(err, "reading the custom role")
	}
	return customRoleFrom(row)
}

func (r CustomRoleRepository) Insert(ctx context.Context, role identity.CustomRole, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(role.ID)
	if err != nil {
		return err
	}
	err = queries.InsertCustomRole(ctx, sqlc.InsertCustomRoleParams{
		ID: id, Key: role.Key, Name: role.Name, Permissions: role.Permissions, At: timestampOf(at),
	})
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == customRoleKeyIndex {
		return shared.ErrConflict.WithDetail("roles.key_taken").WithParams(map[string]string{"key": role.Key})
	}
	return roleQueryError(err, "writing the custom role")
}

func (r CustomRoleRepository) Update(ctx context.Context, role identity.CustomRole, version int, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(role.ID)
	if err != nil {
		return false, err
	}
	affected, err := queries.UpdateCustomRole(ctx, sqlc.UpdateCustomRoleParams{
		Name: role.Name, Permissions: role.Permissions, At: timestampOf(at), ID: id,
		//nolint:gosec // G115: a version is a row counter
		Version: int32(version),
	})
	if err != nil {
		return false, roleQueryError(err, "updating the custom role")
	}
	return affected > 0, nil
}

func (r CustomRoleRepository) Delete(ctx context.Context, roleID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(roleID)
	if err != nil {
		return false, err
	}
	affected, err := queries.DeleteCustomRole(ctx, id)
	if err != nil {
		return false, roleQueryError(err, "deleting the custom role")
	}
	return affected > 0, nil
}

func (r CustomRoleRepository) Grant(ctx context.Context, grant identity.CustomGrant, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	params := sqlc.GrantCustomRoleParams{ScopeType: sqlc.MembershipScope(grant.Scope.Type), At: timestampOf(at)}
	for _, field := range []struct {
		into *pgtype.UUID
		id   shared.ID
	}{{&params.ID, grant.ID}, {&params.RoleID, grant.RoleID}} {
		if *field.into, err = uuidOf(field.id); err != nil {
			return false, err
		}
	}
	for _, field := range []struct {
		into *pgtype.UUID
		id   shared.ID
	}{{&params.AccountID, grant.AccountID}, {&params.GroupID, grant.GroupID}, {&params.ScopeID, grant.Scope.ID}} {
		if *field.into, err = optionalUUID(field.id); err != nil {
			return false, err
		}
	}
	affected, err := queries.GrantCustomRole(ctx, params)
	if err != nil {
		if isForeignKeyViolation(err) {
			return false, shared.ErrNotFound.WithDetail("memberships.subject_not_found")
		}
		return false, roleQueryError(err, "granting the custom role")
	}
	return affected > 0, nil
}

func (r CustomRoleRepository) FindGrant(ctx context.Context, grantID shared.ID) (repository.CustomGrantRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.CustomGrantRow{}, err
	}
	id, err := uuidOf(grantID)
	if err != nil {
		return repository.CustomGrantRow{}, err
	}
	row, err := queries.FindCustomRoleGrant(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return repository.CustomGrantRow{}, shared.ErrNotFound.WithDetail("memberships.not_found")
		}
		return repository.CustomGrantRow{}, roleQueryError(err, "reading the custom role grant")
	}
	return customGrantFrom(sqlc.ListCustomRoleGrantsRow(row))
}

func (r CustomRoleRepository) Revoke(ctx context.Context, grantID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(grantID)
	if err != nil {
		return false, err
	}
	affected, err := queries.RevokeCustomRoleGrant(ctx, id)
	if err != nil {
		return false, roleQueryError(err, "revoking the custom role grant")
	}
	return affected > 0, nil
}

func (r CustomRoleRepository) Grants(ctx context.Context, limit int) ([]repository.CustomGrantRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: the caller's limit is a page size
	rows, err := queries.ListCustomRoleGrants(ctx, int32(limit))
	if err != nil {
		return nil, roleQueryError(err, "listing the custom role grants")
	}
	out := make([]repository.CustomGrantRow, 0, len(rows))
	for _, row := range rows {
		grant, err := customGrantFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, grant)
	}
	return out, nil
}

// customRolesAlong is what Memberships.Along adds for the custom roles held along a path.
func customRolesAlong(
	ctx context.Context, queries *sqlc.Queries, accountID shared.ID, account pgtype.UUID, scopeIDs []pgtype.UUID,
) ([]identity.Membership, error) {
	rows, err := queries.CustomRolesAlongPath(ctx, sqlc.CustomRolesAlongPathParams{AccountID: account, ScopeIds: scopeIDs})
	if err != nil {
		return nil, roleQueryError(err, "reading the custom roles held")
	}
	out := make([]identity.Membership, 0, len(rows))
	for _, row := range rows {
		scopeID, err := optionalID(row.ScopeID)
		if err != nil {
			return nil, err
		}
		out = append(out, identity.Membership{
			AccountID:   accountID,
			Scope:       identity.Scope{Type: identity.ScopeType(row.ScopeType), ID: scopeID},
			Custom:      row.Key,
			Permissions: row.Permissions,
		})
	}
	return out, nil
}

func customRoleFrom(row sqlc.CustomRole) (repository.CustomRoleRow, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return repository.CustomRoleRow{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return repository.CustomRoleRow{}, err
	}
	return repository.CustomRoleRow{
		Role: identity.CustomRole{
			ID: id, TenantID: tenant, Key: row.Key, Name: row.Name, Permissions: row.Permissions,
		},
		CreatedAt: timeFrom(row.CreatedAt), UpdatedAt: timeFrom(row.UpdatedAt), Version: int(row.Version),
	}, nil
}

func customGrantFrom(row sqlc.ListCustomRoleGrantsRow) (repository.CustomGrantRow, error) {
	ids := make([]shared.ID, 6)
	for i, value := range []pgtype.UUID{row.ID, row.TenantID, row.AccountID, row.GroupID, row.ScopeID, row.RoleID} {
		id, err := optionalID(value)
		if err != nil {
			return repository.CustomGrantRow{}, err
		}
		ids[i] = id
	}
	return repository.CustomGrantRow{
		Grant: identity.CustomGrant{
			ID: ids[0], TenantID: ids[1], AccountID: ids[2], GroupID: ids[3],
			Scope: identity.Scope{Type: identity.ScopeType(row.ScopeType), ID: ids[4]}, RoleID: ids[5],
		},
		RoleKey: row.RoleKey,
	}, nil
}

func roleQueryError(err error, what string) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}
