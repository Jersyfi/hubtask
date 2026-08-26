// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The reading half of the trail (E-09, audit.md §5).
//
// Its own type beside AuditSink rather than a second method on it, and that is the same decision
// the port makes: the sink is a dependency of every use case that writes anything, and a sink that
// could also read would put the whole trail one method call away from code that has no business
// reading it.
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement (ADR-0010) - the statement repeats the condition all the
// same, because `audit_time_idx` and `audit_id_uq` both begin with `tenant_id` and a query without
// it would be a sequential scan over every partition.
type AuditTrailRepository struct {
	cursors security.CursorCodec
}

func NewAuditTrailRepository(cursors security.CursorCodec) AuditTrailRepository {
	return AuditTrailRepository{cursors: cursors}
}

var _ repository.Trail = AuditTrailRepository{}

// Query answers one page of the trail, newest first.
func (r AuditTrailRepository) Query(
	ctx context.Context, filter repository.Filter,
) (repository.RecordPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.RecordPage{}, err
	}

	params, err := auditQueryParams(r.cursors, filter)
	if err != nil {
		return repository.RecordPage{}, err
	}

	rows, err := queries.ListAuditEntries(ctx, params)
	if err != nil {
		return repository.RecordPage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the audit trail: %w", err))
	}

	records := make([]repository.Record, 0, len(rows))
	for _, row := range rows {
		record, err := auditRecordFrom(row)
		if err != nil {
			return repository.RecordPage{}, err
		}
		records = append(records, record)
	}

	kept, info := pageOf(records, filter.Page.Size, r.cursors,
		func(record repository.Record) security.Position {
			return security.At(record.Entry.OccurredAt.UTC().Format(time.RFC3339Nano), record.ID)
		})
	return repository.RecordPage{
		Records: kept,
		Info:    repository.PageInfo{NextCursor: info.NextCursor, HasMore: info.HasMore},
	}, nil
}

// walkBatch is how many entries one round trip of a walk reads.
//
// Large enough that a year of a busy tenant is a few hundred round trips rather than tens of
// thousands, small enough that one batch is a few hundred kilobytes rather than a holding. It is
// not the page size of a list: nobody is looking at these rows, they are being hashed.
const walkBatch = 500

// Walk hands over every entry of a period, oldest first.
//
// It pages on the same keyset the list descends by, so a walk that runs for minutes while entries
// are being appended can neither repeat an entry nor skip one. Entries appended *during* the walk
// are outside its period or after its cursor, which is the same answer a snapshot would give and
// costs no open transaction.
func (r AuditTrailRepository) Walk(
	ctx context.Context, period repository.Period, yield func(repository.Record) error,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	params := sqlc.WalkAuditEntriesParams{
		FromTime:  timestampOf(period.From),
		ToTime:    timestampOf(period.To),
		BatchSize: walkBatch,
	}

	for {
		rows, err := queries.WalkAuditEntries(ctx, params)
		if err != nil {
			return shared.ErrUnavailable.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("walking the audit trail: %w", err))
		}
		if len(rows) == 0 {
			return nil
		}

		for _, row := range rows {
			record, err := auditRecordFrom(row)
			if err != nil {
				return err
			}
			if err := yield(record); err != nil {
				return err
			}
		}

		last := rows[len(rows)-1]
		params.CursorOccurredAt, params.CursorID = last.OccurredAt, last.ID
		if len(rows) < walkBatch {
			// A short batch is the end of the period. Asking once more would be one round trip per
			// walk to be told what this already says.
			return nil
		}
	}
}

// LatestAnchor answers the last chain end this tenant exported outside the database.
func (r AuditTrailRepository) LatestAnchor(ctx context.Context) (repository.Anchor, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Anchor{}, err
	}

	row, err := queries.LastAuditAnchor(ctx)
	switch {
	case IsNoRows(err):
		// Nothing anchored, which is every installation today: external anchoring is optional and
		// is not pretended to be in place (audit.md §3, open point A-2).
		return repository.Anchor{}, nil
	case err != nil:
		return repository.Anchor{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the audit anchor: %w", err))
	}

	return repository.Anchor{
		AnchoredAt: timeFrom(row.AnchoredAt), LastSeq: row.LastSeq, ChainHash: row.ChainHash,
	}, nil
}

// auditQueryParams turns the filter into bound parameters, one per condition.
//
// Every absent filter reaches the statement as NULL, which is what makes the condition disappear
// without the statement being assembled from strings (rule 9, T-06). The alternative - building a
// WHERE out of the filters a caller happened to send - is the shape that eventually concatenates
// one of them.
func auditQueryParams(
	cursors security.CursorCodec, filter repository.Filter,
) (sqlc.ListAuditEntriesParams, error) {
	var params sqlc.ListAuditEntriesParams

	actorID, err := optionalUUID(filter.ActorID)
	if err != nil {
		return params, err
	}
	targetID, err := optionalUUID(filter.TargetID)
	if err != nil {
		return params, err
	}
	boundary, err := auditCursor(cursors, filter.Page.Cursor)
	if err != nil {
		return params, err
	}

	return sqlc.ListAuditEntriesParams{
		FromTime:         timestampOf(filter.From),
		ToTime:           timestampOf(filter.To),
		ActionPrefix:     optionalText(filter.ActionPrefix),
		ActorID:          actorID,
		TargetType:       optionalText(filter.TargetType),
		TargetID:         targetID,
		Outcome:          optionalText(string(filter.Outcome)),
		CursorOccurredAt: boundary.occurredAt,
		CursorID:         boundary.id,
		PageSize:         pageProbe(filter.Page.Size),
	}, nil
}

// auditBoundary is a decoded cursor: the moment and the entry the page continues after.
type auditBoundary struct {
	occurredAt pgtype.Timestamptz
	id         pgtype.UUID
}

func auditCursor(cursors security.CursorCodec, cursor string) (auditBoundary, error) {
	if cursor == "" {
		return auditBoundary{}, nil
	}

	position, err := cursors.Decode(cursor)
	if err != nil {
		return auditBoundary{}, err
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, position.SortKey())
	if err != nil {
		return auditBoundary{}, shared.ErrValidation.
			WithDetail("shared.cursor_invalid").WithCause(err)
	}
	id, err := uuidOf(position.ID)
	if err != nil {
		return auditBoundary{}, err
	}
	return auditBoundary{occurredAt: timestampOf(occurredAt), id: id}, nil
}

// auditRecordFrom maps one row onto the record, including the two hashes.
//
// The tenant is read back here, which no other repository in this package does: the row was found
// through row level security and is this tenant's by construction, so everywhere else the column
// would be the database being asked to confirm itself. Here it is an input to the digest - the
// hash was taken over the value in the row, and a verifier that used its own context instead would
// be checking what it expected rather than what is written down.
func auditRecordFrom(row sqlc.AuditLog) (repository.Record, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return repository.Record{}, err
	}
	actorID, err := optionalID(row.ActorID)
	if err != nil {
		return repository.Record{}, err
	}
	onBehalfOf, err := optionalID(row.OnBehalfOfID)
	if err != nil {
		return repository.Record{}, err
	}
	targetID, err := optionalID(row.TargetID)
	if err != nil {
		return repository.Record{}, err
	}
	if !row.OccurredAt.Valid {
		// The column is NOT NULL, so this is unreachable - and unreachable is what to refuse
		// rather than default, because the zero time it would default to is a moment in 1970 that
		// would sort the entry to the start of every period.
		return repository.Record{}, shared.ErrInternal.WithDetail("postgres.row_incoherent")
	}

	auditContext, err := auditContextFrom(row.Context)
	if err != nil {
		return repository.Record{}, err
	}
	changes, err := auditChangesFrom(row.Changes)
	if err != nil {
		return repository.Record{}, err
	}

	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return repository.Record{}, err
	}

	return repository.Record{
		ID:  id,
		Seq: row.Seq,
		Entry: port.Entry{
			TenantID:    tenantID,
			OccurredAt:  row.OccurredAt.Time.UTC(),
			Action:      port.Action(row.Action),
			Outcome:     port.Outcome(row.Outcome),
			Severity:    port.Severity(row.Severity),
			ActorKind:   shared.ActorKind(row.ActorType),
			ActorID:     actorID,
			ActorLabel:  stringFrom(row.ActorLabel),
			OnBehalfOf:  onBehalfOf,
			TargetType:  stringFrom(row.TargetType),
			TargetID:    targetID,
			TargetLabel: stringFrom(row.TargetLabel),
			Context:     auditContext,
			Changes:     changes,
			LegalBasis:  stringFrom(row.LegalBasis),
		},
		PrevHash: row.PrevHash,
		Hash:     row.Hash,
	}, nil
}

func auditContextFrom(raw []byte) (port.Context, error) {
	var value port.Context
	if len(raw) == 0 {
		return value, nil
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, shared.ErrInternal.
			WithDetail("audit.entry_unreadable").
			WithCause(fmt.Errorf("reading the context of an audit entry: %w", err))
	}
	return value, nil
}

func auditChangesFrom(raw []byte) (map[string]any, error) {
	// An empty object rather than nil, because that is what the sink writes for an entry with no
	// changed fields - and the hash was taken over `{}`, so a verifier that read nil would
	// recompute a different digest for a row nobody touched.
	changes := map[string]any{}
	if len(raw) == 0 {
		return changes, nil
	}
	if err := json.Unmarshal(raw, &changes); err != nil {
		return nil, shared.ErrInternal.
			WithDetail("audit.entry_unreadable").
			WithCause(fmt.Errorf("reading the changes of an audit entry: %w", err))
	}
	return changes, nil
}

// AuditPartitionRepository keeps the trail's partitions conforming (E-09, audit.md §3).
//
// Its own type beside the trail rather than a method on it: this is maintenance of the table, not
// a read of the evidence, and a repository that could do both would put a DDL statement one call
// away from the request path.
type AuditPartitionRepository struct{}

func NewAuditPartitionRepository() AuditPartitionRepository { return AuditPartitionRepository{} }

var _ repository.Partitions = AuditPartitionRepository{}

// Ensure runs the duty for one month.
//
// The work is in the database rather than here, and deliberately: creating a partition of
// `audit_log`, revoking on it and giving it its policy are the owner's rights, and the application
// role holds none of them. What this calls is a SECURITY DEFINER function whose only parameter is
// a date (0043_audit_partition_duty.sql) - which is also why there is no statement built here.
func (r AuditPartitionRepository) Ensure(ctx context.Context, month time.Time) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}

	name, err := queries.EnsureAuditPartition(ctx, pgtype.Date{
		Time: time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC), Valid: true,
	})
	if err != nil {
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("ensuring the audit partition of %s: %w", month.Format("2006-01"), err))
	}
	return name, nil
}
