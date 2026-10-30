// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

var _ repository.Approvals = ServiceDeskApprovals{}

// ServiceDeskApprovals stores approvals, their decisions and delegations (migration 0268).
type ServiceDeskApprovals struct{}

func (ServiceDeskRepository) Approvals() ServiceDeskApprovals { return ServiceDeskApprovals{} }

// approvalLevelDocument is one level as the approval opened: the step's level in the shape's own
// spelling and the people counted then.
type approvalLevelDocument struct {
	Level   workflowApprovalLevelDocument `json:"level"`
	Counted []string                      `json:"counted"`
}

func levelsColumn(levels []servicedesk.OpenLevel) ([]byte, error) {
	doc := make([]approvalLevelDocument, 0, len(levels))
	for _, level := range levels {
		counted := make([]string, 0, len(level.Counted))
		for _, id := range level.Counted {
			counted = append(counted, id.String())
		}
		l := level.Level
		doc = append(doc, approvalLevelDocument{Counted: counted, Level: workflowApprovalLevelDocument{
			People: l.People, Groups: l.Groups, Roles: l.Roles, PersonFields: l.PersonFields, Rule: string(l.Rule), Count: l.Count,
		}})
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("postgres.encode_failed").WithCause(err)
	}
	return encoded, nil
}

func levelsFrom(raw []byte) ([]servicedesk.OpenLevel, error) {
	var doc []approvalLevelDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, shared.ErrInternal.WithDetail("postgres.decode_failed").WithCause(err)
	}
	out := make([]servicedesk.OpenLevel, 0, len(doc))
	for _, level := range doc {
		counted := make([]shared.ID, 0, len(level.Counted))
		for _, id := range level.Counted {
			counted = append(counted, shared.ID(id))
		}
		l := level.Level
		out = append(out, servicedesk.OpenLevel{Counted: counted, Level: work.ApprovalLevel{
			People: l.People, Groups: l.Groups, Roles: l.Roles, PersonFields: l.PersonFields, Rule: work.ApprovalRule(l.Rule), Count: l.Count,
		}})
	}
	return out, nil
}

func approvalFromRow(row sqlc.Approval) (servicedesk.Approval, error) {
	ids := make([]shared.ID, 4)
	for i, raw := range []pgtype.UUID{row.ID, row.TenantID, row.ItemID, row.RequestedBy} {
		id, err := idFrom(raw)
		if err != nil {
			return servicedesk.Approval{}, err
		}
		ids[i] = id
	}
	levels, err := levelsFrom(row.Levels)
	if err != nil {
		return servicedesk.Approval{}, err
	}
	a := servicedesk.Approval{
		ID: ids[0], TenantID: ids[1], ItemID: ids[2], Workflow: row.WorkflowKey, Step: row.StepKey,
		ItemVersion: int(row.ItemVersion), Levels: levels, DeclineTo: row.DeclineTo, DueHours: int(row.DueHours),
		Current: int(row.CurrentLevel), State: servicedesk.ApprovalState(row.State), RequestedBy: ids[3],
		RequestedAt: timeFrom(row.RequestedAt), LevelSince: timeFrom(row.LevelSince), Version: int(row.Version),
	}
	if row.DecidedAt.Valid {
		at := row.DecidedAt.Time
		a.DecidedAt = &at
	}
	return a, nil
}

func approvalsFrom(rows []sqlc.Approval) ([]servicedesk.Approval, error) {
	out := make([]servicedesk.Approval, 0, len(rows))
	for _, row := range rows {
		a, err := approvalFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (ServiceDeskApprovals) Insert(ctx context.Context, a servicedesk.Approval) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	uuids, err := uuidsOf([]shared.ID{a.ID, a.TenantID, a.ItemID, a.RequestedBy})
	if err != nil {
		return err
	}
	levels, err := levelsColumn(a.Levels)
	if err != nil {
		return err
	}
	err = queries.InsertApproval(ctx, sqlc.InsertApprovalParams{
		ID: uuids[0], TenantID: uuids[1], ItemID: uuids[2], WorkflowKey: a.Workflow, StepKey: a.Step,
		ItemVersion: int32(a.ItemVersion), Levels: levels, DeclineTo: a.DeclineTo, //nolint:gosec // an entry's version
		DueHours: int32(a.DueHours), RequestedBy: uuids[3], RequestedAt: timestampOf(a.RequestedAt), //nolint:gosec // bounded by the domain
	})
	switch {
	case err == nil:
		return nil
	case isUniqueViolation(err):
		return shared.ErrConflict.WithDetail("approvals.pending")
	case isForeignKeyViolation(err):
		return shared.ErrNotFound.WithDetail("items.not_found")
	default:
		return queryFailed("writing an approval", err)
	}
}

func (ServiceDeskApprovals) Update(ctx context.Context, a servicedesk.Approval, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(a.ID)
	if err != nil {
		return err
	}
	rows, err := queries.UpdateApproval(ctx, sqlc.UpdateApprovalParams{
		ID: id, CurrentLevel: int16(a.Current), State: string(a.State), LevelSince: timestampOf(a.LevelSince), //nolint:gosec // at most five levels
		DecidedAt: optionalTimestamp(a.DecidedAt), ExpectedVersion: int32(expectedVersion), //nolint:gosec // a row's version
	})
	if err != nil {
		return queryFailed("updating an approval", err)
	}
	if rows == 0 {
		return shared.ErrConflict.WithDetail("approvals.changed")
	}
	return nil
}

func (ServiceDeskApprovals) one(ctx context.Context, read func(*sqlc.Queries, pgtype.UUID) (sqlc.Approval, error), id shared.ID) (servicedesk.Approval, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.Approval{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return servicedesk.Approval{}, err
	}
	row, err := read(queries, key)
	if err != nil {
		if IsNoRows(err) {
			return servicedesk.Approval{}, shared.ErrNotFound.WithDetail("approvals.not_found")
		}
		return servicedesk.Approval{}, queryFailed("reading an approval", err)
	}
	return approvalFromRow(row)
}

func (r ServiceDeskApprovals) Find(ctx context.Context, id shared.ID) (servicedesk.Approval, error) {
	return r.one(ctx, func(q *sqlc.Queries, key pgtype.UUID) (sqlc.Approval, error) { return q.FindApproval(ctx, key) }, id)
}

func (r ServiceDeskApprovals) PendingOf(ctx context.Context, itemID shared.ID) (servicedesk.Approval, error) {
	return r.one(ctx, func(q *sqlc.Queries, key pgtype.UUID) (sqlc.Approval, error) {
		return q.PendingApprovalOfItem(ctx, key)
	}, itemID)
}

func (ServiceDeskApprovals) OfItem(ctx context.Context, itemID shared.ID) ([]servicedesk.Approval, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListApprovalsOfItem(ctx, key)
	if err != nil {
		return nil, queryFailed("listing an entry's approvals", err)
	}
	return approvalsFrom(rows)
}

func (ServiceDeskApprovals) Pending(ctx context.Context, limit int) ([]servicedesk.Approval, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPendingApprovals(ctx, int32(limit)) //nolint:gosec // bounded by the use case
	if err != nil {
		return nil, queryFailed("listing the pending approvals", err)
	}
	return approvalsFrom(rows)
}

func (ServiceDeskApprovals) Record(ctx context.Context, d servicedesk.Decision) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	uuids, err := uuidsOf([]shared.ID{d.ID, d.TenantID, d.ApprovalID, d.ApproverID})
	if err != nil {
		return err
	}
	behalf, err := optionalUUID(d.OnBehalfOf)
	if err != nil {
		return err
	}
	err = queries.InsertApprovalDecision(ctx, sqlc.InsertApprovalDecisionParams{
		ID: uuids[0], TenantID: uuids[1], ApprovalID: uuids[2], Level: int16(d.Level), ApproverID: uuids[3], //nolint:gosec // at most five levels
		OnBehalfOf: behalf, Verdict: string(d.Verdict), Reason: d.Reason, ItemVersion: int32(d.ItemVersion), //nolint:gosec // an entry's version
		DecidedAt: timestampOf(d.DecidedAt),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.WithDetail("approvals.already_decided")
		}
		return queryFailed("writing an approval decision", err)
	}
	return nil
}

func (ServiceDeskApprovals) Decisions(ctx context.Context, approvalID shared.ID) ([]servicedesk.Decision, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(approvalID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListApprovalDecisions(ctx, key)
	if err != nil {
		return nil, queryFailed("listing an approval's decisions", err)
	}
	out := make([]servicedesk.Decision, 0, len(rows))
	for _, row := range rows {
		ids := make([]shared.ID, 4)
		for i, raw := range []pgtype.UUID{row.ID, row.ApprovalID, row.ApproverID, row.TenantID} {
			if ids[i], err = idFrom(raw); err != nil {
				return nil, err
			}
		}
		behalf, err := optionalID(row.OnBehalfOf)
		if err != nil {
			return nil, err
		}
		out = append(out, servicedesk.Decision{
			ID: ids[0], TenantID: ids[3], ApprovalID: ids[1], Level: int(row.Level), ApproverID: ids[2], OnBehalfOf: behalf,
			Verdict: servicedesk.Verdict(row.Verdict), Reason: row.Reason, ItemVersion: int(row.ItemVersion),
			DecidedAt: timeFrom(row.DecidedAt),
		})
	}
	return out, nil
}

func delegationFrom(row sqlc.ApprovalDelegation) (servicedesk.Delegation, error) {
	ids := make([]shared.ID, 3)
	for i, raw := range []pgtype.UUID{row.TenantID, row.AccountID, row.DelegateID} {
		id, err := idFrom(raw)
		if err != nil {
			return servicedesk.Delegation{}, err
		}
		ids[i] = id
	}
	return servicedesk.Delegation{TenantID: ids[0], AccountID: ids[1], DelegateID: ids[2], From: timeFrom(row.StartsAt), Until: timeFrom(row.EndsAt)}, nil
}

func (ServiceDeskApprovals) Delegate(ctx context.Context, d servicedesk.Delegation) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	uuids, err := uuidsOf([]shared.ID{d.TenantID, d.AccountID, d.DelegateID})
	if err != nil {
		return err
	}
	if err := queries.SaveApprovalDelegation(ctx, sqlc.SaveApprovalDelegationParams{
		TenantID: uuids[0], AccountID: uuids[1], DelegateID: uuids[2], StartsAt: timestampOf(d.From), EndsAt: timestampOf(d.Until),
	}); err != nil {
		return queryFailed("writing a delegation", err)
	}
	return nil
}

func (ServiceDeskApprovals) DelegationOf(ctx context.Context, accountID shared.ID) (servicedesk.Delegation, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.Delegation{}, err
	}
	key, err := uuidOf(accountID)
	if err != nil {
		return servicedesk.Delegation{}, err
	}
	row, err := queries.FindApprovalDelegation(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return servicedesk.Delegation{}, shared.ErrNotFound.WithDetail("approvals.no_delegation")
		}
		return servicedesk.Delegation{}, queryFailed("reading a delegation", err)
	}
	return delegationFrom(row)
}

func (ServiceDeskApprovals) DelegationsTo(ctx context.Context, delegateID shared.ID) ([]servicedesk.Delegation, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(delegateID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.DelegationsTo(ctx, key)
	if err != nil {
		return nil, queryFailed("listing the delegations to a person", err)
	}
	out := make([]servicedesk.Delegation, 0, len(rows))
	for _, row := range rows {
		d, err := delegationFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (ServiceDeskApprovals) EndDelegation(ctx context.Context, accountID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	rows, err := queries.DeleteApprovalDelegation(ctx, key)
	if err != nil {
		return false, queryFailed("ending a delegation", err)
	}
	return rows > 0, nil
}
