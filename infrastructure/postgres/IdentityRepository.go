// SPDX-License-Identifier: BUSL-1.1
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

// isUniqueViolation reports the one driver error this package translates rather than wraps: an
// index refusing a duplicate. It is a race that racing callers must both be told about honestly -
// one of them created the row, and the other has to hear "taken" rather than "database error".
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// AccountRepository is the account table.
//
// No method takes a tenant. Row level security bounds every statement to the tenant of the running
// transaction, which is what makes an account of another tenant *not found* rather than forbidden -
// anything else confirms that it exists (ADR-0010, multi-tenancy.md §2).
type AccountRepository struct{}

func NewAccountRepository() AccountRepository { return AccountRepository{} }

var _ repository.Accounts = AccountRepository{}

func (r AccountRepository) Find(ctx context.Context, accountID shared.ID) (identity.Account, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Account{}, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return identity.Account{}, err
	}

	row, err := queries.FindAccount(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return identity.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		return identity.Account{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading account %s: %w", accountID, err))
	}
	return accountFrom(row.ID, row.Kind, row.Email, row.DisplayName, row.Status,
		row.Locale, row.TimeZone, row.WeekStart)
}

func (r AccountRepository) FindByEmail(ctx context.Context, email string) (identity.Account, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Account{}, err
	}

	row, err := queries.FindAccountByEmail(ctx, email)
	if err != nil {
		if IsNoRows(err) {
			return identity.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		return identity.Account{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			// The address stays out of the message: it is personal data, and an error travels
			// into logs (rule 10).
			WithCause(fmt.Errorf("reading an account by address: %w", err))
	}
	return accountFrom(row.ID, row.Kind, row.Email, row.DisplayName, row.Status,
		row.Locale, row.TimeZone, row.WeekStart)
}

func (r AccountRepository) Insert(ctx context.Context, account identity.Account) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(account.ID)
	if err != nil {
		return err
	}

	err = queries.InsertAccount(ctx, sqlc.InsertAccountParams{
		ID:          id,
		Kind:        sqlc.AccountKind(account.Kind),
		Email:       optionalText(account.Email),
		DisplayName: account.DisplayName,
		Status:      sqlc.AccountStatus(account.Status),
		Locale:      optionalText(account.Locale),
		TimeZone:    optionalText(account.TimeZone),
		WeekStart:   optionalText(account.WeekStart),
	})
	if err != nil {
		if isUniqueViolation(err) {
			// Two administrators inviting the same address at the same moment: both passed the
			// check, and the index is what decides. The loser gets a conflict rather than a
			// driver error.
			return shared.ErrConflict.WithDetail("accounts.email_taken")
		}
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing an account: %w", err))
	}
	return nil
}

// Restricted answers which of the accounts named may not be processed automatically.
//
// One round trip for a whole pool rather than one per candidate: the caller is a draw over a
// policy's candidates, and a question per candidate would make an assignment policy's cost grow
// with the size of the team.
func (r AccountRepository) Restricted(
	ctx context.Context, accountIDs []shared.ID,
) (map[shared.ID]bool, error) {
	restricted := map[shared.ID]bool{}
	if len(accountIDs) == 0 {
		return restricted, nil
	}

	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	keys := make([]pgtype.UUID, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		key, err := uuidOf(accountID)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	rows, err := queries.RestrictedAccounts(ctx, keys)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading which accounts are restricted: %w", err))
	}
	for _, row := range rows {
		accountID, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		restricted[accountID] = true
	}
	return restricted, nil
}

func (r AccountRepository) UpdatePreferences(
	ctx context.Context, account identity.Account, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(account.ID)
	if err != nil {
		return err
	}

	affected, err := queries.UpdateAccountPreferences(ctx, sqlc.UpdateAccountPreferencesParams{
		Locale:    optionalText(account.Locale),
		TimeZone:  optionalText(account.TimeZone),
		WeekStart: optionalText(account.WeekStart),
		UpdatedAt: timestampOf(at),
		ID:        id,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the preferences of %s: %w", account.ID, err))
	}
	if affected == 0 {
		// The account was there when it was read and is not now - deleted, or another tenant's
		// identifier that row level security hides.
		return shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return nil
}

// accountFrom rebuilds an account from its columns. Shared by both lookups, so the two cannot
// disagree about what a row means.
func accountFrom(
	id pgtype.UUID, kind sqlc.AccountKind, email *string, displayName string,
	status sqlc.AccountStatus, locale, timeZone, weekStart *string,
) (identity.Account, error) {
	accountID, err := idFrom(id)
	if err != nil {
		return identity.Account{}, err
	}
	return identity.Account{
		ID:          accountID,
		Kind:        identity.AccountKind(kind),
		Email:       stringFrom(email),
		DisplayName: displayName,
		Status:      identity.AccountStatus(status),
		Locale:      stringFrom(locale),
		TimeZone:    stringFrom(timeZone),
		WeekStart:   stringFrom(weekStart),
	}, nil
}

// GroupRepository is the group table and its member links.
type GroupRepository struct{}

func NewGroupRepository() GroupRepository { return GroupRepository{} }

var _ repository.Groups = GroupRepository{}

func (r GroupRepository) Find(ctx context.Context, groupID shared.ID) (identity.Group, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Group{}, err
	}
	id, err := uuidOf(groupID)
	if err != nil {
		return identity.Group{}, err
	}

	row, err := queries.FindGroup(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return identity.Group{}, shared.ErrNotFound.WithDetail("groups.not_found")
		}
		return identity.Group{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading group %s: %w", groupID, err))
	}

	id2, err := idFrom(row.ID)
	if err != nil {
		return identity.Group{}, err
	}
	return identity.Group{
		ID:          id2,
		Name:        row.Name,
		Description: stringFrom(row.Description),
		Version:     int(row.Version),
	}, nil
}

func (r GroupRepository) Insert(ctx context.Context, group identity.Group) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(group.ID)
	if err != nil {
		return err
	}

	err = queries.InsertGroup(ctx, sqlc.InsertGroupParams{
		ID:          id,
		Name:        group.Name,
		Description: optionalText(group.Description),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.
				WithDetail("groups.name_taken").
				WithParams(map[string]string{"name": group.Name})
		}
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing a group: %w", err))
	}
	return nil
}

func (r GroupRepository) Update(ctx context.Context, group identity.Group, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(group.ID)
	if err != nil {
		return err
	}

	affected, err := queries.UpdateGroup(ctx, sqlc.UpdateGroupParams{
		Name:        group.Name,
		Description: optionalText(group.Description),
		ID:          id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.
				WithDetail("groups.name_taken").
				WithParams(map[string]string{"name": group.Name})
		}
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing group %s: %w", group.ID, err))
	}
	if affected == 0 {
		// Either it is gone or somebody else moved it on. The second is the interesting one and
		// the one a client can act on: read it again and reapply (api-guidelines.md).
		return shared.ErrVersionConflict.
			WithDetail("groups.version_conflict").
			WithParams(map[string]string{"group_id": group.ID.String()})
	}
	return nil
}

func (r GroupRepository) Delete(ctx context.Context, groupID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(groupID)
	if err != nil {
		return err
	}

	if _, err := queries.DeleteGroup(ctx, id); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting group %s: %w", groupID, err))
	}
	return nil
}

func (r GroupRepository) AddMember(ctx context.Context, groupID, accountID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	group, account, err := pairOf(groupID, accountID)
	if err != nil {
		return err
	}

	if err := queries.AddGroupMember(ctx, sqlc.AddGroupMemberParams{
		GroupID: group, AccountID: account,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("adding %s to group %s: %w", accountID, groupID, err))
	}
	return nil
}

func (r GroupRepository) RemoveMember(ctx context.Context, groupID, accountID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	group, account, err := pairOf(groupID, accountID)
	if err != nil {
		return err
	}

	if err := queries.RemoveGroupMember(ctx, sqlc.RemoveGroupMemberParams{
		GroupID: group, AccountID: account,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing %s from group %s: %w", accountID, groupID, err))
	}
	return nil
}

func (r GroupRepository) Members(ctx context.Context, groupID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(groupID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.GroupMembers(ctx, id)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the members of group %s: %w", groupID, err))
	}

	members := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		accountID, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		members = append(members, accountID)
	}
	return members, nil
}

// MembershipGrantRepository is the write half of the membership table.
type MembershipGrantRepository struct{}

func NewMembershipGrantRepository() MembershipGrantRepository { return MembershipGrantRepository{} }

var _ repository.MembershipGrants = MembershipGrantRepository{}

func (r MembershipGrantRepository) Grant(ctx context.Context, grant identity.Grant) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(grant.ID)
	if err != nil {
		return err
	}
	accountID, err := optionalUUID(grant.AccountID)
	if err != nil {
		return err
	}
	groupID, err := optionalUUID(grant.GroupID)
	if err != nil {
		return err
	}
	scopeID, err := optionalUUID(grant.Scope.ID)
	if err != nil {
		return err
	}

	err = queries.GrantMembership(ctx, sqlc.GrantMembershipParams{
		ID:        id,
		AccountID: accountID,
		GroupID:   groupID,
		ScopeType: sqlc.MembershipScope(grant.Scope.Type),
		ScopeID:   scopeID,
		Role:      sqlc.MembershipRole(grant.Role),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("granting a membership: %w", err))
	}
	return nil
}

func (r MembershipGrantRepository) Revoke(ctx context.Context, membershipID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(membershipID)
	if err != nil {
		return false, err
	}

	affected, err := queries.RevokeMembership(ctx, id)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("revoking membership %s: %w", membershipID, err))
	}
	return affected > 0, nil
}

func (r MembershipGrantRepository) Find(ctx context.Context, membershipID shared.ID) (identity.Grant, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Grant{}, err
	}
	id, err := uuidOf(membershipID)
	if err != nil {
		return identity.Grant{}, err
	}

	row, err := queries.FindMembership(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return identity.Grant{}, shared.ErrNotFound.WithDetail("memberships.not_found")
		}
		return identity.Grant{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading membership %s: %w", membershipID, err))
	}

	grantID, err := idFrom(row.ID)
	if err != nil {
		return identity.Grant{}, err
	}
	accountID, err := optionalID(row.AccountID)
	if err != nil {
		return identity.Grant{}, err
	}
	groupID, err := optionalID(row.GroupID)
	if err != nil {
		return identity.Grant{}, err
	}
	scopeID, err := optionalID(row.ScopeID)
	if err != nil {
		return identity.Grant{}, err
	}

	return identity.Grant{
		ID:        grantID,
		AccountID: accountID,
		GroupID:   groupID,
		Scope:     identity.Scope{Type: identity.ScopeType(row.ScopeType), ID: scopeID},
		Role:      identity.Role(row.Role),
	}, nil
}

// pairOf converts the two identifiers every member link needs.
func pairOf(groupID, accountID shared.ID) (group, account pgtype.UUID, err error) {
	if group, err = uuidOf(groupID); err != nil {
		return group, account, err
	}
	account, err = uuidOf(accountID)
	return group, account, err
}
