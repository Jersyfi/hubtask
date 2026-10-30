// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// TimeResourceRepository stores capacity, absences, cost rates and budgets (migration 0247, M7).
type TimeResourceRepository struct{}

var _ repository.Resources = TimeResourceRepository{}

// NewTimeResourceRepository answers the adapter.
func NewTimeResourceRepository() TimeResourceRepository { return TimeResourceRepository{} }

func versionConflict(code string, expected int) error {
	return shared.ErrVersionConflict.WithDetail(code).
		WithParams(map[string]string{"expected_version": strconv.Itoa(expected)})
}

// Capacities answers the capacities of the people named, or of everybody.
func (TimeResourceRepository) Capacities(ctx context.Context, accounts []shared.ID, max int) ([]domain.Capacity, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]pgtype.UUID, 0, len(accounts))
	for _, a := range accounts {
		key, err := uuidOf(a)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListTimeCapacities(ctx, sqlc.ListTimeCapacitiesParams{AccountIds: keys, Max: int32(max)})
	if err != nil {
		return nil, timeError(err, "reading capacities")
	}
	out := make([]domain.Capacity, 0, len(rows))
	for _, r := range rows {
		account, err := idFrom(r.AccountID)
		if err != nil {
			return nil, err
		}
		c := domain.Capacity{AccountID: account, From: dateDay(r.FromDay), Version: int(r.Version)}
		for i := 0; i < 7 && i < len(r.WeekMinutes); i++ {
			c.WeekMinutes[i] = int(r.WeekMinutes[i])
		}
		out = append(out, c)
	}
	return out, nil
}

// PutCapacity writes a person's capacity from its day on.
func (TimeResourceRepository) PutCapacity(ctx context.Context, id shared.ID, c domain.Capacity, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	account, err := uuidOf(c.AccountID)
	if err != nil {
		return err
	}
	who, err := optionalUUID(by)
	if err != nil {
		return err
	}
	minutes := make([]int32, 7)
	for i, m := range c.WeekMinutes {
		//nolint:gosec // G115: a weekday's minutes are bounded by CheckCapacity
		minutes[i] = int32(m)
	}
	if err := queries.UpsertTimeCapacity(ctx, sqlc.UpsertTimeCapacityParams{
		ID: key, AccountID: account, FromDay: dayDate(c.From), WeekMinutes: minutes, At: timestampOf(at), UpdatedBy: who,
	}); err != nil {
		return timeError(err, "writing a capacity")
	}
	return nil
}

// RemoveCapacity removes the capacity a person has from a day.
func (TimeResourceRepository) RemoveCapacity(ctx context.Context, account shared.ID, from domain.Day) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(account)
	if err != nil {
		return false, err
	}
	affected, err := queries.DeleteTimeCapacity(ctx, sqlc.DeleteTimeCapacityParams{AccountID: key, FromDay: dayDate(from)})
	if err != nil {
		return false, timeError(err, "removing a capacity")
	}
	return affected > 0, nil
}

func absenceFrom(idValue, accountValue pgtype.UUID, from, to pgtype.Date, kind string, half bool, version int32) (domain.Absence, error) {
	id, err := idFrom(idValue)
	if err != nil {
		return domain.Absence{}, err
	}
	account, err := idFrom(accountValue)
	if err != nil {
		return domain.Absence{}, err
	}
	return domain.Absence{
		ID: id, AccountID: account, From: dateDay(from), To: dateDay(to), Kind: domain.AbsenceKind(kind),
		Half: half, Version: int(version),
	}, nil
}

// Absences answers the absences sharing a day with a range.
func (TimeResourceRepository) Absences(ctx context.Context, account shared.ID, from, to domain.Day, max int) ([]domain.Absence, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	who, err := optionalUUID(account)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListTimeAbsences(ctx, sqlc.ListTimeAbsencesParams{
		FromDay: dayDate(from), ToDay: dayDate(to), AccountID: who, Max: int32(max),
	})
	if err != nil {
		return nil, timeError(err, "reading absences")
	}
	out := make([]domain.Absence, 0, len(rows))
	for _, r := range rows {
		a, err := absenceFrom(r.ID, r.AccountID, r.FromDay, r.ToDay, r.Kind, r.Half, r.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// FindAbsence answers one absence.
func (TimeResourceRepository) FindAbsence(ctx context.Context, id shared.ID) (domain.Absence, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Absence{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Absence{}, err
	}
	r, err := queries.FindTimeAbsence(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Absence{}, shared.ErrNotFound.WithDetail("time.absence_not_found")
	}
	if err != nil {
		return domain.Absence{}, timeError(err, "reading an absence")
	}
	return absenceFrom(r.ID, r.AccountID, r.FromDay, r.ToDay, r.Kind, r.Half, r.Version)
}

// PutAbsence writes an absence.
func (TimeResourceRepository) PutAbsence(ctx context.Context, a domain.Absence, expected int, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(a.ID)
	if err != nil {
		return err
	}
	if expected == 0 {
		account, err := uuidOf(a.AccountID)
		if err != nil {
			return err
		}
		who, err := optionalUUID(by)
		if err != nil {
			return err
		}
		if err := queries.InsertTimeAbsence(ctx, sqlc.InsertTimeAbsenceParams{
			ID: key, AccountID: account, FromDay: dayDate(a.From), ToDay: dayDate(a.To), Kind: string(a.Kind),
			Half: a.Half, At: timestampOf(at), CreatedBy: who,
		}); err != nil {
			return timeError(err, "writing an absence")
		}
		return nil
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.UpdateTimeAbsence(ctx, sqlc.UpdateTimeAbsenceParams{
		ID: key, FromDay: dayDate(a.From), ToDay: dayDate(a.To), Kind: string(a.Kind), Half: a.Half,
		At: timestampOf(at), ExpectedVersion: int32(expected),
	})
	if err != nil {
		return timeError(err, "writing an absence")
	}
	if affected == 0 {
		return versionConflict("time.absence_version_conflict", expected)
	}
	return nil
}

// RemoveAbsence removes an absence held to the version read.
func (TimeResourceRepository) RemoveAbsence(ctx context.Context, id shared.ID, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.DeleteTimeAbsence(ctx, sqlc.DeleteTimeAbsenceParams{ID: key, ExpectedVersion: int32(expected)})
	if err != nil {
		return timeError(err, "removing an absence")
	}
	if affected == 0 {
		return versionConflict("time.absence_version_conflict", expected)
	}
	return nil
}

func rateFrom(idValue, accountValue pgtype.UUID, role *string, hour int64, currency string, from, to pgtype.Date, version int32) (domain.CostRate, error) {
	id, err := idFrom(idValue)
	if err != nil {
		return domain.CostRate{}, err
	}
	account, err := optionalID(accountValue)
	if err != nil {
		return domain.CostRate{}, err
	}
	r := domain.CostRate{
		ID: id, AccountID: account, HourMinor: hour, Currency: currency, From: dateDay(from), Version: int(version),
	}
	if role != nil {
		r.Role = *role
	}
	if to.Valid {
		d := dateDay(to)
		r.To = &d
	}
	return r, nil
}

// Rates answers the workspace's cost rates.
func (TimeResourceRepository) Rates(ctx context.Context, max int) ([]domain.CostRate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is a bound the use case sets
	rows, err := queries.ListTimeCostRates(ctx, int32(max))
	if err != nil {
		return nil, timeError(err, "reading cost rates")
	}
	out := make([]domain.CostRate, 0, len(rows))
	for _, r := range rows {
		rate, err := rateFrom(r.ID, r.AccountID, r.Role, r.HourMinor, r.Currency, r.FromDay, r.ToDay, r.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, rate)
	}
	return out, nil
}

// FindRate answers one rate.
func (TimeResourceRepository) FindRate(ctx context.Context, id shared.ID) (domain.CostRate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.CostRate{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.CostRate{}, err
	}
	r, err := queries.FindTimeCostRate(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CostRate{}, shared.ErrNotFound.WithDetail("time.rate_not_found")
	}
	if err != nil {
		return domain.CostRate{}, timeError(err, "reading a cost rate")
	}
	return rateFrom(r.ID, r.AccountID, r.Role, r.HourMinor, r.Currency, r.FromDay, r.ToDay, r.Version)
}

// PutRate writes a rate.
func (TimeResourceRepository) PutRate(ctx context.Context, r domain.CostRate, expected int, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(r.ID)
	if err != nil {
		return err
	}
	who, err := optionalUUID(by)
	if err != nil {
		return err
	}
	if expected == 0 {
		account, err := optionalUUID(r.AccountID)
		if err != nil {
			return err
		}
		var role *string
		if r.Role != "" {
			role = &r.Role
		}
		if err := queries.InsertTimeCostRate(ctx, sqlc.InsertTimeCostRateParams{
			ID: key, AccountID: account, Role: role, HourMinor: r.HourMinor, Currency: r.Currency,
			FromDay: dayDate(r.From), ToDay: optionalDayDate(r.To), At: timestampOf(at), UpdatedBy: who,
		}); err != nil {
			return timeError(err, "writing a cost rate")
		}
		return nil
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.UpdateTimeCostRate(ctx, sqlc.UpdateTimeCostRateParams{
		ID: key, HourMinor: r.HourMinor, Currency: r.Currency, FromDay: dayDate(r.From), ToDay: optionalDayDate(r.To),
		At: timestampOf(at), UpdatedBy: who, ExpectedVersion: int32(expected),
	})
	if err != nil {
		return timeError(err, "writing a cost rate")
	}
	if affected == 0 {
		return versionConflict("time.rate_version_conflict", expected)
	}
	return nil
}

// RemoveRate removes a rate held to the version read.
func (TimeResourceRepository) RemoveRate(ctx context.Context, id shared.ID, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.DeleteTimeCostRate(ctx, sqlc.DeleteTimeCostRateParams{ID: key, ExpectedVersion: int32(expected)})
	if err != nil {
		return timeError(err, "removing a cost rate")
	}
	if affected == 0 {
		return versionConflict("time.rate_version_conflict", expected)
	}
	return nil
}

// Budget answers a plan's budget.
func (TimeResourceRepository) Budget(ctx context.Context, plan shared.ID) (domain.Budget, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Budget{}, false, err
	}
	key, err := uuidOf(plan)
	if err != nil {
		return domain.Budget{}, false, err
	}
	r, err := queries.FindTimeBudget(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Budget{PlanID: plan}, false, nil
	}
	if err != nil {
		return domain.Budget{}, false, timeError(err, "reading a budget")
	}
	return domain.Budget{
		PlanID: plan, Minutes: r.Minutes, AmountMinor: r.AmountMinor, Currency: r.Currency, Version: int(r.Version),
	}, true, nil
}

// PutBudget writes a plan's budget.
func (TimeResourceRepository) PutBudget(ctx context.Context, id shared.ID, b domain.Budget, expected int, by shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	plan, err := uuidOf(b.PlanID)
	if err != nil {
		return err
	}
	who, err := optionalUUID(by)
	if err != nil {
		return err
	}
	if expected == 0 {
		key, err := uuidOf(id)
		if err != nil {
			return err
		}
		err = queries.InsertTimeBudget(ctx, sqlc.InsertTimeBudgetParams{
			ID: key, PlanID: plan, Minutes: b.Minutes, AmountMinor: b.AmountMinor, Currency: b.Currency,
			At: timestampOf(at), UpdatedBy: who,
		})
		if isUniqueViolation(err) {
			return versionConflict("time.budget_version_conflict", expected)
		}
		if isForeignKeyViolation(err) {
			return shared.ErrNotFound.WithDetail("containers.not_found")
		}
		if err != nil {
			return timeError(err, "writing a budget")
		}
		return nil
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.UpdateTimeBudget(ctx, sqlc.UpdateTimeBudgetParams{
		PlanID: plan, Minutes: b.Minutes, AmountMinor: b.AmountMinor, Currency: b.Currency, At: timestampOf(at),
		UpdatedBy: who, ExpectedVersion: int32(expected),
	})
	if err != nil {
		return timeError(err, "writing a budget")
	}
	if affected == 0 {
		return versionConflict("time.budget_version_conflict", expected)
	}
	return nil
}

// RemoveBudget removes a plan's budget held to the version read.
func (TimeResourceRepository) RemoveBudget(ctx context.Context, plan shared.ID, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(plan)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.DeleteTimeBudget(ctx, sqlc.DeleteTimeBudgetParams{PlanID: key, ExpectedVersion: int32(expected)})
	if err != nil {
		return timeError(err, "removing a budget")
	}
	if affected == 0 {
		return versionConflict("time.budget_version_conflict", expected)
	}
	return nil
}
