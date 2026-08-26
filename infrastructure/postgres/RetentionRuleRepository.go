// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// RetentionRuleRepository stores the rule model of data-retention.md §2 (E-07).
type RetentionRuleRepository struct{}

func NewRetentionRuleRepository() RetentionRuleRepository { return RetentionRuleRepository{} }

var _ repository.Rules = RetentionRuleRepository{}

// notifyRow is the advance warning as the column holds it. The names are the contract's, so that
// the stored document and the response say the same words.
type notifyRow struct {
	BeforeDays int      `json:"before_days,omitempty"`
	Recipients []string `json:"recipients,omitempty"`
}

// Insert writes a rule.
func (r RetentionRuleRepository) Insert(ctx context.Context, rule domain.Rule) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(rule.ID)
	if err != nil {
		return err
	}
	scopeID, err := optionalUUID(rule.Scope.ID)
	if err != nil {
		return err
	}
	exportTarget, err := optionalUUID(rule.ExportTargetID)
	if err != nil {
		return err
	}
	createdBy, err := uuidOf(rule.CreatedBy)
	if err != nil {
		return err
	}

	notify := notifyRow{BeforeDays: rule.Notify.BeforeDays}
	for _, recipient := range rule.Notify.Recipients {
		notify.Recipients = append(notify.Recipients, string(recipient))
	}
	encoded, err := json.Marshal(notify)
	if err != nil {
		return shared.Internalf("postgres: a retention rule's warning could not be encoded: %w", err)
	}

	err = queries.InsertRetentionRule(ctx, sqlc.InsertRetentionRuleParams{
		ID: id, ScopeKind: string(rule.Scope.Kind), ScopeID: scopeID,
		DataKind: string(rule.DataKind), Condition: optionalText(rule.Condition),
		//nolint:gosec // G115: bounded by the domain, which refuses a negative period
		RetainDays: int32(rule.RetainDays), Action: string(rule.Action),
		ThenAfterDays: optionalStage(rule.ThenAfterDays),
		ThenAction:    optionalText(string(rule.ThenAction)),
		//nolint:gosec // G115: bounded by the domain, which refuses a negative grace period
		GraceDays: int32(rule.GraceDays), Notify: encoded,
		Justification: optionalText(rule.Justification), Enabled: rule.Enabled,
		ExportTargetID: exportTarget, CreatedBy: createdBy, Now: timestampOf(rule.CreatedAt),
	})
	if err != nil {
		if isUniqueViolation(err) {
			// One rule per kind per level, which the unique index makes true. A conflict rather
			// than an internal error: the caller asked for something the model does not allow, and
			// the answer is which of their rules already covers it.
			return shared.ErrConflict.WithDetail(domain.CodeRuleAlreadyExists).
				WithParams(map[string]string{"data_kind": string(rule.DataKind)})
		}
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the retention rule: %w", err))
	}
	return nil
}

// List answers every rule the tenant has, narrowest scope first.
func (r RetentionRuleRepository) List(ctx context.Context) ([]domain.Rule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListRetentionRules(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the retention rules: %w", err))
	}

	rules := make([]domain.Rule, 0, len(rows))
	for _, row := range rows {
		rule, err := ruleFrom(sqlc.FindRetentionRuleRow(row))
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// Find answers one rule.
func (r RetentionRuleRepository) Find(ctx context.Context, id shared.ID) (domain.Rule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Rule{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Rule{}, err
	}
	row, err := queries.FindRetentionRule(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.Rule{}, shared.ErrNotFound.WithDetail(domain.CodeRuleNotFound).
				WithParams(map[string]string{"policy_id": id.String()})
		}
		return domain.Rule{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the retention rule: %w", err))
	}
	return ruleFrom(row)
}

// CarryOver writes the old table's period as a tenant-wide rule for a tenant that has none.
func (r RetentionRuleRepository) CarryOver(
	ctx context.Context, id shared.ID, kind domain.DataKind, now time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	err = queries.CarryOverRetentionPolicy(ctx, sqlc.CarryOverRetentionPolicyParams{
		ID: key, DataKind: string(kind), Now: timestampOf(now),
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("carrying over the retention period of %s: %w", kind, err))
	}
	return nil
}

func ruleFrom(row sqlc.FindRetentionRuleRow) (domain.Rule, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Rule{}, err
	}
	scopeID, err := optionalID(row.ScopeID)
	if err != nil {
		return domain.Rule{}, err
	}
	exportTarget, err := optionalID(row.ExportTargetID)
	if err != nil {
		return domain.Rule{}, err
	}
	createdBy, err := optionalID(row.CreatedBy)
	if err != nil {
		return domain.Rule{}, err
	}

	var notify notifyRow
	if len(row.Notify) > 0 {
		if err := json.Unmarshal(row.Notify, &notify); err != nil {
			return domain.Rule{}, shared.Internalf(
				"postgres: a retention rule's warning could not be read: %w", err)
		}
	}
	warning := domain.Notify{BeforeDays: notify.BeforeDays}
	for _, recipient := range notify.Recipients {
		warning.Recipients = append(warning.Recipients, domain.Recipient(recipient))
	}

	return domain.Rule{
		ID: id, Scope: domain.Scope{Kind: domain.ScopeKind(row.ScopeKind), ID: scopeID},
		DataKind: domain.DataKind(row.DataKind), Condition: stringFrom(row.Condition),
		RetainDays: int(row.RetainDays), Action: domain.Action(row.Action),
		ThenAfterDays: stageFrom(row.ThenAfterDays),
		ThenAction:    domain.Action(stringFrom(row.ThenAction)),
		GraceDays:     int(row.GraceDays), Notify: warning,
		Justification: stringFrom(row.Justification), Enabled: row.Enabled,
		ExportTargetID: exportTarget, CreatedBy: createdBy,
		CreatedAt: timeFrom(row.CreatedAt), UpdatedAt: timeFrom(row.UpdatedAt),
		Version: int(row.Version),
	}, nil
}

// optionalStage is a chain's second period, absent when there is no second stage.
func optionalStage(value int) *int32 {
	if value == 0 {
		return nil
	}
	//nolint:gosec // G115: bounded by the domain, which refuses a negative period
	count := int32(value)
	return &count
}

func stageFrom(value *int32) int {
	if value == nil {
		return 0
	}
	return int(*value)
}

// idsOf turns a batch into what a statement takes. A pass hands in a thousand
// identities at a time, and building the array once per call is what keeps the statement one round
// trip rather than a thousand.
func idsOf(ids []shared.ID) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		value, err := uuidOf(id)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}
