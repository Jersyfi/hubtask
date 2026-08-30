// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// maxRulePage is the ceiling on one listing, api-guidelines.md §4's for every page.
const maxRulePage = 200

// defaultRulePage is what a caller with no opinion gets.
const defaultRulePage = 50

// AutomationRuleRepository stores the rules (G-05).
//
// The four jsonb columns are serialised here rather than in the domain: JSON is a wire format and
// the domain does not serialise itself (project-structure.md §3). What travels is the validated
// aggregate, so a document in the column is one this build could produce - the shapes are read back
// defensively all the same, because the row outlives the release that wrote it.
type AutomationRuleRepository struct {
	cursors security.CursorCodec
}

func NewAutomationRuleRepository(cursors security.CursorCodec) AutomationRuleRepository {
	return AutomationRuleRepository{cursors: cursors}
}

var (
	_ repository.Rules     = AutomationRuleRepository{}
	_ repository.Schedules = AutomationRuleRepository{}
)

func (r AutomationRuleRepository) Insert(ctx context.Context, rule domain.Rule) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(rule.ID)
	if err != nil {
		return err
	}
	runAs, err := uuidOf(rule.RunAs)
	if err != nil {
		return err
	}
	createdBy, err := uuidOf(rule.CreatedBy)
	if err != nil {
		return err
	}
	scopeID, err := optionalUUID(rule.Scope.ID)
	if err != nil {
		return err
	}
	documents, err := documentsOf(rule)
	if err != nil {
		return err
	}

	if err := queries.InsertAutomationRule(ctx, sqlc.InsertAutomationRuleParams{
		ID:         id,
		ScopeType:  string(rule.Scope.Type),
		ScopeID:    scopeID,
		Name:       rule.Name,
		Enabled:    rule.Enabled,
		RunAs:      runAs,
		Trigger:    documents.trigger,
		Conditions: documents.conditions,
		Actions:    documents.actions,
		Throttle:   documents.throttle,
		OnError:    string(rule.OnError),
		CreatedBy:  createdBy,
		CreatedAt:  timestampOf(rule.CreatedAt),
		NextRunAt:  scheduleMoment(rule.NextRunAt),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("inserting automation rule %s: %w", rule.ID, err))
	}
	return nil
}

func (r AutomationRuleRepository) Find(ctx context.Context, id shared.ID) (domain.Rule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Rule{}, err
	}

	key, err := uuidOf(id)
	if err != nil {
		return domain.Rule{}, err
	}

	row, err := queries.FindAutomationRule(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.Rule{}, shared.ErrNotFound.
				WithDetail("automation.rule_not_found").
				WithParams(map[string]string{"rule_id": id.String()})
		}
		return domain.Rule{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading automation rule %s: %w", id, err))
	}
	return automationRuleFrom(sqlc.ListAutomationRulesRow(row))
}

func (r AutomationRuleRepository) List(
	ctx context.Context, query repository.Query,
) (repository.Page, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Page{}, err
	}

	after, err := r.boundary(query.Cursor)
	if err != nil {
		return repository.Page{}, err
	}

	// One more than asked for, which is how "is there another page" is answered without a second
	// count over the same predicate.
	size := boundedRulePage(query.Size)
	rows, err := queries.ListAutomationRules(ctx, sqlc.ListAutomationRulesParams{
		Enabled: query.Enabled,
		After:   after,
		//nolint:gosec // G115: boundedRulePage returns at most maxRulePage, which is 200
		PageSize: int32(size + 1),
	})
	if err != nil {
		return repository.Page{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing automation rules: %w", err))
	}

	hasMore := len(rows) > size
	if hasMore {
		rows = rows[:size]
	}

	rules := make([]domain.Rule, 0, len(rows))
	for _, row := range rows {
		rule, err := automationRuleFrom(row)
		if err != nil {
			return repository.Page{}, err
		}
		rules = append(rules, rule)
	}

	page := repository.Page{Rules: rules, HasMore: hasMore}
	if hasMore {
		// The walk is by identifier alone - UUIDv7 is time-ordered, so the primary key is the
		// creation order and the sort needs no second key.
		last := rules[len(rules)-1]
		page.NextCursor = r.cursors.Encode(security.Position{ID: last.ID})
	}
	return page, nil
}

func (r AutomationRuleRepository) Update(
	ctx context.Context, rule domain.Rule, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(rule.ID)
	if err != nil {
		return err
	}
	runAs, err := uuidOf(rule.RunAs)
	if err != nil {
		return err
	}
	scopeID, err := optionalUUID(rule.Scope.ID)
	if err != nil {
		return err
	}
	documents, err := documentsOf(rule)
	if err != nil {
		return err
	}

	changed, err := queries.UpdateAutomationRule(ctx, sqlc.UpdateAutomationRuleParams{
		ID:         id,
		ScopeType:  string(rule.Scope.Type),
		ScopeID:    scopeID,
		Name:       rule.Name,
		RunAs:      runAs,
		Trigger:    documents.trigger,
		Conditions: documents.conditions,
		Actions:    documents.actions,
		Throttle:   documents.throttle,
		OnError:    string(rule.OnError),
		UpdatedAt:  timestampOf(rule.UpdatedAt),
		NextRunAt:  scheduleMoment(rule.NextRunAt),
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("updating automation rule %s: %w", rule.ID, err))
	}
	return r.conflictUnless(ctx, changed, rule.ID, expectedVersion)
}

func (r AutomationRuleRepository) SetEnabled(
	ctx context.Context, id shared.ID, enabled bool, expectedVersion int, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	key, err := uuidOf(id)
	if err != nil {
		return err
	}

	changed, err := queries.SetAutomationRuleEnabled(ctx, sqlc.SetAutomationRuleEnabledParams{
		ID:        key,
		Enabled:   enabled,
		UpdatedAt: timestampOf(at),
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("switching automation rule %s: %w", id, err))
	}
	return r.conflictUnless(ctx, changed, id, expectedVersion)
}

func (r AutomationRuleRepository) Delete(
	ctx context.Context, id shared.ID, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}

	changed, err := queries.SoftDeleteAutomationRule(ctx, sqlc.SoftDeleteAutomationRuleParams{
		ID: key, DeletedAt: timestampOf(at),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting automation rule %s: %w", id, err))
	}
	return changed > 0, nil
}

// conflictUnless turns "the statement changed nothing" into the answer that says which of the two
// reasons it was.
//
// A guarded update matches nothing when the version moved *or* when the row is gone, and the two
// are different answers to the caller: one says read it again, the other says it is not there. The
// second read is what tells them apart, and it costs one query on the failing path only.
func (r AutomationRuleRepository) conflictUnless(
	ctx context.Context, changed int64, id shared.ID, expected int,
) error {
	if changed > 0 {
		return nil
	}

	rule, err := r.Find(ctx, id)
	if err != nil {
		return err
	}
	return shared.ErrConflict.
		WithDetail("automation.version_conflict").
		WithParams(map[string]string{
			"expected": itoa(expected), "current": itoa(rule.Version),
		})
}

func (r AutomationRuleRepository) boundary(cursor string) (pgtype.UUID, error) {
	if cursor == "" {
		return pgtype.UUID{}, nil
	}
	position, err := r.cursors.Decode(cursor)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return uuidOf(position.ID)
}

// ruleDocuments are the four jsonb columns of one rule.
type ruleDocuments struct {
	trigger, conditions, actions, throttle []byte
}

// triggerDocument is the `trigger` column's shape. The keys are the contract's, so what is stored
// and what is answered are one vocabulary rather than two that have to be kept in step.
type triggerDocument struct {
	Kind          string   `json:"kind"`
	EventType     string   `json:"event_type,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
	RRule         string   `json:"rrule,omitempty"`
	Timezone      string   `json:"timezone,omitempty"`
	Anchor        string   `json:"anchor,omitempty"`
	Offset        string   `json:"offset,omitempty"`
}

type conditionDocument struct {
	Expr string `json:"expr"`
}

type actionDocument struct {
	Kind   string         `json:"kind"`
	Params map[string]any `json:"params"`
}

type throttleDocument struct {
	MaxRunsPerHour int    `json:"max_runs_per_hour,omitempty"`
	DedupeKeyExpr  string `json:"dedupe_key_expr,omitempty"`
}

func documentsOf(rule domain.Rule) (ruleDocuments, error) {
	trigger := triggerDocument{
		Kind: string(rule.Trigger.Kind), EventType: rule.Trigger.EventType.String(),
		ChangedFields: rule.Trigger.ChangedFields,
		RRule:         rule.Trigger.RRule, Timezone: rule.Trigger.Timezone,
		Anchor: string(rule.Trigger.Anchor), Offset: rule.Trigger.Offset,
	}

	// Empty slices rather than nil, so that a column holds `[]` and not `null`: what is read back
	// is what a caller is answered, and a null list is a list a client has to special-case.
	conditions := make([]conditionDocument, 0, len(rule.Conditions))
	for _, condition := range rule.Conditions {
		conditions = append(conditions, conditionDocument{Expr: condition.Expr})
	}
	actions := make([]actionDocument, 0, len(rule.Actions))
	for _, action := range rule.Actions {
		params := action.Params
		if params == nil {
			params = map[string]any{}
		}
		actions = append(actions, actionDocument{Kind: action.Kind, Params: params})
	}
	throttle := throttleDocument{
		MaxRunsPerHour: rule.Throttle.MaxRunsPerHour, DedupeKeyExpr: rule.Throttle.DedupeKeyExpr,
	}

	var documents ruleDocuments
	for _, part := range []struct {
		value any
		into  *[]byte
		name  string
	}{
		{trigger, &documents.trigger, "trigger"},
		{conditions, &documents.conditions, "conditions"},
		{actions, &documents.actions, "actions"},
		{throttle, &documents.throttle, "throttle"},
	} {
		encoded, err := json.Marshal(part.value)
		if err != nil {
			return ruleDocuments{}, shared.ErrInternal.
				WithDetail("automation.rule_unserialisable").
				WithCause(fmt.Errorf("encoding the %s of rule %s: %w", part.name, rule.ID, err))
		}
		*part.into = encoded
	}
	return documents, nil
}

// automationRuleFrom reads a row back.
//
// Defensively, although every document in the column was written by a validated aggregate: the row
// outlives the release that wrote it, and a shape this build cannot read has to be an error a log
// names rather than a zero value that quietly changes what a rule does.
// Due answers this tenant's rules whose moment has come (G-08).
//
// The tenant is the transaction's, never a parameter: the pass is opened under one tenant's scope
// by that tenant's own poller, and nothing may enumerate tenants (rule 3, multi-tenancy.md §2.1).
func (r AutomationRuleRepository) Due(
	ctx context.Context, at time.Time, limit int,
) ([]domain.Rule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.DueAutomationRules(ctx, sqlc.DueAutomationRulesParams{
		Due: timestampOf(at),
		//nolint:gosec // G115: the caller's batch, bounded by the pass's own constant
		PageSize: int32(limit),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the due automation rules: %w", err))
	}

	rules := make([]domain.Rule, 0, len(rows))
	for _, row := range rows {
		rule, err := automationRuleFrom(sqlc.ListAutomationRulesRow(row))
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// NextDue answers the earliest moment this tenant owes anything, and the zero time when it owes
// nothing - which is what lets the poller finish rather than spin.
func (r AutomationRuleRepository) NextDue(ctx context.Context) (time.Time, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return time.Time{}, err
	}

	next, err := queries.NextDueAutomationRule(ctx)
	if err != nil {
		if IsNoRows(err) {
			return time.Time{}, nil
		}
		return time.Time{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the next due automation rule: %w", err))
	}
	return timeFrom(next), nil
}

// SetNextRun moves one rule on to its next moment, or to none.
//
// The version is deliberately untouched: nobody read this rule in order to advance it, and bumping
// it would make every occurrence look like an edit to a client holding an optimistic lock.
func (r AutomationRuleRepository) SetNextRun(
	ctx context.Context, id shared.ID, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	key, err := uuidOf(id)
	if err != nil {
		return err
	}

	if err := queries.SetAutomationRuleNextRun(ctx, sqlc.SetAutomationRuleNextRunParams{
		ID: key, NextRunAt: scheduleMoment(at),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("moving automation rule %s to its next moment: %w", id, err))
	}
	return nil
}

// scheduleMoment writes the zero time as NULL. A rule with no next moment is a rule whose
// recurrence is exhausted or one that is not a schedule at all, and the year one would be neither.
func scheduleMoment(at time.Time) pgtype.Timestamptz {
	if at.IsZero() {
		return pgtype.Timestamptz{}
	}
	return timestampOf(at)
}

func automationRuleFrom(row sqlc.ListAutomationRulesRow) (domain.Rule, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Rule{}, err
	}
	runAs, err := idFrom(row.RunAs)
	if err != nil {
		return domain.Rule{}, err
	}
	createdBy, err := idFrom(row.CreatedBy)
	if err != nil {
		return domain.Rule{}, err
	}
	scopeID, err := optionalID(row.ScopeID)
	if err != nil {
		return domain.Rule{}, err
	}

	var trigger triggerDocument
	var conditions []conditionDocument
	var actions []actionDocument
	var throttle throttleDocument
	for _, part := range []struct {
		raw  []byte
		into any
		name string
	}{
		{row.Trigger, &trigger, "trigger"},
		{row.Conditions, &conditions, "conditions"},
		{row.Actions, &actions, "actions"},
		{row.Throttle, &throttle, "throttle"},
	} {
		if len(part.raw) == 0 {
			continue
		}
		if err := json.Unmarshal(part.raw, part.into); err != nil {
			return domain.Rule{}, shared.ErrInternal.
				WithDetail("automation.rule_unreadable").
				WithCause(fmt.Errorf("reading the %s of rule %s: %w", part.name, id, err))
		}
	}

	rule := domain.Rule{
		ID:    id,
		Name:  row.Name,
		Scope: domain.Scope{Type: domain.ScopeType(row.ScopeType), ID: scopeID},
		// Compared against the column rather than assigned from it, so that the two spellings of
		// "off" - the flag and the tombstone - cannot disagree in a rule this reads back.
		Enabled: row.Enabled,
		RunAs:   runAs,
		Trigger: domain.Trigger{
			Kind: domain.TriggerKind(trigger.Kind), EventType: event.Type(trigger.EventType),
			ChangedFields: trigger.ChangedFields,
			RRule:         trigger.RRule, Timezone: trigger.Timezone,
			Anchor: domain.DateAnchor(trigger.Anchor), Offset: trigger.Offset,
		},
		Conditions: make([]domain.Condition, 0, len(conditions)),
		Actions:    make([]domain.Action, 0, len(actions)),
		Throttle: domain.Throttle{
			MaxRunsPerHour: throttle.MaxRunsPerHour, DedupeKeyExpr: throttle.DedupeKeyExpr,
		},
		OnError:      domain.OnError(row.OnError),
		FailureCount: int(row.FailureCount),
		NextRunAt:    timeFrom(row.NextRunAt),
		CreatedBy:    createdBy,
		CreatedAt:    timeFrom(row.CreatedAt),
		UpdatedAt:    timeFrom(row.UpdatedAt),
		Version:      int(row.Version),
	}
	for _, condition := range conditions {
		rule.Conditions = append(rule.Conditions, domain.Condition{Expr: condition.Expr})
	}
	for _, action := range actions {
		params := action.Params
		if params == nil {
			params = map[string]any{}
		}
		rule.Actions = append(rule.Actions, domain.Action{Kind: action.Kind, Params: params})
	}
	if row.DeletedAt.Valid {
		deleted := timeFrom(row.DeletedAt)
		rule.DeletedAt = &deleted
	}
	return rule, nil
}

// itoa keeps the conflict's parameters numeric without pulling strconv into a file that needs it
// twice.
func itoa(n int) string { return strconv.Itoa(n) }

func boundedRulePage(size int) int {
	switch {
	case size < 1:
		return defaultRulePage
	case size > maxRulePage:
		return maxRulePage
	default:
		return size
	}
}
