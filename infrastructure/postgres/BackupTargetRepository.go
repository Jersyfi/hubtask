// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// BackupTargetRepository stores the targets a tenant has configured (E-03).
//
// The credential is read by one method, and that method reads nothing else. It is the same
// separation the statements make and for the same reason: what a credential must never do is
// travel alongside something on its way to a response, and the way to make that structural rather
// than remembered is for the rows that go to a response never to contain one.
type BackupTargetRepository struct{}

func NewBackupTargetRepository() BackupTargetRepository { return BackupTargetRepository{} }

var _ repository.Targets = BackupTargetRepository{}

// Insert writes the target and the sealed credential beside it.
func (r BackupTargetRepository) Insert(
	ctx context.Context, target domain.Target, credential crypto.Sealed,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(target.ID)
	if err != nil {
		return err
	}
	tenantID, err := uuidOf(target.TenantID)
	if err != nil {
		return err
	}
	createdBy, err := uuidOf(target.CreatedBy)
	if err != nil {
		return err
	}
	acknowledgedBy, err := optionalUUID(target.InsecureAckBy)
	if err != nil {
		return err
	}

	config, err := json.Marshal(target.Config)
	if err != nil {
		return shared.ErrInternal.
			WithDetail("backup.config_unserialisable").
			WithCause(fmt.Errorf("serialising the configuration of target %s: %w", target.ID, err))
	}

	err = queries.InsertBackupTarget(ctx, sqlc.InsertBackupTargetParams{
		ID: id, TenantID: tenantID, Name: target.Name, Kind: target.Kind.String(),
		Config:          config,
		CredentialEnc:   credential.Ciphertext,
		CredentialKeyID: optionalText(credential.KeyID),
		EncryptionMode:  target.EncryptionMode.String(),
		EncryptionKeyID: optionalText(target.EncryptionKeyID),
		RegionNote:      optionalText(target.RegionNote),
		InsecureAckBy:   acknowledgedBy,
		InsecureAckAt:   momentOrNothing(target.InsecureAckAt),
		CreatedAt:       timestampOf(target.CreatedAt),
		CreatedBy:       createdBy,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.
				WithDetail("backup.target_name_taken").
				WithParams(map[string]string{"name": target.Name}).
				WithFields(shared.FieldError{Path: "/name", Code: "backup.target_name_taken"})
		}
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing backup target %s: %w", target.ID, err))
	}
	return nil
}

// List answers the tenant's targets, by name and without a credential among them.
func (r BackupTargetRepository) List(ctx context.Context) ([]domain.Target, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListBackupTargets(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the backup targets: %w", err))
	}

	targets := make([]domain.Target, 0, len(rows))
	for _, row := range rows {
		target, err := targetOf(sqlc.FindBackupTargetRow(row))
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

// Find answers one target, without its credential.
func (r BackupTargetRepository) Find(
	ctx context.Context, id shared.ID,
) (domain.Target, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Target{}, err
	}
	targetID, err := uuidOf(id)
	if err != nil {
		return domain.Target{}, err
	}

	row, err := queries.FindBackupTarget(ctx, targetID)
	if err != nil {
		if IsNoRows(err) {
			return domain.Target{}, notFoundTarget(id)
		}
		return domain.Target{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading backup target %s: %w", id, err))
	}
	return targetOf(row)
}

// Credential answers the sealed credential, still sealed. This repository never learns what is
// inside it: the key lives in the encryptor, and that is what makes a database dump worth nothing
// on its own (security.md §3).
func (r BackupTargetRepository) Credential(
	ctx context.Context, id shared.ID,
) (crypto.Sealed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return crypto.Sealed{}, err
	}
	targetID, err := uuidOf(id)
	if err != nil {
		return crypto.Sealed{}, err
	}

	row, err := queries.FindBackupTargetCredential(ctx, targetID)
	if err != nil {
		if IsNoRows(err) {
			return crypto.Sealed{}, notFoundTarget(id)
		}
		return crypto.Sealed{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the credential of backup target %s: %w", id, err))
	}

	sealed := crypto.Sealed{Ciphertext: row.CredentialEnc}
	if row.CredentialKeyID != nil {
		sealed.KeyID = *row.CredentialKeyID
	}
	return sealed, nil
}

// RecordTest writes down what the probe found.
func (r BackupTargetRepository) RecordTest(
	ctx context.Context, id shared.ID, at time.Time, ok bool, code string,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	targetID, err := uuidOf(id)
	if err != nil {
		return err
	}

	affected, err := queries.RecordBackupTargetTest(ctx, sqlc.RecordBackupTargetTestParams{
		TestedAt: timestampOf(at), Succeeded: &ok, ErrorCode: optionalText(code), ID: targetID,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the probe of backup target %s: %w", id, err))
	}
	if affected == 0 {
		return notFoundTarget(id)
	}
	return nil
}

// Coverage is what the installation's health surface asks.
func (r BackupTargetRepository) Coverage(ctx context.Context) (repository.Coverage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Coverage{}, err
	}

	row, err := queries.CountBackupTargets(ctx)
	if err != nil {
		return repository.Coverage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the backup targets: %w", err))
	}
	return repository.Coverage{
		Configured: int(row.Configured), Unencrypted: int(row.Unencrypted),
	}, nil
}

// targetOf maps a row. It takes the row type of the single read, and the listing's identical row
// is converted at the call site - sqlc generates one type per statement, and a mapper per type
// would be two places for a column to be forgotten.
func targetOf(row sqlc.FindBackupTargetRow) (domain.Target, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Target{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return domain.Target{}, err
	}
	createdBy, err := idFrom(row.CreatedBy)
	if err != nil {
		return domain.Target{}, err
	}

	config := domain.TargetConfig{}
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &config); err != nil {
			return domain.Target{}, shared.ErrInternal.
				WithDetail("backup.config_unreadable").
				WithCause(fmt.Errorf("reading the configuration of target %s: %w", id, err))
		}
	}

	target := domain.Target{
		ID: id, TenantID: tenantID, Name: row.Name,
		Kind: domain.TargetKind(row.Kind), Config: config,
		EncryptionMode: domain.EncryptionMode(row.EncryptionMode),
		Enabled:        row.Enabled, CreatedAt: row.CreatedAt.Time,
		CreatedBy: createdBy, Version: int(row.Version),
	}
	if row.CredentialKeyID != nil {
		target.CredentialKeyID = *row.CredentialKeyID
	}
	if row.EncryptionKeyID != nil {
		target.EncryptionKeyID = *row.EncryptionKeyID
	}
	if row.RegionNote != nil {
		target.RegionNote = *row.RegionNote
	}
	if row.LastTestAt.Valid {
		target.LastTestAt = row.LastTestAt.Time
	}
	if row.LastTestOk != nil {
		target.LastTestOK = row.LastTestOk
	}
	if row.LastTestError != nil {
		target.LastTestError = *row.LastTestError
	}
	if row.InsecureAckAt.Valid {
		target.InsecureAckAt = row.InsecureAckAt.Time
	}
	if row.InsecureAckBy.Valid {
		if acknowledgedBy, err := idFrom(row.InsecureAckBy); err == nil {
			target.InsecureAckBy = acknowledgedBy
		}
	}
	return target, nil
}

// notFoundTarget is the one answer for a target in another tenant, an instance-wide target, and a
// target that never existed.
func notFoundTarget(id shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("backup.target_not_found").
		WithParams(map[string]string{"target_id": id.String()})
}

// momentOrNothing is a moment that may not have happened. The neighbouring optionalTimestamp
// takes a pointer, which is the shape a nullable column has elsewhere in this package; a zero
// time is the shape the backup aggregate uses, because "never acknowledged" is a fact about the
// target rather than an absent field.
func momentOrNothing(at time.Time) pgtype.Timestamptz {
	if at.IsZero() {
		return pgtype.Timestamptz{}
	}
	return timestampOf(at)
}
