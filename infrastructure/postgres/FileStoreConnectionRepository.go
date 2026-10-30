// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/filestore"
	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// FileStoreConnectionRepository stores a person's file store connections (migration 0279)
// through the unit of work's transaction (rule 3).
type FileStoreConnectionRepository struct{}

// NewFileStoreConnectionRepository answers the store.
func NewFileStoreConnectionRepository() FileStoreConnectionRepository {
	return FileStoreConnectionRepository{}
}

var _ repository.Store = FileStoreConnectionRepository{}

func fileStoreFailed(what string, err error) error {
	return fmt.Errorf("postgres: file store connection: %s: %w", what, err)
}

func fileStoreConnectionOf(r sqlc.ListFileStoreConnectionsRow) filelink.Connection {
	var k cmdbIDs
	return filelink.Connection{
		ID: k.of(r.ID), AccountID: k.of(r.AccountID), Kind: filelink.ConnectionKind(r.Kind), Server: r.Server,
		Login: r.Login, CreatedAt: timeFrom(r.CreatedAt), UsedAt: timeFrom(r.UsedAt),
	}
}

func (FileStoreConnectionRepository) List(ctx context.Context, accountID shared.ID) ([]filelink.Connection, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFileStoreConnections(ctx, account)
	if err != nil {
		return nil, fileStoreFailed("listing", err)
	}
	out := make([]filelink.Connection, 0, len(rows))
	for _, r := range rows {
		out = append(out, fileStoreConnectionOf(r))
	}
	return out, nil
}

func (FileStoreConnectionRepository) Count(ctx context.Context, accountID shared.ID) (int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return 0, err
	}
	n, err := q.CountFileStoreConnections(ctx, account)
	if err != nil {
		return 0, fileStoreFailed("counting", err)
	}
	return int(n), nil
}

func (FileStoreConnectionRepository) Find(ctx context.Context, id shared.ID) (filelink.Connection, crypto.Sealed, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return filelink.Connection{}, crypto.Sealed{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return filelink.Connection{}, crypto.Sealed{}, shared.ErrNotFound.WithDetail("filestore.unknown")
	}
	r, err := q.FindFileStoreConnection(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return filelink.Connection{}, crypto.Sealed{}, shared.ErrNotFound.WithDetail("filestore.unknown")
	}
	if err != nil {
		return filelink.Connection{}, crypto.Sealed{}, fileStoreFailed("finding", err)
	}
	c := fileStoreConnectionOf(sqlc.ListFileStoreConnectionsRow{
		ID: r.ID, AccountID: r.AccountID, Kind: r.Kind, Server: r.Server, Login: r.Login, CreatedAt: r.CreatedAt, UsedAt: r.UsedAt,
	})
	return c, crypto.Sealed{KeyID: r.SecretKeyID, Ciphertext: r.SecretEnc}, nil
}

func (FileStoreConnectionRepository) Connect(ctx context.Context, c filelink.Connection, sealed crypto.Sealed) (shared.ID, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	id, err := uuidOf(c.ID)
	if err != nil {
		return "", err
	}
	account, err := uuidOf(c.AccountID)
	if err != nil {
		return "", err
	}
	stored, err := q.UpsertFileStoreConnection(ctx, sqlc.UpsertFileStoreConnectionParams{
		ID: id, AccountID: account, Kind: string(c.Kind), Server: c.Server, Login: c.Login,
		SecretEnc: sealed.Ciphertext, SecretKeyID: sealed.KeyID, CreatedAt: timestampOf(c.CreatedAt),
	})
	if err != nil {
		return "", fileStoreFailed("connecting", err)
	}
	var k cmdbIDs
	return k.of(stored), nil
}

func (FileStoreConnectionRepository) Reseal(ctx context.Context, id shared.ID, sealed crypto.Sealed) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	if err := q.UpdateFileStoreConnectionSecret(ctx, sqlc.UpdateFileStoreConnectionSecretParams{
		SecretEnc: sealed.Ciphertext, SecretKeyID: sealed.KeyID, ID: key,
	}); err != nil {
		return fileStoreFailed("resealing", err)
	}
	return nil
}

func (FileStoreConnectionRepository) Touch(ctx context.Context, id shared.ID, at time.Time) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	if err := q.TouchFileStoreConnection(ctx, sqlc.TouchFileStoreConnectionParams{UsedAt: timestampOf(at), ID: key}); err != nil {
		return fileStoreFailed("touching", err)
	}
	return nil
}

func (FileStoreConnectionRepository) Disconnect(ctx context.Context, id shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return shared.ErrNotFound.WithDetail("filestore.unknown")
	}
	n, err := q.DeleteFileStoreConnection(ctx, key)
	if err != nil {
		return fileStoreFailed("disconnecting", err)
	}
	if n == 0 {
		return shared.ErrNotFound.WithDetail("filestore.unknown")
	}
	return nil
}

func (FileStoreConnectionRepository) Await(ctx context.Context, p repository.Pending, stateHash []byte) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(p.ID)
	if err != nil {
		return err
	}
	account, err := uuidOf(p.AccountID)
	if err != nil {
		return err
	}
	if err := q.InsertFileStoreAuthorization(ctx, sqlc.InsertFileStoreAuthorizationParams{
		ID: id, AccountID: account, Kind: string(p.Kind), Server: p.Server, PollEndpoint: p.PollEndpoint,
		StateHash: stateHash, SecretEnc: p.Secret.Ciphertext, SecretKeyID: p.Secret.KeyID, ExpiresAt: timestampOf(p.ExpiresAt),
	}); err != nil {
		return fileStoreFailed("awaiting", err)
	}
	return nil
}

func (FileStoreConnectionRepository) Pending(ctx context.Context, stateHash []byte) (repository.Pending, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return repository.Pending{}, err
	}
	r, err := q.FindFileStoreAuthorization(ctx, stateHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.Pending{}, shared.ErrNotFound.WithDetail("filestore.state_unknown")
	}
	if err != nil {
		return repository.Pending{}, fileStoreFailed("finding the waiting connection", err)
	}
	var k cmdbIDs
	return repository.Pending{
		ID: k.of(r.ID), AccountID: k.of(r.AccountID), Kind: filelink.ConnectionKind(r.Kind), Server: r.Server,
		PollEndpoint: r.PollEndpoint, Secret: crypto.Sealed{KeyID: r.SecretKeyID, Ciphertext: r.SecretEnc},
		ExpiresAt: timeFrom(r.ExpiresAt),
	}, nil
}

func (FileStoreConnectionRepository) Forget(ctx context.Context, id shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	if err := q.DeleteFileStoreAuthorization(ctx, key); err != nil {
		return fileStoreFailed("forgetting", err)
	}
	return nil
}

func (FileStoreConnectionRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteExpiredFileStoreAuthorizations(ctx, timestampOf(now))
	if err != nil {
		return 0, fileStoreFailed("forgetting the expired", err)
	}
	return int(n), nil
}
