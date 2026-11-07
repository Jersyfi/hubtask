// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// MailUndeliverableRepository stores the addresses the mail server refused for good
// (0367_mail_undeliverable.sql).
type MailUndeliverableRepository struct{}

func NewMailUndeliverableRepository() MailUndeliverableRepository {
	return MailUndeliverableRepository{}
}

var _ repository.Undeliverable = MailUndeliverableRepository{}

// Mark records the refusal.
func (r MailUndeliverableRepository) Mark(
	ctx context.Context, accountID shared.ID, address string, since time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	if err := queries.MarkMailUndeliverable(ctx, sqlc.MarkMailUndeliverableParams{
		AccountID: account, Address: address, Since: timestampOf(since),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("marking an address undeliverable: %w", err))
	}
	return nil
}

// Find returns the marked address.
func (r MailUndeliverableRepository) Find(
	ctx context.Context, accountID shared.ID,
) (string, time.Time, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return "", time.Time{}, err
	}
	row, err := queries.FindMailUndeliverable(ctx, account)
	if err != nil {
		if IsNoRows(err) {
			return "", time.Time{}, shared.ErrNotFound
		}
		return "", time.Time{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading an undeliverable address: %w", err))
	}
	return row.Address, row.Since.Time, nil
}

// Clear removes the mark.
func (r MailUndeliverableRepository) Clear(ctx context.Context, accountID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	if err := queries.ClearMailUndeliverable(ctx, account); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing an undeliverable address: %w", err))
	}
	return nil
}
