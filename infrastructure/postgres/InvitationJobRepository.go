// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

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
		if len(row.LastErrorParams) > 0 {
			if err := json.Unmarshal(row.LastErrorParams, &job.Params); err != nil {
				job.Params = nil
			}
		}
		out = append(out, job)
	}
	return out, nil
}
