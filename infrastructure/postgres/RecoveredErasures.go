// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// RecoveredErasuresTimeout bounds the whole read of the temporary cluster (rule 7).
const RecoveredErasuresTimeout = RecoveredHoldsTimeout

// RecoveredErasure is one erasure case as the temporary cluster holds it, and whom it was about.
type RecoveredErasure struct {
	Case domain.Request
	// Unresolved says why the case's subject could not be named: a full deletion whose case no
	// longer names the account, and for which the trail and the journal name none or more than one.
	// Empty when the subject is named, or when the case was about an address nobody held.
	Unresolved string
}

const (
	// SubjectAmbiguous is a full deletion whose moment matches more than one removed account.
	SubjectAmbiguous = "SUBJECT_AMBIGUOUS"
	// SubjectUnresolved is a full deletion whose moment matches no removed account.
	SubjectUnresolved = "SUBJECT_UNRESOLVED"
)

// ReadRecoveredErasures answers one workspace's erasure cases completed after the recovery point,
// as another database holds them: the temporary cluster the operator recovered to the end of the
// WAL archive (backup-restore.md §8.5 step 5). Read through the transaction wrapper under the
// workspace's scope, with the workspace named in the statement too (ReadRecoveredHolds' reason).
func ReadRecoveredErasures(
	ctx context.Context, dsn string, tenantID shared.ID, recoveryPoint time.Time, timeout time.Duration,
) ([]RecoveredErasure, error) {
	if tenantID.IsZero() {
		return nil, shared.ErrInternal.WithDetail("postgres.scope_missing")
	}
	cfg, err := recoveredHoldsSource(dsn, timeout)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.pool_unavailable").
			WithCause(fmt.Errorf("opening the temporary cluster: %w", err))
	}
	defer pool.Close()

	var cases []RecoveredErasure
	err = NewUnitOfWork(pool).WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			queries, err := queriesFrom(ctx)
			if err != nil {
				return err
			}
			rows, err := queries.RecoveredErasureCases(ctx, timestampOf(recoveryPoint))
			if err != nil {
				return shared.ErrUnavailable.WithDetail("postgres.query_failed").
					WithCause(fmt.Errorf("reading the temporary cluster's erasure cases: %w", err))
			}
			cases = make([]RecoveredErasure, 0, len(rows))
			for _, row := range rows {
				recovered, err := recoveredErasureFrom(row)
				if err != nil {
					return err
				}
				cases = append(cases, recovered)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return cases, nil
}

// recoveredErasureFrom names the subject of one case: the one the case names, or - for a full
// deletion that took the account - the one account the erasure removed at the moment of its trail
// entry. None or several is reported, never guessed: erasing the wrong person cannot be undone.
func recoveredErasureFrom(row sqlc.RecoveredErasureCasesRow) (RecoveredErasure, error) {
	request, err := requestFrom(sqlc.ListDataSubjectRequestsRow{
		ID: row.ID, SubjectAccountID: row.SubjectAccountID, SubjectEmail: row.SubjectEmail,
		Kind: string(domain.KindErasure), Status: string(domain.StatusCompleted),
		Scope: string(domain.ScopeTenant), ErasureMode: row.ErasureMode,
		ReceivedAt: row.ReceivedAt, DueAt: row.DueAt, CompletedAt: row.CompletedAt,
		HandledBy: row.HandledBy, Notes: row.Notes, OriginalDueAt: row.OriginalDueAt,
		ExtensionReason: row.ExtensionReason, InformedOn: row.InformedOn,
	})
	if err != nil {
		return RecoveredErasure{}, err
	}
	recovered := RecoveredErasure{Case: request}
	if !request.SubjectAccountID.IsZero() || request.ErasureMode != domain.ModeFullDelete {
		return recovered, nil
	}
	switch len(row.ErasedAccounts) {
	case 1:
		subject, err := idFrom(row.ErasedAccounts[0])
		if err != nil {
			return RecoveredErasure{}, err
		}
		recovered.Case.SubjectAccountID = subject
	case 0:
		if request.SubjectEmail == "" {
			recovered.Unresolved = SubjectUnresolved
		}
	default:
		recovered.Unresolved = SubjectAmbiguous
	}
	return recovered, nil
}
