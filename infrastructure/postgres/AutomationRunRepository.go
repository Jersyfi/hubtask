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

// AutomationRunRepository is the log of what the rules have done (G-07), the failure counter, and
// the per-event lookup the dispatcher makes.
//
// Three ports on one type because they are one table pair, and three ports rather than one because
// the callers differ: a subscriber inside the dispatcher's transaction has no business writing a
// rule, and the writer that manages rules has no business switching one off behind a person's back.
type AutomationRunRepository struct {
	cursors security.CursorCodec
}

func NewAutomationRunRepository(cursors security.CursorCodec) AutomationRunRepository {
	return AutomationRunRepository{cursors: cursors}
}

var (
	_ repository.Runs     = AutomationRunRepository{}
	_ repository.Failures = AutomationRunRepository{}
	_ repository.Matching = AutomationRunRepository{}
)

func (r AutomationRunRepository) Start(ctx context.Context, run domain.Run) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(run.ID)
	if err != nil {
		return err
	}
	ruleID, err := uuidOf(run.RuleID)
	if err != nil {
		return err
	}
	eventID, err := optionalUUID(run.EventID)
	if err != nil {
		return err
	}
	triggeredBy, err := optionalUUID(run.TriggeredBy)
	if err != nil {
		return err
	}
	subjectID, err := optionalUUID(run.SubjectID)
	if err != nil {
		return err
	}
	conditions, actions, err := runDocuments(run)
	if err != nil {
		return err
	}

	if err := queries.InsertRuleRun(ctx, sqlc.InsertRuleRunParams{
		ID: id, RuleID: ruleID, EventID: eventID,
		Trigger: string(run.Trigger), TriggeredBy: triggeredBy, SubjectID: subjectID,
		Occasion:         optionalText(run.Occasion),
		Status:           string(run.Status),
		ConditionResults: conditions,
		ActionResults:    actions,
		StartedAt:        timestampOf(run.StartedAt),
		//nolint:gosec // G115: the depth is bounded by MaxCausationDepth, which is 5
		CausationDepth: int32(run.CausationDepth),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("starting rule run %s: %w", run.ID, err))
	}
	return nil
}

func (r AutomationRunRepository) Finish(ctx context.Context, run domain.Run) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(run.ID)
	if err != nil {
		return err
	}
	conditions, actions, err := runDocuments(run)
	if err != nil {
		return err
	}

	// A parked run has no finished moment, and the column says so: NULL is what keeps a WAITING
	// row honest about not being over (G-09). A finished run without the stamp - which the domain
	// never produces - would fall back to when it started rather than inventing a reading here.
	finished := pgtype.Timestamptz{}
	if run.FinishedAt != nil {
		finished = timestampOf(*run.FinishedAt)
	} else if run.Status.Finished() {
		finished = timestampOf(run.StartedAt)
	}

	if err := queries.FinishRuleRun(ctx, sqlc.FinishRuleRunParams{
		ID: id, Status: string(run.Status),
		ConditionResults: conditions, ActionResults: actions,
		ErrorCode:  optionalText(run.ErrorCode),
		FinishedAt: finished,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("finishing rule run %s: %w", run.ID, err))
	}
	return nil
}

func (r AutomationRunRepository) Find(ctx context.Context, id shared.ID) (domain.Run, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Run{}, err
	}

	key, err := uuidOf(id)
	if err != nil {
		return domain.Run{}, err
	}

	row, err := queries.FindRuleRun(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.Run{}, shared.ErrNotFound.
				WithDetail("automation.run_not_found").
				WithParams(map[string]string{"run_id": id.String()})
		}
		return domain.Run{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading rule run %s: %w", id, err))
	}
	return runFrom(sqlc.ListRuleRunsRow(row))
}

func (r AutomationRunRepository) List(
	ctx context.Context, query repository.RunQuery,
) (repository.RunPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.RunPage{}, err
	}

	after, err := r.boundary(query.Cursor)
	if err != nil {
		return repository.RunPage{}, err
	}
	ruleID, err := optionalUUID(query.RuleID)
	if err != nil {
		return repository.RunPage{}, err
	}

	// One more than asked for, which is how "is there another page" is answered without a second
	// count over the same predicate.
	size := boundedRulePage(query.Size)
	rows, err := queries.ListRuleRuns(ctx, sqlc.ListRuleRunsParams{
		RuleID:  ruleID,
		Status:  optionalText(string(query.Status)),
		Trigger: optionalText(string(query.Trigger)),
		After:   after,
		//nolint:gosec // G115: boundedRulePage returns at most maxRulePage, which is 200
		PageSize: int32(size + 1),
	})
	if err != nil {
		return repository.RunPage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing rule runs: %w", err))
	}

	hasMore := len(rows) > size
	if hasMore {
		rows = rows[:size]
	}

	runs := make([]domain.Run, 0, len(rows))
	for _, row := range rows {
		run, err := runFrom(row)
		if err != nil {
			return repository.RunPage{}, err
		}
		runs = append(runs, run)
	}

	page := repository.RunPage{Runs: runs, HasMore: hasMore}
	if hasMore {
		page.NextCursor = r.cursors.Encode(security.Position{ID: runs[len(runs)-1].ID})
	}
	return page, nil
}

func (r AutomationRunRepository) CountSince(
	ctx context.Context, ruleID shared.ID, since time.Time,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	key, err := uuidOf(ruleID)
	if err != nil {
		return 0, err
	}

	count, err := queries.CountRunsSince(ctx, sqlc.CountRunsSinceParams{
		RuleID: key, Since: timestampOf(since),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting runs of rule %s: %w", ruleID, err))
	}
	return int(count), nil
}

func (r AutomationRunRepository) Bump(
	ctx context.Context, ruleID shared.ID, at time.Time,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	key, err := uuidOf(ruleID)
	if err != nil {
		return 0, err
	}

	count, err := queries.BumpRuleFailure(ctx, sqlc.BumpRuleFailureParams{
		ID: key, At: timestampOf(at),
	})
	if err != nil {
		if IsNoRows(err) {
			// The rule was deleted while the run was in flight. Not an error the run can act on -
			// there is nothing left to disable - and reporting it as one would fail a job whose
			// work is already irrelevant.
			return 0, nil
		}
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the failure of rule %s: %w", ruleID, err))
	}
	return int(count), nil
}

func (r AutomationRunRepository) Clear(ctx context.Context, ruleID shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	key, err := uuidOf(ruleID)
	if err != nil {
		return err
	}

	if err := queries.ClearRuleFailure(ctx, sqlc.ClearRuleFailureParams{
		ID: key, At: timestampOf(at),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the failures of rule %s: %w", ruleID, err))
	}
	return nil
}

func (r AutomationRunRepository) Disable(
	ctx context.Context, ruleID shared.ID, threshold int, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	key, err := uuidOf(ruleID)
	if err != nil {
		return false, err
	}

	changed, err := queries.DisableFailingRule(ctx, sqlc.DisableFailingRuleParams{
		ID: key, At: timestampOf(at),
		//nolint:gosec // G115: the threshold is a constant of the domain, not a value from a request
		Threshold: int32(threshold),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("disabling rule %s: %w", ruleID, err))
	}
	return changed > 0, nil
}

// ByTriggerKind answers this tenant's enabled rules of one kind, which is what a producer that is
// not the event dispatcher asks - the relative-date producer does (G-08).
func (r AutomationRunRepository) ByTriggerKind(
	ctx context.Context, kind domain.TriggerKind,
) ([]domain.Rule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.RulesByTriggerKind(ctx, kind.String())
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the %s rules: %w", kind, err))
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

func (r AutomationRunRepository) ForEventType(
	ctx context.Context, eventType event.Type,
) ([]domain.Rule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.RulesForEventType(ctx, eventType.String())
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("finding the rules for %s: %w", eventType, err))
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

func (r AutomationRunRepository) boundary(cursor string) (pgtype.UUID, error) {
	if cursor == "" {
		return pgtype.UUID{}, nil
	}
	position, err := r.cursors.Decode(cursor)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return uuidOf(position.ID)
}

// conditionDocument and actionDocument are the two jsonb columns' shapes. The keys are the
// contract's, so what is stored and what is answered are one vocabulary.
type conditionResultDocument struct {
	Index     int    `json:"index"`
	Matched   bool   `json:"matched"`
	ErrorCode string `json:"error_code,omitempty"`
}

type actionResultDocument struct {
	Index          int    `json:"index"`
	Kind           string `json:"kind"`
	Path           string `json:"path,omitempty"`
	Matched        *bool  `json:"matched,omitempty"`
	Status         string `json:"status"`
	ErrorCode      string `json:"error_code,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

func runDocuments(run domain.Run) (conditions, actions []byte, err error) {
	// Empty slices rather than nil, so a column holds `[]` and not `null`: what is read back is what
	// a caller is answered, and a null list is a list a client has to special-case.
	conditionRows := make([]conditionResultDocument, 0, len(run.ConditionResults))
	for _, result := range run.ConditionResults {
		conditionRows = append(conditionRows, conditionResultDocument{
			Index: result.Index, Matched: result.Matched, ErrorCode: result.ErrorCode,
		})
	}
	actionRows := make([]actionResultDocument, 0, len(run.ActionResults))
	for _, result := range run.ActionResults {
		actionRows = append(actionRows, actionResultDocument{
			Index: result.Index, Kind: result.Kind, Path: result.Path, Matched: result.Matched,
			Status:    string(result.Status),
			ErrorCode: result.ErrorCode, IdempotencyKey: result.IdempotencyKey,
		})
	}

	if conditions, err = json.Marshal(conditionRows); err != nil {
		return nil, nil, shared.ErrInternal.
			WithDetail("automation.run_unserialisable").
			WithCause(fmt.Errorf("encoding the conditions of run %s: %w", run.ID, err))
	}
	if actions, err = json.Marshal(actionRows); err != nil {
		return nil, nil, shared.ErrInternal.
			WithDetail("automation.run_unserialisable").
			WithCause(fmt.Errorf("encoding the actions of run %s: %w", run.ID, err))
	}
	return conditions, actions, nil
}

// runFrom reads a row back, defensively for automationRuleFrom's reason: the row outlives the
// release that wrote it.
func runFrom(row sqlc.ListRuleRunsRow) (domain.Run, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Run{}, err
	}
	ruleID, err := idFrom(row.RuleID)
	if err != nil {
		return domain.Run{}, err
	}
	eventID, err := optionalID(row.EventID)
	if err != nil {
		return domain.Run{}, err
	}
	triggeredBy, err := optionalID(row.TriggeredBy)
	if err != nil {
		return domain.Run{}, err
	}
	subjectID, err := optionalID(row.SubjectID)
	if err != nil {
		return domain.Run{}, err
	}

	var conditionRows []conditionResultDocument
	var actionRows []actionResultDocument
	for _, part := range []struct {
		raw  []byte
		into any
		name string
	}{
		{row.ConditionResults, &conditionRows, "conditions"},
		{row.ActionResults, &actionRows, "actions"},
	} {
		if len(part.raw) == 0 {
			continue
		}
		if err := json.Unmarshal(part.raw, part.into); err != nil {
			return domain.Run{}, shared.ErrInternal.
				WithDetail("automation.run_unreadable").
				WithCause(fmt.Errorf("reading the %s of run %s: %w", part.name, id, err))
		}
	}

	run := domain.Run{
		ID: id, RuleID: ruleID, EventID: eventID,
		Trigger: domain.TriggerKind(row.Trigger), TriggeredBy: triggeredBy, SubjectID: subjectID,
		Occasion:         stringFrom(row.Occasion),
		Status:           domain.RunStatus(row.Status),
		ConditionResults: make([]domain.ConditionResult, 0, len(conditionRows)),
		ActionResults:    make([]domain.ActionResult, 0, len(actionRows)),
		ErrorCode:        stringFrom(row.ErrorCode),
		StartedAt:        timeFrom(row.StartedAt),
		CausationDepth:   int(row.CausationDepth),
	}
	for _, result := range conditionRows {
		run.ConditionResults = append(run.ConditionResults, domain.ConditionResult{
			Index: result.Index, Matched: result.Matched, ErrorCode: result.ErrorCode,
		})
	}
	for _, result := range actionRows {
		path := result.Path
		if path == "" {
			// A row written before G-09 named actions by index alone, and every action was
			// top-level - where the path is the index.
			path = strconv.Itoa(result.Index)
		}
		run.ActionResults = append(run.ActionResults, domain.ActionResult{
			Index: result.Index, Kind: result.Kind, Path: path, Matched: result.Matched,
			Status:    domain.ActionStatus(result.Status),
			ErrorCode: result.ErrorCode, IdempotencyKey: result.IdempotencyKey,
		})
	}
	if row.FinishedAt.Valid {
		finished := timeFrom(row.FinishedAt)
		run.FinishedAt = &finished
	}
	return run, nil
}
