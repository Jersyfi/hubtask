// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/goal"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// GoalRepository stores periods, goals, linked entries, check-ins, portfolios, their updates and
// dashboards (migration 0248, M8). Key results, a check-in's values and a dashboard's widgets are
// `jsonb`; their shape is this adapter's.
type GoalRepository struct{}

var _ repository.Store = GoalRepository{}

// NewGoalRepository answers the adapter.
func NewGoalRepository() GoalRepository { return GoalRepository{} }

func goalError(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return shared.ErrNotFound.WithDetail("goals.not_found")
	}
	var typed *shared.Error
	if errors.As(err, &typed) {
		return err
	}
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

type storedResult struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Kind    string  `json:"kind"`
	Start   float64 `json:"start"`
	Target  float64 `json:"target"`
	Current float64 `json:"current"`
	Unit    string  `json:"unit,omitempty"`
	Source  string  `json:"source"`
	ViewID  string  `json:"view_id,omitempty"`
	EntryID string  `json:"entry_id,omitempty"`
}

type storedWidget struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Chart   string         `json:"chart"`
	Source  string         `json:"source"`
	ViewID  string         `json:"view_id,omitempty"`
	Query   map[string]any `json:"query,omitempty"`
	GoalIDs []string       `json:"goal_ids,omitempty"`
	// Compare and Cumulative are named as on the wire: the replica hands the column on as it is.
	Compare    *storedCompare `json:"compare,omitempty"`
	Cumulative bool           `json:"cumulative,omitempty"`
	X          int            `json:"x"`
	Y          int            `json:"y"`
	W          int            `json:"w"`
	H          int            `json:"h"`
}

type storedCompare struct {
	FirstTitle string         `json:"first_title"`
	Title      string         `json:"title"`
	Query      map[string]any `json:"query"`
}

func parseIDs(raw ...string) ([]shared.ID, error) {
	out := make([]shared.ID, 0, len(raw))
	for _, r := range raw {
		if r == "" {
			out = append(out, "")
			continue
		}
		id, err := shared.ParseID(r)
		if err != nil {
			return nil, shared.ErrInternal.WithDetail("goals.stored_shape").WithCause(err)
		}
		out = append(out, id)
	}
	return out, nil
}

func encodeResults(results []domain.KeyResult) ([]byte, error) {
	stored := make([]storedResult, 0, len(results))
	for _, r := range results {
		stored = append(stored, storedResult{
			ID: r.ID.String(), Title: r.Title, Kind: string(r.Kind), Start: r.Start, Target: r.Target,
			Current: r.Current, Unit: r.Unit, Source: string(r.Source), ViewID: r.ViewID.String(), EntryID: r.EntryID.String(),
		})
	}
	return json.Marshal(stored)
}

func decodeResults(raw []byte) ([]domain.KeyResult, error) {
	var stored []storedResult
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, shared.ErrInternal.WithDetail("goals.stored_shape").WithCause(err)
	}
	out := make([]domain.KeyResult, 0, len(stored))
	for _, s := range stored {
		ids, err := parseIDs(s.ID, s.ViewID, s.EntryID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.KeyResult{
			ID: ids[0], Title: s.Title, Kind: domain.ResultKind(s.Kind), Start: s.Start, Target: s.Target,
			Current: s.Current, Unit: s.Unit, Source: domain.ResultSource(s.Source), ViewID: ids[1], EntryID: ids[2],
		})
	}
	return out, nil
}

func encodeWidgets(widgets []domain.Widget) ([]byte, error) {
	stored := make([]storedWidget, 0, len(widgets))
	for _, w := range widgets {
		goals := make([]string, 0, len(w.GoalIDs))
		for _, g := range w.GoalIDs {
			goals = append(goals, g.String())
		}
		var compare *storedCompare
		if w.Compare != nil {
			compare = &storedCompare{FirstTitle: w.Compare.FirstTitle, Title: w.Compare.Title, Query: w.Compare.Query}
		}
		stored = append(stored, storedWidget{
			ID: w.ID.String(), Title: w.Title, Chart: string(w.Chart), Source: string(w.Source),
			ViewID: w.ViewID.String(), Query: w.Query, GoalIDs: goals, Compare: compare, Cumulative: w.Cumulative,
			X: w.X, Y: w.Y, W: w.W, H: w.H,
		})
	}
	return json.Marshal(stored)
}

func decodeWidgets(raw []byte) ([]domain.Widget, error) {
	var stored []storedWidget
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, shared.ErrInternal.WithDetail("goals.stored_shape").WithCause(err)
	}
	out := make([]domain.Widget, 0, len(stored))
	for _, s := range stored {
		ids, err := parseIDs(s.ID, s.ViewID)
		if err != nil {
			return nil, err
		}
		goals, err := parseIDs(s.GoalIDs...)
		if err != nil {
			return nil, err
		}
		var compare *domain.Compare
		if s.Compare != nil {
			compare = &domain.Compare{FirstTitle: s.Compare.FirstTitle, Title: s.Compare.Title, Query: s.Compare.Query}
		}
		out = append(out, domain.Widget{
			ID: ids[0], Title: s.Title, Chart: domain.Chart(s.Chart), Source: domain.WidgetSource(s.Source),
			ViewID: ids[1], Query: s.Query, GoalIDs: goals, Compare: compare, Cumulative: s.Cumulative,
			X: s.X, Y: s.Y, W: s.W, H: s.H,
		})
	}
	return out, nil
}

func encodeValues(values map[shared.ID]float64) ([]byte, error) {
	stored := make(map[string]float64, len(values))
	for id, v := range values {
		stored[id.String()] = v
	}
	return json.Marshal(stored)
}

func decodeValues(raw []byte) (map[shared.ID]float64, error) {
	var stored map[string]float64
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, shared.ErrInternal.WithDetail("goals.stored_shape").WithCause(err)
	}
	out := make(map[shared.ID]float64, len(stored))
	for k, v := range stored {
		id, err := shared.ParseID(k)
		if err != nil {
			return nil, shared.ErrInternal.WithDetail("goals.stored_shape").WithCause(err)
		}
		out[id] = v
	}
	return out, nil
}

func ids(values ...pgtype.UUID) ([]shared.ID, error) {
	out := make([]shared.ID, 0, len(values))
	for _, v := range values {
		id, err := optionalID(v)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func uuids(values ...shared.ID) ([]pgtype.UUID, error) { return optionalUUIDs(values...) }

func optionalDate(d *domain.Day) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return planDate(*d)
}

func dateOrNil(v pgtype.Date) *domain.Day {
	if !v.Valid {
		return nil
	}
	d := planDay(v)
	return &d
}

func conflictOrGone(affected int64, code string, expected int) error {
	if affected == 0 {
		return versionConflict(code, expected)
	}
	return nil
}

// Periods answers the periods, the latest first.
func (GoalRepository) Periods(ctx context.Context, limit int) ([]domain.Period, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListGoalPeriods(ctx, int32(limit))
	if err != nil {
		return nil, goalError(err, "reading periods")
	}
	out := make([]domain.Period, 0, len(rows))
	for _, r := range rows {
		id, err := idFrom(r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Period{
			ID: id, Name: r.Name, StartOn: planDay(r.StartOn), EndOn: planDay(r.EndOn),
			State: domain.PeriodState(r.State), Version: int(r.Version),
		})
	}
	return out, nil
}

// Period answers one period.
func (GoalRepository) Period(ctx context.Context, id shared.ID) (domain.Period, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Period{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Period{}, err
	}
	r, err := queries.FindGoalPeriod(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Period{}, shared.ErrNotFound.WithDetail("goal_periods.not_found")
	}
	if err != nil {
		return domain.Period{}, goalError(err, "reading a period")
	}
	return domain.Period{
		ID: id, Name: r.Name, StartOn: planDay(r.StartOn), EndOn: planDay(r.EndOn),
		State: domain.PeriodState(r.State), Version: int(r.Version),
	}, nil
}

// InsertPeriod writes a new period.
func (GoalRepository) InsertPeriod(ctx context.Context, p domain.Period, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(p.ID, by)
	if err != nil {
		return err
	}
	err = queries.InsertGoalPeriod(ctx, sqlc.InsertGoalPeriodParams{
		ID: keys[0], Name: p.Name, StartOn: planDate(p.StartOn), EndOn: planDate(p.EndOn), At: timestampOf(at), By: keys[1],
	})
	return wrapGoal(err, "writing a period")
}

// UpdatePeriod writes an open period's name and dates.
func (GoalRepository) UpdatePeriod(ctx context.Context, p domain.Period, expected int, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(p.ID)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.UpdateGoalPeriod(ctx, sqlc.UpdateGoalPeriodParams{
		Name: p.Name, StartOn: planDate(p.StartOn), EndOn: planDate(p.EndOn), At: timestampOf(at),
		ID: key, ExpectedVersion: int32(expected),
	})
	if err != nil {
		return goalError(err, "writing a period")
	}
	return conflictOrGone(affected, "goal_periods.version_conflict", expected)
}

// ClosePeriod closes an open period and grades its goals.
func (GoalRepository) ClosePeriod(ctx context.Context, id shared.ID, expected int, grades map[shared.ID]float64, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(id, by)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.CloseGoalPeriod(ctx, sqlc.CloseGoalPeriodParams{
		At: timestampOf(at), By: keys[1], ID: keys[0], ExpectedVersion: int32(expected),
	})
	if err != nil {
		return goalError(err, "closing a period")
	}
	if affected == 0 {
		return versionConflict("goal_periods.version_conflict", expected)
	}
	for goal, grade := range grades {
		key, err := uuidOf(goal)
		if err != nil {
			return err
		}
		if _, err := queries.GradeGoal(ctx, sqlc.GradeGoalParams{Grade: grade, At: timestampOf(at), ID: key}); err != nil {
			return goalError(err, "grading a goal")
		}
	}
	return nil
}

func goalOf(r sqlc.ListGoalsRow) (domain.Goal, error) {
	found, err := ids(r.ID, r.PeriodID, r.HubID, r.ParentID, r.OwnerID)
	if err != nil {
		return domain.Goal{}, err
	}
	results, err := decodeResults(r.KeyResults)
	if err != nil {
		return domain.Goal{}, err
	}
	return domain.Goal{
		ID: found[0], PeriodID: found[1], HubID: found[2], ParentID: found[3], OwnerID: found[4],
		Title: r.Title, Description: r.Description, Status: domain.Status(r.Status),
		Confidence: int(r.Confidence), KeyResults: results, Grade: r.Grade, Version: int(r.Version),
	}, nil
}

// Goals answers the goals a filter keeps, the oldest first.
func (GoalRepository) Goals(ctx context.Context, filter repository.GoalFilter) ([]domain.Goal, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuids(filter.PeriodID, filter.ParentID, filter.OwnerID)
	if err != nil {
		return nil, err
	}
	only, err := uuidsOf(filter.IDs)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListGoals(ctx, sqlc.ListGoalsParams{
		PeriodID: keys[0], ParentID: keys[1], OwnerID: keys[2], Ids: only, Max: int32(filter.Max),
	})
	if err != nil {
		return nil, goalError(err, "reading goals")
	}
	out := make([]domain.Goal, 0, len(rows))
	for _, r := range rows {
		g, err := goalOf(r)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

// Goal answers one goal.
func (GoalRepository) Goal(ctx context.Context, id shared.ID) (domain.Goal, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Goal{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Goal{}, err
	}
	r, err := queries.FindGoal(ctx, key)
	if err != nil {
		return domain.Goal{}, goalError(err, "reading a goal")
	}
	return goalOf(sqlc.ListGoalsRow(r))
}

// InsertGoal writes a new goal.
func (GoalRepository) InsertGoal(ctx context.Context, g domain.Goal, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(g.ID, g.PeriodID, g.HubID, g.ParentID, g.OwnerID, by)
	if err != nil {
		return err
	}
	results, err := encodeResults(g.KeyResults)
	if err != nil {
		return err
	}
	err = queries.InsertGoal(ctx, sqlc.InsertGoalParams{
		ID: keys[0], PeriodID: keys[1], HubID: keys[2], ParentID: keys[3], Title: g.Title,
		Description: g.Description, OwnerID: keys[4], Status: string(g.Status), KeyResults: results,
		At: timestampOf(at), By: keys[5],
	})
	return wrapGoal(err, "writing a goal")
}

// UpdateGoal writes a goal held to its version.
func (GoalRepository) UpdateGoal(ctx context.Context, g domain.Goal, expected int, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(g.ID, g.PeriodID, g.ParentID, g.OwnerID, by)
	if err != nil {
		return err
	}
	results, err := encodeResults(g.KeyResults)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version and a confidence are bounded by the row
	affected, err := queries.UpdateGoal(ctx, sqlc.UpdateGoalParams{
		PeriodID: keys[1], ParentID: keys[2], Title: g.Title, Description: g.Description, OwnerID: keys[3],
		Status: string(g.Status), Confidence: int32(g.Confidence), KeyResults: results, At: timestampOf(at),
		By: keys[4], ID: keys[0], ExpectedVersion: int32(expected),
	})
	if err != nil {
		return wrapGoal(err, "writing a goal")
	}
	return conflictOrGone(affected, "goals.version_conflict", expected)
}

// DeleteGoal removes a goal and hands its goals to its parent.
func (GoalRepository) DeleteGoal(ctx context.Context, g domain.Goal, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(g.ID, g.ParentID)
	if err != nil {
		return err
	}
	if err := queries.ReparentGoals(ctx, sqlc.ReparentGoalsParams{NewParentID: keys[1], ParentID: keys[0]}); err != nil {
		return goalError(err, "handing on a goal's goals")
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.DeleteGoal(ctx, sqlc.DeleteGoalParams{ID: keys[0], ExpectedVersion: int32(expected)})
	if err != nil {
		return goalError(err, "deleting a goal")
	}
	return conflictOrGone(affected, "goals.version_conflict", expected)
}

// LinkedEntries answers the entries linked to each goal.
func (GoalRepository) LinkedEntries(ctx context.Context, goals []shared.ID) (map[shared.ID][]shared.ID, error) {
	out := map[shared.ID][]shared.ID{}
	if len(goals) == 0 {
		return out, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(goals)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListGoalEntries(ctx, keys)
	if err != nil {
		return nil, goalError(err, "reading a goal's entries")
	}
	for _, r := range rows {
		pair, err := ids(r.GoalID, r.EntryID)
		if err != nil {
			return nil, err
		}
		out[pair[0]] = append(out[pair[0]], pair[1])
	}
	return out, nil
}

// GoalsOfEntry answers the goals an entry is linked to.
func (GoalRepository) GoalsOfEntry(ctx context.Context, entry shared.ID, limit int) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(entry)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListGoalsOfEntry(ctx, sqlc.ListGoalsOfEntryParams{EntryID: key, Max: int32(limit)})
	if err != nil {
		return nil, goalError(err, "reading an entry's goals")
	}
	return ids(rows...)
}

// LinkEntries links entries to a goal; one linked already stays as it was.
func (GoalRepository) LinkEntries(ctx context.Context, goal shared.ID, entries []shared.ID, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(goal, by)
	if err != nil {
		return err
	}
	for _, e := range entries {
		entry, err := uuidOf(e)
		if err != nil {
			return err
		}
		if err := queries.LinkGoalEntry(ctx, sqlc.LinkGoalEntryParams{
			GoalID: keys[0], EntryID: entry, At: timestampOf(at), By: keys[1],
		}); err != nil {
			if isForeignKeyViolation(err) {
				return shared.ErrNotFound.WithDetail("items.not_found")
			}
			return goalError(err, "linking an entry to a goal")
		}
	}
	return nil
}

// UnlinkEntry removes an entry's link to a goal and reports whether there was one.
func (GoalRepository) UnlinkEntry(ctx context.Context, goal, entry shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	keys, err := uuids(goal, entry)
	if err != nil {
		return false, err
	}
	affected, err := queries.UnlinkGoalEntry(ctx, sqlc.UnlinkGoalEntryParams{GoalID: keys[0], EntryID: keys[1]})
	if err != nil {
		return false, goalError(err, "unlinking an entry from a goal")
	}
	return affected > 0, nil
}

// AddCheckIn writes a check-in.
func (GoalRepository) AddCheckIn(ctx context.Context, c domain.CheckIn) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(c.ID, c.GoalID, c.AuthorID)
	if err != nil {
		return err
	}
	values, err := encodeValues(c.Values)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a confidence is 0 to 10
	err = queries.InsertGoalCheckIn(ctx, sqlc.InsertGoalCheckInParams{
		ID: keys[0], GoalID: keys[1], AuthorID: keys[2], Status: string(c.Status), Confidence: int32(c.Confidence),
		Note: c.Note, Values: values, At: timestampOf(c.At),
	})
	return wrapGoal(err, "writing a check-in")
}

// CheckIns answers a goal's check-ins, the latest first.
func (GoalRepository) CheckIns(ctx context.Context, goal shared.ID, limit int) ([]domain.CheckIn, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(goal)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListGoalCheckIns(ctx, sqlc.ListGoalCheckInsParams{GoalID: key, Max: int32(limit)})
	if err != nil {
		return nil, goalError(err, "reading check-ins")
	}
	out := make([]domain.CheckIn, 0, len(rows))
	for _, r := range rows {
		found, err := ids(r.ID, r.GoalID, r.AuthorID)
		if err != nil {
			return nil, err
		}
		values, err := decodeValues(r.Values)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.CheckIn{
			ID: found[0], GoalID: found[1], AuthorID: found[2], Status: domain.Status(r.Status),
			Confidence: int(r.Confidence), Note: r.Note, Values: values, At: r.CreatedAt.Time,
		})
	}
	return out, nil
}

func portfolioOf(r sqlc.ListPortfoliosRow) (domain.Portfolio, error) {
	found, err := ids(r.ID, r.ParentID, r.OwnerID)
	if err != nil {
		return domain.Portfolio{}, err
	}
	return domain.Portfolio{
		ID: found[0], ParentID: found[1], OwnerID: found[2], Name: r.Name, Description: r.Description,
		Status: domain.Status(r.Status), Version: int(r.Version),
	}, nil
}

func withItems(ctx context.Context, queries *sqlc.Queries, portfolios []domain.Portfolio) error {
	if len(portfolios) == 0 {
		return nil
	}
	keys := make([]shared.ID, 0, len(portfolios))
	at := map[shared.ID]int{}
	for i, p := range portfolios {
		keys = append(keys, p.ID)
		at[p.ID] = i
	}
	params, err := uuidsOf(keys)
	if err != nil {
		return err
	}
	rows, err := queries.ListPortfolioItems(ctx, params)
	if err != nil {
		return goalError(err, "reading a portfolio's projects")
	}
	for _, r := range rows {
		pair, err := ids(r.PortfolioID, r.ContainerID)
		if err != nil {
			return err
		}
		i := at[pair[0]]
		portfolios[i].Items = append(portfolios[i].Items, domain.PortfolioItem{
			ContainerID: pair[1], StartOn: dateOrNil(r.StartOn), EndOn: dateOrNil(r.EndOn), Status: domain.Status(r.Status),
		})
	}
	return nil
}

// Portfolios answers the portfolios with their projects, by name.
func (GoalRepository) Portfolios(ctx context.Context, limit int) ([]domain.Portfolio, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListPortfolios(ctx, int32(limit))
	if err != nil {
		return nil, goalError(err, "reading portfolios")
	}
	out := make([]domain.Portfolio, 0, len(rows))
	for _, r := range rows {
		p, err := portfolioOf(r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, withItems(ctx, queries, out)
}

// Portfolio answers one portfolio with its projects.
func (GoalRepository) Portfolio(ctx context.Context, id shared.ID) (domain.Portfolio, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Portfolio{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Portfolio{}, err
	}
	r, err := queries.FindPortfolio(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Portfolio{}, shared.ErrNotFound.WithDetail("portfolios.not_found")
	}
	if err != nil {
		return domain.Portfolio{}, goalError(err, "reading a portfolio")
	}
	p, err := portfolioOf(sqlc.ListPortfoliosRow(r))
	if err != nil {
		return domain.Portfolio{}, err
	}
	one := []domain.Portfolio{p}
	if err := withItems(ctx, queries, one); err != nil {
		return domain.Portfolio{}, err
	}
	return one[0], nil
}

func writeItems(ctx context.Context, queries *sqlc.Queries, p domain.Portfolio) error {
	key, err := uuidOf(p.ID)
	if err != nil {
		return err
	}
	if err := queries.ClearPortfolioItems(ctx, key); err != nil {
		return goalError(err, "clearing a portfolio's projects")
	}
	for i, item := range p.Items {
		container, err := uuidOf(item.ContainerID)
		if err != nil {
			return err
		}
		err = queries.InsertPortfolioItem(ctx, sqlc.InsertPortfolioItemParams{
			PortfolioID: key, ContainerID: container, Position: int32(i),
			StartOn: optionalDate(item.StartOn), EndOn: optionalDate(item.EndOn), Status: string(item.Status),
		})
		if isForeignKeyViolation(err) {
			return shared.ErrNotFound.WithDetail("containers.not_found")
		}
		if err != nil {
			return goalError(err, "writing a portfolio's project")
		}
	}
	return nil
}

// InsertPortfolio writes a new portfolio with its projects.
func (GoalRepository) InsertPortfolio(ctx context.Context, p domain.Portfolio, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(p.ID, p.ParentID, p.OwnerID, by)
	if err != nil {
		return err
	}
	if err := queries.InsertPortfolio(ctx, sqlc.InsertPortfolioParams{
		ID: keys[0], ParentID: keys[1], Name: p.Name, Description: p.Description, OwnerID: keys[2],
		Status: string(p.Status), At: timestampOf(at), By: keys[3],
	}); err != nil {
		return goalError(err, "writing a portfolio")
	}
	return writeItems(ctx, queries, p)
}

// UpdatePortfolio writes a portfolio and replaces its projects.
func (GoalRepository) UpdatePortfolio(ctx context.Context, p domain.Portfolio, expected int, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(p.ID, p.ParentID, p.OwnerID, by)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.UpdatePortfolio(ctx, sqlc.UpdatePortfolioParams{
		ParentID: keys[1], Name: p.Name, Description: p.Description, OwnerID: keys[2], Status: string(p.Status),
		At: timestampOf(at), By: keys[3], ID: keys[0], ExpectedVersion: int32(expected),
	})
	if err != nil {
		return goalError(err, "writing a portfolio")
	}
	if affected == 0 {
		return versionConflict("portfolios.version_conflict", expected)
	}
	return writeItems(ctx, queries, p)
}

// DeletePortfolio removes a portfolio and hands its portfolios to its parent.
func (GoalRepository) DeletePortfolio(ctx context.Context, p domain.Portfolio, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(p.ID, p.ParentID)
	if err != nil {
		return err
	}
	if err := queries.ReparentPortfolios(ctx, sqlc.ReparentPortfoliosParams{NewParentID: keys[1], ParentID: keys[0]}); err != nil {
		return goalError(err, "handing on a portfolio's portfolios")
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.DeletePortfolio(ctx, sqlc.DeletePortfolioParams{ID: keys[0], ExpectedVersion: int32(expected)})
	if err != nil {
		return goalError(err, "deleting a portfolio")
	}
	return conflictOrGone(affected, "portfolios.version_conflict", expected)
}

// AddStatusUpdate writes a portfolio's status update.
func (GoalRepository) AddStatusUpdate(ctx context.Context, u domain.StatusUpdate) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(u.ID, u.PortfolioID, u.AuthorID)
	if err != nil {
		return err
	}
	err = queries.InsertPortfolioUpdate(ctx, sqlc.InsertPortfolioUpdateParams{
		ID: keys[0], PortfolioID: keys[1], AuthorID: keys[2], Status: string(u.Status), Summary: u.Summary,
		At: timestampOf(u.At),
	})
	return wrapGoal(err, "writing a status update")
}

// StatusUpdates answers a portfolio's updates, the latest first.
func (GoalRepository) StatusUpdates(ctx context.Context, portfolio shared.ID, limit int) ([]domain.StatusUpdate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(portfolio)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListPortfolioUpdates(ctx, sqlc.ListPortfolioUpdatesParams{PortfolioID: key, Max: int32(limit)})
	if err != nil {
		return nil, goalError(err, "reading status updates")
	}
	out := make([]domain.StatusUpdate, 0, len(rows))
	for _, r := range rows {
		found, err := ids(r.ID, r.PortfolioID, r.AuthorID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.StatusUpdate{
			ID: found[0], PortfolioID: found[1], AuthorID: found[2], Status: domain.Status(r.Status),
			Summary: r.Summary, At: r.CreatedAt.Time,
		})
	}
	return out, nil
}

func dashboardOf(r sqlc.ListDashboardsRow) (domain.Dashboard, error) {
	found, err := ids(r.ID, r.HubID, r.OwnerID)
	if err != nil {
		return domain.Dashboard{}, err
	}
	widgets, err := decodeWidgets(r.Widgets)
	if err != nil {
		return domain.Dashboard{}, err
	}
	groups, err := idsFrom(r.GroupIds)
	if err != nil {
		return domain.Dashboard{}, err
	}
	return domain.Dashboard{
		ID: found[0], HubID: found[1], OwnerID: found[2], Name: r.Name, Sharing: domain.Sharing(r.Sharing),
		GroupIDs: groups, Widgets: widgets, Version: int(r.Version),
	}, nil
}

// Dashboards answers the workspace's dashboards by name; the use case keeps the readable ones.
func (GoalRepository) Dashboards(ctx context.Context, limit int) ([]domain.Dashboard, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListDashboards(ctx, int32(limit))
	if err != nil {
		return nil, goalError(err, "reading dashboards")
	}
	out := make([]domain.Dashboard, 0, len(rows))
	for _, r := range rows {
		d, err := dashboardOf(r)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// Dashboard answers one dashboard.
func (GoalRepository) Dashboard(ctx context.Context, id shared.ID) (domain.Dashboard, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Dashboard{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Dashboard{}, err
	}
	r, err := queries.FindDashboard(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Dashboard{}, shared.ErrNotFound.WithDetail("dashboards.not_found")
	}
	if err != nil {
		return domain.Dashboard{}, goalError(err, "reading a dashboard")
	}
	return dashboardOf(sqlc.ListDashboardsRow(r))
}

// InsertDashboard writes a new dashboard.
func (GoalRepository) InsertDashboard(ctx context.Context, d domain.Dashboard, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(d.ID, d.HubID, d.OwnerID)
	if err != nil {
		return err
	}
	widgets, err := encodeWidgets(d.Widgets)
	if err != nil {
		return err
	}
	groups, err := uuidsOf(d.GroupIDs)
	if err != nil {
		return err
	}
	err = queries.InsertDashboard(ctx, sqlc.InsertDashboardParams{
		ID: keys[0], HubID: keys[1], OwnerID: keys[2], Name: d.Name, Sharing: string(d.Sharing), GroupIds: groups,
		Widgets: widgets, At: timestampOf(at),
	})
	return wrapGoal(err, "writing a dashboard")
}

// UpdateDashboard writes a dashboard held to its version.
func (GoalRepository) UpdateDashboard(ctx context.Context, d domain.Dashboard, expected int, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(d.ID, by)
	if err != nil {
		return err
	}
	widgets, err := encodeWidgets(d.Widgets)
	if err != nil {
		return err
	}
	groups, err := uuidsOf(d.GroupIDs)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.UpdateDashboard(ctx, sqlc.UpdateDashboardParams{
		Name: d.Name, Sharing: string(d.Sharing), GroupIds: groups, Widgets: widgets, At: timestampOf(at), By: keys[1],
		ID: keys[0], ExpectedVersion: int32(expected),
	})
	if err != nil {
		return goalError(err, "writing a dashboard")
	}
	return conflictOrGone(affected, "dashboards.version_conflict", expected)
}

// DeleteDashboard removes a dashboard held to its version.
func (GoalRepository) DeleteDashboard(ctx context.Context, id shared.ID, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.DeleteDashboard(ctx, sqlc.DeleteDashboardParams{ID: key, ExpectedVersion: int32(expected)})
	if err != nil {
		return goalError(err, "deleting a dashboard")
	}
	return conflictOrGone(affected, "dashboards.version_conflict", expected)
}

// wrapGoal maps an insert's error: a missing period, hub or parent row is not found.
func wrapGoal(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case isForeignKeyViolation(err):
		return shared.ErrNotFound.WithDetail("goals.reference_not_found")
	case isUniqueViolation(err):
		return shared.ErrConflict.WithDetail("goals.already_exists")
	}
	return goalError(err, what)
}

func digestOf(r sqlc.FindDashboardDigestRow) (domain.Digest, error) {
	found, err := ids(r.ID, r.DashboardID, r.AccountID)
	if err != nil {
		return domain.Digest{}, err
	}
	d := domain.Digest{
		ID: found[0], DashboardID: found[1], AccountID: found[2], Cadence: domain.Cadence(r.Cadence),
		Weekday: int(r.Weekday), Hour: int(r.Hour), TimeZone: r.TimeZone, LastSentAt: optionalTime(r.LastSentAt),
	}
	if r.NextAt.Valid {
		d.NextAt = r.NextAt.Time.UTC()
	}
	return d, nil
}

// Digest answers one person's digest of a dashboard.
func (GoalRepository) Digest(ctx context.Context, dashboard, account shared.ID) (domain.Digest, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Digest{}, err
	}
	keys, err := uuids(dashboard, account)
	if err != nil {
		return domain.Digest{}, err
	}
	r, err := queries.FindDashboardDigest(ctx, sqlc.FindDashboardDigestParams{DashboardID: keys[0], AccountID: keys[1]})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Digest{}, shared.ErrNotFound.WithDetail("dashboards.digest_not_found")
	}
	if err != nil {
		return domain.Digest{}, goalError(err, "reading a digest")
	}
	return digestOf(r)
}

// DigestByID answers a digest by its identifier.
func (GoalRepository) DigestByID(ctx context.Context, id shared.ID) (domain.Digest, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Digest{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Digest{}, err
	}
	r, err := queries.FindDashboardDigestByID(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Digest{}, shared.ErrNotFound.WithDetail("dashboards.digest_not_found")
	}
	if err != nil {
		return domain.Digest{}, goalError(err, "reading a digest")
	}
	return digestOf(sqlc.FindDashboardDigestRow(r))
}

// PutDigest writes a person's digest of a dashboard, replacing theirs.
func (GoalRepository) PutDigest(ctx context.Context, d domain.Digest, at time.Time) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	keys, err := uuids(d.ID, d.DashboardID, d.AccountID)
	if err != nil {
		return "", err
	}
	//nolint:gosec // G115: weekday and hour are checked by the domain
	kept, err := queries.PutDashboardDigest(ctx, sqlc.PutDashboardDigestParams{
		ID: keys[0], DashboardID: keys[1], AccountID: keys[2], Cadence: string(d.Cadence),
		Weekday: int16(d.Weekday), Hour: int16(d.Hour), TimeZone: d.TimeZone,
		NextAt: timestampOf(d.NextAt), At: timestampOf(at),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return "", shared.ErrNotFound.WithDetail("dashboards.not_found")
		}
		return "", goalError(err, "writing a digest")
	}
	found, err := ids(kept)
	if err != nil {
		return "", err
	}
	return found[0], nil
}

// DeleteDigest removes a person's digest of a dashboard.
func (GoalRepository) DeleteDigest(ctx context.Context, dashboard, account shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	keys, err := uuids(dashboard, account)
	if err != nil {
		return false, err
	}
	affected, err := queries.DeleteDashboardDigest(ctx, sqlc.DeleteDashboardDigestParams{DashboardID: keys[0], AccountID: keys[1]})
	if err != nil {
		return false, goalError(err, "deleting a digest")
	}
	return affected > 0, nil
}

// DigestSent records a digest's run and its next moment.
func (GoalRepository) DigestSent(ctx context.Context, id shared.ID, sent *time.Time, next time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	_, err = queries.MarkDashboardDigestSent(ctx, sqlc.MarkDashboardDigestSentParams{
		SentAt: optionalTimestamp(sent), NextAt: timestampOf(next), ID: key,
	})
	if err != nil {
		return goalError(err, "recording a digest")
	}
	return nil
}
