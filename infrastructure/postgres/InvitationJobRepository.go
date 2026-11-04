// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// InvitationJobRepository reads the queue's last invitation job per account (0362). The job table
// has no row level security, so the statement names the transaction's tenant itself.
type InvitationJobRepository struct{}

func NewInvitationJobRepository() InvitationJobRepository { return InvitationJobRepository{} }

var _ repository.InvitationJobs = InvitationJobRepository{}

// Latest answers the newest invitation job of each account that has one.
func (r InvitationJobRepository) Latest(
	ctx context.Context, accountIDs []shared.ID,
) ([]repository.InvitationJob, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	tenant, err := tenantOfTransaction(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(accountIDs))
	for _, id := range accountIDs {
		keys = append(keys, id.String())
	}
	rows, err := queries.LatestInvitationDeliveries(ctx, sqlc.LatestInvitationDeliveriesParams{
		TenantID: tenant, AccountIds: keys,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading invitation deliveries: %w", err))
	}
	out := make([]repository.InvitationJob, 0, len(rows))
	for _, row := range rows {
		account, err := shared.ParseID(row.AccountID)
		if err != nil {
			// Not an account's key: nothing this read can name.
			continue
		}
		job := repository.InvitationJob{AccountID: account, State: row.State, QueuedAt: row.CreatedAt.Time}
		if row.FinishedAt.Valid {
			job.FinishedAt = row.FinishedAt.Time
		}
		if row.LastError != nil {
			job.Code = *row.LastError
		}
		job.MailState = row.MailState
		if row.MailReason != nil {
			job.MailReason = *row.MailReason
		}
		if row.MailSentAt.Valid {
			job.MailSentAt = row.MailSentAt.Time
		}
		if len(row.LastErrorParams) > 0 {
			if err := json.Unmarshal(row.LastErrorParams, &job.Params); err != nil {
				job.Params = nil
			}
		}
		out = append(out, job)
	}
	return out, nil
}

// CancelOpen cancels the invitation mail still waiting for one account, in the transaction's
// workspace, and sets its unsent record aside; one being sent right now is a conflict.
func (r InvitationJobRepository) CancelOpen(ctx context.Context, accountID shared.ID, now time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	tenant, err := tenantOfTransaction(ctx)
	if err != nil {
		return 0, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return 0, err
	}
	failed := func(what string, err error) error {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("%s: %w", what, err))
	}
	sending, err := queries.InvitationMailSending(ctx, sqlc.InvitationMailSendingParams{TenantID: tenant, AccountID: account})
	if err != nil {
		return 0, failed("reading the invitation mail being sent", err)
	}
	if sending {
		return 0, shared.ErrConflict.WithDetail("accounts.invitation_mail_sending")
	}
	cancelled, err := queries.CancelOpenInvitationMail(ctx, sqlc.CancelOpenInvitationMailParams{
		Now: timestampOf(now), TenantID: tenant, AccountID: account,
	})
	if err != nil {
		return 0, failed("cancelling the invitation mail", err)
	}
	if _, err := queries.SupersedePendingInvitationMail(ctx, sqlc.SupersedePendingInvitationMailParams{
		TenantID: tenant, AccountID: account,
	}); err != nil {
		return 0, failed("setting the unsent invitation mail aside", err)
	}
	return int(cancelled), nil
}
