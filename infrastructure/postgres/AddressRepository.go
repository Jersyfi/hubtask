// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// AddressRepository keeps a managed account's address waiting for its mailbox (ADR-0074 §6) on an
// ADDRESS row of auth_pending. The token is hashed under the pending credential's purpose label,
// the reset link's, because it is read the same way.
type AddressRepository struct {
	pendingHasher security.PendingTokenHasher
}

// NewAddressRepository builds it with the pending credential's hasher.
func NewAddressRepository(pendingHasher security.PendingTokenHasher) AddressRepository {
	return AddressRepository{pendingHasher: pendingHasher}
}

var _ repository.AddressRequests = AddressRepository{}

// LockAccount reads the account and holds its row for the transaction.
func (r AddressRepository) LockAccount(ctx context.Context, accountID shared.ID) (identity.Account, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Account{}, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return identity.Account{}, err
	}
	row, err := queries.LockAccountForAddress(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return identity.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		return identity.Account{}, queryFailed("locking the account", err)
	}
	return identity.Account{
		ID: accountID, Email: stringFrom(row.Email), SignInName: stringFrom(row.SignInName),
		Status: identity.AccountStatus(row.Status),
	}, nil
}

// LatestRequest answers the newest request's moment, or the zero time.
func (r AddressRepository) LatestRequest(ctx context.Context, accountID shared.ID) (time.Time, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return time.Time{}, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return time.Time{}, err
	}
	at, err := queries.LatestAddressRequest(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return time.Time{}, nil
		}
		return time.Time{}, queryFailed("reading the last address request", err)
	}
	return timeFrom(at), nil
}

// Supersede spends the account's unspent requests.
func (r AddressRepository) Supersede(ctx context.Context, accountID shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	if _, err := queries.SupersedePending(ctx, sqlc.SupersedePendingParams{
		Now: pgtype.Timestamptz{Time: at.UTC(), Valid: true}, AccountID: id,
		Purpose: string(identity.PendingAddress),
	}); err != nil {
		return queryFailed("superseding the address requests", err)
	}
	return nil
}

// Insert writes a request with its address and no token yet.
func (r AddressRepository) Insert(
	ctx context.Context, request identity.PendingCredential, address string,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(request.ID)
	if err != nil {
		return err
	}
	account, err := uuidOf(request.AccountID)
	if err != nil {
		return err
	}
	if err := queries.InsertAddressRequest(ctx, sqlc.InsertAddressRequestParams{
		ID: id, AccountID: account, Address: &address,
		CreatedAt: pgtype.Timestamptz{Time: request.CreatedAt.UTC(), Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: request.ExpiresAt.UTC(), Valid: true},
	}); err != nil {
		return queryFailed("writing the address request", err)
	}
	return nil
}

// Mint stores the mail's token hash and answers where the mail goes.
func (r AddressRepository) Mint(
	ctx context.Context, requestID shared.ID, presented identity.Token, now time.Time,
) (repository.AddressMail, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.AddressMail{}, err
	}
	id, err := uuidOf(requestID)
	if err != nil {
		return repository.AddressMail{}, err
	}
	row, err := queries.MintAddressToken(ctx, sqlc.MintAddressTokenParams{
		TokenHash: r.pendingHasher.Hash(presented.Secret()), ID: id,
		Now: pgtype.Timestamptz{Time: now.UTC(), Valid: true},
	})
	if err != nil {
		if IsNoRows(err) {
			return repository.AddressMail{}, shared.ErrNotFound.WithDetail("auth.address_link_invalid")
		}
		return repository.AddressMail{}, queryFailed("minting the address link", err)
	}
	return repository.AddressMail{
		Address: row.Address, Locale: stringFrom(row.Locale), DisplayName: row.DisplayName,
	}, nil
}

// FindByToken answers the request a presented link names.
func (r AddressRepository) FindByToken(
	ctx context.Context, token identity.Token,
) (repository.AddressLookup, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.AddressLookup{}, err
	}
	row, err := queries.FindAddressRequestByHash(ctx, r.pendingHasher.Hash(token.Secret()))
	if err != nil {
		if IsNoRows(err) {
			return repository.AddressLookup{}, shared.ErrNotFound.WithDetail("auth.address_link_invalid")
		}
		return repository.AddressLookup{}, queryFailed("reading the address request", err)
	}
	requestID, err := idFrom(row.ID)
	if err != nil {
		return repository.AddressLookup{}, err
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return repository.AddressLookup{}, err
	}
	return repository.AddressLookup{
		Credential: identity.PendingCredential{
			ID: requestID, TenantID: token.TenantID(), AccountID: accountID,
			Purpose:   identity.PendingAddress,
			CreatedAt: timeFrom(row.CreatedAt), ExpiresAt: timeFrom(row.ExpiresAt),
			ConsumedAt: timeFrom(row.ConsumedAt),
		},
		Address:       row.Address,
		AccountStatus: identity.AccountStatus(row.AccountStatus),
		TenantStatus:  identity.TenantStatus(row.TenantStatus),
	}, nil
}

// Consume spends the request once.
func (r AddressRepository) Consume(ctx context.Context, requestID shared.ID, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(requestID)
	if err != nil {
		return false, err
	}
	spent, err := queries.ConsumePendingCredential(ctx, sqlc.ConsumePendingCredentialParams{
		Now: pgtype.Timestamptz{Time: at.UTC(), Valid: true}, ID: id,
	})
	if err != nil {
		return false, queryFailed("spending the address request", err)
	}
	return spent == 1, nil
}

// Confirm makes the address the account's while it is still managed.
func (r AddressRepository) Confirm(
	ctx context.Context, accountID shared.ID, address string, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.ConfirmAccountAddress(ctx, sqlc.ConfirmAccountAddressParams{
		Address: &address, At: pgtype.Timestamptz{Time: at.UTC(), Valid: true}, AccountID: id,
	})
	if err != nil {
		if isUniqueViolation(err) {
			// The address became another account's here between the request and the click; the
			// index decides, and whoever holds the mailbox hears it.
			return false, shared.ErrConflict.WithDetail("accounts.address_taken").
				WithFields(shared.FieldError{Path: "/token", Code: "accounts.address_taken"})
		}
		return false, queryFailed("confirming the address", err)
	}
	return changed == 1, nil
}
