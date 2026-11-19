// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// WopiRepository stores the WOPI host's rows: the document server, the locks, the versions.
type WopiRepository struct{}

var _ mediarepo.Wopi = WopiRepository{}

func wopiFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// DocumentServer answers the workspace's choice.
func (WopiRepository) DocumentServer(ctx context.Context) (media.DocumentServerChoice, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return media.DocumentServerChoice{}, err
	}
	row, err := queries.FindDocumentServer(ctx)
	if IsNoRows(err) {
		return media.DocumentServerChoice{Kind: media.DocumentServerNone}, nil
	}
	if err != nil {
		return media.DocumentServerChoice{}, wopiFailed("reading the document server", err)
	}
	return media.DocumentServerChoice{Kind: media.DocumentServer(row.Kind), URL: row.Url}, nil
}

// SetDocumentServer writes the choice; NONE removes the row.
func (WopiRepository) SetDocumentServer(
	ctx context.Context, choice media.DocumentServerChoice, setBy shared.ID, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if !choice.IsOn() {
		if _, err := queries.DeleteDocumentServer(ctx); err != nil {
			return wopiFailed("removing the document server", err)
		}
		return nil
	}
	by, err := optionalUUID(setBy)
	if err != nil {
		return err
	}
	if err := queries.UpsertDocumentServer(ctx, sqlc.UpsertDocumentServerParams{
		Kind: string(choice.Kind), Url: choice.URL, SetBy: by, UpdatedAt: timestampOf(at),
	}); err != nil {
		return wopiFailed("writing the document server", err)
	}
	return nil
}

// Lock answers the lock row.
func (WopiRepository) Lock(ctx context.Context, mediaID shared.ID) (*media.Lock, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(mediaID)
	if err != nil {
		return nil, err
	}
	row, err := queries.FindMediaLock(ctx, id)
	if IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, wopiFailed("reading a lock", err)
	}
	return &media.Lock{ID: row.LockID, ExpiresAt: timeFrom(row.ExpiresAt)}, nil
}

// PutLock writes the lock.
func (WopiRepository) PutLock(ctx context.Context, mediaID shared.ID, lock media.Lock) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(mediaID)
	if err != nil {
		return err
	}
	if err := queries.UpsertMediaLock(ctx, sqlc.UpsertMediaLockParams{
		MediaID: id, LockID: lock.ID, ExpiresAt: timestampOf(lock.ExpiresAt),
	}); err != nil {
		return wopiFailed("writing a lock", err)
	}
	return nil
}

// DropLock removes the lock.
func (WopiRepository) DropLock(ctx context.Context, mediaID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(mediaID)
	if err != nil {
		return err
	}
	if _, err := queries.DeleteMediaLock(ctx, id); err != nil {
		return wopiFailed("removing a lock", err)
	}
	return nil
}

// SaveVersion holds the file's row, keeps its bytes as the earlier version's object, numbers the
// version and gives the file the new bytes - one transaction, so a version never names bytes the
// file does not hold.
func (WopiRepository) SaveVersion(ctx context.Context, mediaID shared.ID, save mediarepo.Save) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	id, err := uuidOf(mediaID)
	if err != nil {
		return 0, err
	}
	if _, err := queries.LockMediaObjectForSave(ctx, id); err != nil {
		if IsNoRows(err) {
			return 0, shared.ErrNotFound
		}
		return 0, wopiFailed("holding a file for its save", err)
	}
	earlierID, err := uuidOf(save.Earlier.ID)
	if err != nil {
		return 0, err
	}
	createdBy, err := optionalUUID(save.Earlier.CreatedBy)
	if err != nil {
		return 0, err
	}
	if err := queries.InsertReadyMediaObject(ctx, sqlc.InsertReadyMediaObjectParams{
		ID: earlierID, StorageKey: save.Earlier.StorageKey, FileName: optionalText(save.Earlier.FileName),
		MimeType: save.Earlier.ContentType, ByteSize: save.Earlier.ByteSize, Usage: string(save.Earlier.Usage),
		CreatedBy: createdBy, CreatedAt: timestampOf(save.Earlier.CreatedAt),
	}); err != nil {
		return 0, wopiFailed("keeping an earlier version", err)
	}
	number, err := queries.NextMediaVersionNumber(ctx, id)
	if err != nil {
		return 0, wopiFailed("numbering a version", err)
	}
	savedBy, err := optionalUUID(save.SavedBy)
	if err != nil {
		return 0, err
	}
	if err := queries.InsertMediaVersion(ctx, sqlc.InsertMediaVersionParams{
		MediaID: id, Number: number, EarlierMediaID: earlierID, SavedBy: savedBy, SavedAt: timestampOf(save.SavedAt),
	}); err != nil {
		return 0, wopiFailed("writing a version", err)
	}
	moved, err := queries.ReplaceMediaContent(ctx, sqlc.ReplaceMediaContentParams{
		ID: id, StorageKey: save.StorageKey, MimeType: save.ContentType, ByteSize: save.ByteSize,
	})
	if err != nil {
		return 0, wopiFailed("giving a file its new bytes", err)
	}
	if moved == 0 {
		return 0, shared.ErrNotFound
	}
	return int(number), nil
}

// Versions lists a file's saves, newest first.
func (WopiRepository) Versions(ctx context.Context, mediaID shared.ID) ([]mediarepo.Version, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(mediaID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListMediaVersions(ctx, id)
	if err != nil {
		return nil, wopiFailed("listing versions", err)
	}
	out := make([]mediarepo.Version, 0, len(rows))
	for _, row := range rows {
		earlier, err := idFrom(row.EarlierMediaID)
		if err != nil {
			return nil, err
		}
		var by shared.ID
		if row.SavedBy.Valid {
			if by, err = idFrom(row.SavedBy); err != nil {
				return nil, err
			}
		}
		out = append(out, mediarepo.Version{
			Number: int(row.Number), EarlierMediaID: earlier, SavedBy: by, SavedAt: timeFrom(row.SavedAt),
		})
	}
	return out, nil
}
