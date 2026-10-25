// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/mail"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// MailboxRepository is a workspace's connected mailboxes (ADR-0094).
type MailboxRepository struct{}

// NewMailboxRepository builds the repository.
func NewMailboxRepository() MailboxRepository { return MailboxRepository{} }

var _ repository.Mailboxes = MailboxRepository{}

func (MailboxRepository) List(ctx context.Context) ([]domain.Mailbox, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListMailboxes(ctx)
	if err != nil {
		return nil, mailQueryFailed("reading the mailboxes", err)
	}
	out := make([]domain.Mailbox, 0, len(rows))
	for _, row := range rows {
		box, err := mailboxFrom(sqlc.FindMailboxRow{
			ID: row.ID, Protocol: row.Protocol, Address: row.Address, Host: row.Host, Port: row.Port,
			SessionUrl: row.SessionUrl, Username: row.Username, Folder: row.Folder, DoneFolder: row.DoneFolder,
			Preset: row.Preset, Enabled: row.Enabled, CheckedAt: row.CheckedAt, Failure: row.Failure,
			FailingSince: row.FailingSince, CreatedAt: row.CreatedAt,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, box)
	}
	return out, nil
}

func (MailboxRepository) Find(ctx context.Context, id shared.ID) (domain.Mailbox, crypto.Sealed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Mailbox{}, crypto.Sealed{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Mailbox{}, crypto.Sealed{}, err
	}
	row, err := queries.FindMailbox(ctx, key)
	if IsNoRows(err) {
		return domain.Mailbox{}, crypto.Sealed{}, shared.ErrNotFound.WithDetail("mail.mailbox_not_found")
	}
	if err != nil {
		return domain.Mailbox{}, crypto.Sealed{}, mailQueryFailed("reading a mailbox", err)
	}
	box, err := mailboxFrom(row)
	if err != nil {
		return domain.Mailbox{}, crypto.Sealed{}, err
	}
	return box, crypto.Sealed{KeyID: row.SecretKeyID, Ciphertext: row.SecretEnc}, nil
}

func (MailboxRepository) Insert(ctx context.Context, box domain.Mailbox, sealed crypto.Sealed, by shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(box.ID)
	if err != nil {
		return err
	}
	byID, err := optionalUUID(by)
	if err != nil {
		return err
	}
	params := sqlc.InsertMailboxParams{
		ID: id, Protocol: string(box.Protocol), Address: box.Address, Username: box.Username,
		Folder: box.Folder, DoneFolder: box.DoneFolder, Preset: optionalText(box.Preset), Enabled: box.Enabled,
		SecretEnc: sealed.Ciphertext, SecretKeyID: sealed.KeyID, CreatedAt: timestampOf(box.CreatedAt), CreatedBy: byID,
	}
	if box.Protocol == domain.ProtocolIMAP {
		port := int32(box.Port) //nolint:gosec // validated to 1-65535 by NewMailbox
		params.Host, params.Port = &box.Host, &port
	} else {
		params.SessionUrl = &box.SessionURL
	}
	err = queries.InsertMailbox(ctx, params)
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("mail.mailbox_taken").
			WithFields(shared.FieldError{Path: "/address", Code: "mail.mailbox_taken"})
	}
	if err != nil {
		return mailQueryFailed("storing a mailbox", err)
	}
	return nil
}

func (MailboxRepository) RecordCheck(ctx context.Context, box domain.Mailbox) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(box.ID)
	if err != nil {
		return err
	}
	_, err = queries.SetMailboxChecked(ctx, sqlc.SetMailboxCheckedParams{
		ID: id, CheckedAt: optionalTimestamp(box.CheckedAt), Failure: optionalText(box.Failure),
		FailingSince: optionalTimestamp(box.FailingSince),
	})
	if err != nil {
		return mailQueryFailed("recording a mailbox read", err)
	}
	return nil
}

func (MailboxRepository) Delete(ctx context.Context, id shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	rows, err := queries.DeleteMailbox(ctx, key)
	if err != nil {
		return mailQueryFailed("removing a mailbox", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("mail.mailbox_not_found")
	}
	return nil
}

func (MailboxRepository) Count(ctx context.Context) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountMailboxes(ctx)
	if err != nil {
		return 0, mailQueryFailed("counting the mailboxes", err)
	}
	return int(count), nil
}

func mailboxFrom(row sqlc.FindMailboxRow) (domain.Mailbox, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Mailbox{}, err
	}
	box := domain.Mailbox{
		ID: id, Protocol: domain.Protocol(row.Protocol), Address: row.Address, Host: stringFrom(row.Host),
		SessionURL: stringFrom(row.SessionUrl), Username: row.Username, Folder: row.Folder,
		DoneFolder: row.DoneFolder, Preset: stringFrom(row.Preset), Enabled: row.Enabled,
		CheckedAt: optionalTime(row.CheckedAt), Failure: stringFrom(row.Failure),
		FailingSince: optionalTime(row.FailingSince), CreatedAt: timeFrom(row.CreatedAt),
	}
	if row.Port != nil {
		box.Port = int(*row.Port)
	}
	return box, nil
}
