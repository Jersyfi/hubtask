// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"sort"
	"strconv"
	"time"

	timerepo "github.com/Jersyfi/hubtask/core/application/repository/timekeeping"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListCostRatesName  = "ListCostRates"
	SetCostRateName    = "SetCostRate"
	RemoveCostRateName = "RemoveCostRate"
	ReadBudgetName     = "ReadBudget"
	SetBudgetName      = "SetBudget"
	RemoveBudgetName   = "RemoveBudget"

	rateTarget   = "time_cost_rate"
	budgetTarget = "time_budget"

	maxRatesKept = 1000
)

// The stable audit codes (audit.md §2).
const (
	RateSetAction       audit.Action = "time_cost_rate.set"
	RateRemovedAction   audit.Action = "time_cost_rate.removed"
	RatesReadAction     audit.Action = "time_cost_rate.read"
	BudgetSetAction     audit.Action = "time_budget.set"
	BudgetRemovedAction audit.Action = "time_budget.removed"
	BudgetReadAction    audit.Action = "time_budget.read"
)

// costsRequest is the question of managing costs at a place: MANAGE_COSTS, which the shipped roles
// reach through MANAGE_MEMBERS (research §6.3: rates are behind a finance permission).
func costsRequest(action audit.Action, scope, target string, path []identity.Scope, id shared.ID) access.Request {
	return access.Request{
		Permission: service.PermissionManageCosts, Alternative: service.PermissionManageMembers,
		Path: path, Action: action, TokenScope: scope, TargetType: target, TargetID: id,
	}
}

func workspacePath() []identity.Scope { return []identity.Scope{identity.TenantScope()} }

// ListCostRates answers the workspace's hourly cost rates (UC-TIM-08): what a person earns is
// read by nobody without MANAGE_COSTS - and so a rate's amount never reaches the audit trail.
type ListCostRates struct{ Keeper Keeper }

// Execute reads them.
func (h ListCostRates) Execute(ctx context.Context, actor appshared.ActorContext) ([]domain.CostRate, error) {
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, costsRequest(RatesReadAction, workspaceManage, rateTarget, workspacePath(), "")); err != nil {
		return nil, err
	}
	var out []domain.CostRate
	err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Keeper.Resources.Rates(ctx, maxRatesKept)
		return err
	})
	return out, err
}

// RateOutput is a rate on the wire.
func RateOutput(r domain.CostRate) usecase.Output {
	out := usecase.Output{
		"id": r.ID.String(), "account_id": idText(r.AccountID), "role": nil, "hour_minor": r.HourMinor,
		"currency": r.Currency, "from": r.From.String(), "to": nil, "version": r.Version,
	}
	if r.Role != "" {
		out["role"] = r.Role
	}
	if r.To != nil {
		out["to"] = r.To.String()
	}
	return out
}

// Descriptor is the catalogue entry.
func (h ListCostRates) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCostRatesName, Permission: service.PermissionManageCosts,
		Summary: "Lists the workspace's hourly cost rates - of a person or of a role, in a currency's minor unit, " +
			"from a day to a day or on. Needs MANAGE_COSTS or MANAGE_MEMBERS at the workspace.",
		SideEffects: "None. Reads only.",
		TokenScope:  workspaceManage, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: RatesReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			list, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(list))
			for _, r := range list {
				data = append(data, RateOutput(r))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, rateTarget)
}

// SetCostRate writes an hourly rate of a person or a role; its days never share one with another
// rate of the same subject, so that a day has one rate.
type SetCostRate struct{ Keeper Keeper }

// Execute writes a new rate (expected 0, no id) or changes one held to the version read.
func (h SetCostRate) Execute(ctx context.Context, actor appshared.ActorContext, r domain.CostRate, expected int) (domain.CostRate, error) {
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, costsRequest(RateSetAction, workspaceManage, rateTarget, workspacePath(), r.ID)); err != nil {
		return domain.CostRate{}, err
	}
	now := h.Keeper.Clock.Now()
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if expected > 0 {
			before, err := h.Keeper.Resources.FindRate(ctx, r.ID)
			if err != nil {
				return err
			}
			r.AccountID, r.Role = before.AccountID, before.Role
		} else {
			r.ID = h.Keeper.IDs.NewID()
		}
		if err := domain.CheckCostRate(r); err != nil {
			return err
		}
		existing, err := h.Keeper.Resources.Rates(ctx, maxRatesKept+1)
		if err != nil {
			return err
		}
		if expected == 0 && len(existing) >= maxRatesKept {
			return shared.ErrValidation.WithDetail("time.too_many_rates").
				WithParams(map[string]string{"maximum": strconv.Itoa(maxRatesKept)})
		}
		if err := domain.CheckRateOverlap(existing, r); err != nil {
			return err
		}
		if err := h.Keeper.Resources.PutRate(ctx, r, expected, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Keeper.ratesChanged(ctx, actor); err != nil {
			return err
		}
		r.Version = expected + 1
		subject := "role"
		if !r.AccountID.IsZero() {
			subject = "person"
		}
		return h.Keeper.record(ctx, actor, RateSetAction, rateTarget, r.ID, now,
			audit.Change{Field: "subject", Classification: audit.Open, To: subject},
			audit.Change{Field: "currency", Classification: audit.Open, To: r.Currency},
			audit.Change{Field: "from", Classification: audit.Open, To: r.From.String()},
			audit.Change{Field: "to", Classification: audit.Open, To: dayText(r.To)})
	})
	return r, err
}

// Descriptor is the catalogue entry.
func (h SetCostRate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetCostRateName, Permission: service.PermissionManageCosts,
		Summary: "Writes an hourly cost rate of a person or of a role - an amount in a currency's minor unit and an " +
			"ISO 4217 code - from a day to a day or on; a person's rate comes before their role's, and days never " +
			"share a rate of the same subject. Without a rate_id it adds one; with one it changes its amount and " +
			"days. Needs MANAGE_COSTS or MANAGE_MEMBERS at the workspace.",
		SideEffects: "Writes the rate and an audit entry without its amount.",
		TokenScope:  workspaceManage,
		Input: []usecase.Field{
			{Name: "rate_id", Kind: usecase.KindID, Description: "The rate to change; none adds one."},
			{Name: "account_id", Kind: usecase.KindID, Description: "The person it prices; or a role."},
			{Name: "role", Kind: usecase.KindString, Description: "The role key it prices; or a person."},
			{Name: "hour_minor", Kind: usecase.KindInt, Required: true, Description: "An hour's cost in the currency's minor unit."},
			{Name: "currency", Kind: usecase.KindString, Required: true, Description: "An ISO 4217 code, three capitals."},
			{Name: "from", Kind: usecase.KindString, Required: true, Description: "The first day it holds, YYYY-MM-DD."},
			{Name: "to", Kind: usecase.KindString, Description: "The last day it holds, YYYY-MM-DD; none holds on."},
			{Name: "expected_version", Kind: usecase.KindAny, CallerOnly: true, Description: "The version read, with a rate_id."},
		},
		Audit: usecase.AuditDeclaration{Action: RateSetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := optionalID(in, "rate_id")
			if err != nil {
				return nil, err
			}
			account, err := optionalID(in, "account_id")
			if err != nil {
				return nil, err
			}
			from, err := dayField(in, "from", true)
			if err != nil {
				return nil, err
			}
			r := domain.CostRate{
				ID: id, AccountID: account, Role: in.String("role"), HourMinor: int64(in.Int("hour_minor")),
				Currency: in.String("currency"), From: from,
			}
			if in.String("to") != "" {
				to, err := dayField(in, "to", true)
				if err != nil {
					return nil, err
				}
				r.To = &to
			}
			expected := 0
			if !id.IsZero() {
				var ok bool
				if expected, ok = intField(in, "expected_version"); !ok || expected < 1 {
					return nil, invalid("/expected_version", "time.version_required")
				}
			}
			out, err := h.Execute(ctx, actor, r, expected)
			if err != nil {
				return nil, err
			}
			return RateOutput(out), nil
		}),
	}, rateTarget)
}

// RemoveCostRate removes a rate.
type RemoveCostRate struct{ Keeper Keeper }

// Execute removes it held to the version read.
func (h RemoveCostRate) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, expected int) error {
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, costsRequest(RateRemovedAction, workspaceManage, rateTarget, workspacePath(), id)); err != nil {
		return err
	}
	now := h.Keeper.Clock.Now()
	return h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Keeper.Resources.RemoveRate(ctx, id, expected); err != nil {
			return err
		}
		if err := h.Keeper.ratesChanged(ctx, actor); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, RateRemovedAction, rateTarget, id, now)
	})
}

// Descriptor is the catalogue entry.
func (h RemoveCostRate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemoveCostRateName, Permission: service.PermissionManageCosts,
		Summary:     "Removes an hourly cost rate, held to the version read. Needs MANAGE_COSTS or MANAGE_MEMBERS at the workspace.",
		SideEffects: "Removes the rate and writes an audit entry.",
		TokenScope:  workspaceManage,
		Input: []usecase.Field{
			{Name: "rate_id", Kind: usecase.KindID, Required: true, Description: "The rate."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		},
		Audit: usecase.AuditDeclaration{Action: RateRemovedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("rate_id")
			if err != nil {
				return nil, err
			}
			expected, ok := intField(in, "expected_version")
			if !ok {
				return nil, invalid("/expected_version", "time.version_required")
			}
			if err := h.Execute(ctx, actor, id, expected); err != nil {
				return nil, err
			}
			return usecase.Output{"removed": true}, nil
		}),
	}, rateTarget)
}

// ReadBudget answers a hub's or a collection's budget and its burn (UC-TIM-08): every minute
// recorded on its entries against the time budget, and what they cost by the day's rate in the
// budget's currency against the money budget, week by week.
type ReadBudget struct{ Keeper Keeper }

// maxDaysOfABurn reaches back far enough for any project: the burn reads every record of the plan.
const maxDaysOfABurn = 3660

// Execute reads it; held is false where the plan has no budget, whose burn is still answered.
func (h ReadBudget) Execute(ctx context.Context, actor appshared.ActorContext, planID shared.ID) (domain.Burn, bool, error) {
	at, err := h.Keeper.planPlace(ctx, actor, planID)
	if err != nil {
		return domain.Burn{}, false, err
	}
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, costsRequest(BudgetReadAction, itemsRead, budgetTarget, at.path(), planID)); err != nil {
		return domain.Burn{}, false, err
	}
	today := planning.DayOf(h.Keeper.Clock.Now(), time.UTC)
	filter := timerepo.Filter{From: today - maxDaysOfABurn, To: today + maxDaysOfARange}
	if at.collection.IsZero() {
		filter.HubID = at.hub
	} else {
		filter.CollectionID = at.collection
	}
	var (
		budget domain.Budget
		held   bool
		lines  []domain.Line
		rates  []domain.CostRate
	)
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if budget, held, err = h.Keeper.Resources.Budget(ctx, planID); err != nil {
			return err
		}
		if lines, err = h.Keeper.Store.Lines(ctx, filter, maxLinesSummed+1); err != nil {
			return err
		}
		if len(lines) > maxLinesSummed {
			return shared.ErrValidation.WithDetail("time.too_many_lines").
				WithParams(map[string]string{"maximum": strconv.Itoa(maxLinesSummed)})
		}
		rates, err = h.Keeper.Resources.Rates(ctx, maxRatesKept)
		return err
	}); err != nil {
		return domain.Burn{}, false, err
	}
	budget.PlanID = planID
	roles, err := h.Keeper.rolesAt(ctx, actor, lines, at)
	if err != nil {
		return domain.Burn{}, false, err
	}
	priced := make([]domain.Priced, 0, len(lines))
	for _, l := range lines {
		priced = append(priced, domain.Priced{AccountID: l.AccountID, Day: l.Day, Minutes: l.Minutes, Roles: roles[l.AccountID]})
	}
	sort.SliceStable(priced, func(i, j int) bool { return priced[i].Day < priced[j].Day })
	return domain.BurnOf(budget, priced, domain.NewRates(rates)), held, nil
}

// rolesAt answers each person's role keys along the plan's path, the most specific first.
func (k Keeper) rolesAt(ctx context.Context, actor appshared.ActorContext, lines []domain.Line, at place) (map[shared.ID][]string, error) {
	out := map[shared.ID][]string{}
	if k.Roles == nil {
		return out, nil
	}
	path := at.path()
	for _, l := range lines {
		if _, done := out[l.AccountID]; done {
			continue
		}
		keys, err := k.Roles.RolesBy(ctx, actor, l.AccountID, path)
		if err != nil {
			return nil, err
		}
		out[l.AccountID] = keys
	}
	return out, nil
}

// BurnOutput is a budget and its burn on the wire.
func BurnOutput(b domain.Burn, held bool) usecase.Output {
	weeks := make([]usecase.Output, 0, len(b.Weeks))
	for _, w := range b.Weeks {
		weeks = append(weeks, usecase.Output{"week": w.Week.String(), "minutes": w.Minutes, "amount_minor": w.AmountMinor})
	}
	return usecase.Output{
		"plan_id": b.Budget.PlanID.String(), "budgeted": held, "budget_minutes": b.Budget.Minutes,
		"budget_amount_minor": b.Budget.AmountMinor, "currency": b.Budget.Currency, "version": b.Budget.Version,
		"minutes": b.Minutes, "amount_minor": b.AmountMinor, "unpriced_minutes": b.UnpricedMinutes,
		"over_time": b.OverTime(), "over_money": b.OverMoney(), "weeks": weeks,
	}
}

// Descriptor is the catalogue entry.
func (h ReadBudget) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadBudgetName, Permission: service.PermissionManageCosts,
		Summary: "Answers a hub's or a collection's budget - minutes, money in one currency, or both - and its burn: " +
			"every minute recorded on its entries, what they cost by each day's rate in the budget's currency (a " +
			"rate in another is not converted; its minutes are unpriced), whether either budget is passed, and the " +
			"cumulative burn week by week. Needs MANAGE_COSTS or MANAGE_MEMBERS at the plan.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or the collection."},
		},
		Audit: usecase.AuditDeclaration{Action: BudgetReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			burn, held, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return BurnOutput(burn, held), nil
		}),
	}, budgetTarget)
}

// SetBudget writes a hub's or a collection's budget.
type SetBudget struct{ Keeper Keeper }

// Execute writes it held to the version read; 0 before the first.
func (h SetBudget) Execute(ctx context.Context, actor appshared.ActorContext, b domain.Budget, expected int) (domain.Budget, error) {
	if err := domain.CheckBudget(b); err != nil {
		return domain.Budget{}, err
	}
	at, err := h.Keeper.planPlace(ctx, actor, b.PlanID)
	if err != nil {
		return domain.Budget{}, err
	}
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, costsRequest(BudgetSetAction, itemsWrite, budgetTarget, at.path(), b.PlanID)); err != nil {
		return domain.Budget{}, err
	}
	now := h.Keeper.Clock.Now()
	err = h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, _, err := h.Keeper.Resources.Budget(ctx, b.PlanID)
		if err != nil {
			return err
		}
		if before.Version != expected {
			return shared.ErrVersionConflict.WithDetail("time.budget_version_conflict").
				WithParams(map[string]string{"expected_version": strconv.Itoa(expected)})
		}
		if err := h.Keeper.Resources.PutBudget(ctx, h.Keeper.IDs.NewID(), b, expected, actor.AccountID, now); err != nil {
			return err
		}
		b.Version = expected + 1
		if err := h.Keeper.budgetChanged(ctx, actor, b.PlanID, &b); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, BudgetSetAction, budgetTarget, b.PlanID, now,
			audit.Change{Field: "minutes", Classification: audit.Open, From: strconv.FormatInt(before.Minutes, 10), To: strconv.FormatInt(b.Minutes, 10)},
			audit.Change{Field: "amount_minor", Classification: audit.Open, From: strconv.FormatInt(before.AmountMinor, 10), To: strconv.FormatInt(b.AmountMinor, 10)},
			audit.Change{Field: "currency", Classification: audit.Open, From: before.Currency, To: b.Currency})
	})
	return b, err
}

// Descriptor is the catalogue entry.
func (h SetBudget) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetBudgetName, Permission: service.PermissionManageCosts,
		Summary: "Writes a hub's or a collection's budget: minutes, money in a currency's minor unit with its ISO " +
			"4217 code, or both; 0 for none. Held to the version read, 0 before the first. Needs MANAGE_COSTS or " +
			"MANAGE_MEMBERS at the plan.",
		SideEffects: "Writes the budget and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or the collection."},
			{Name: "minutes", Kind: usecase.KindInt, Description: "The time budget in minutes; 0 for none."},
			{Name: "amount_minor", Kind: usecase.KindInt, Description: "The money budget in the currency's minor unit; 0 for none."},
			{Name: "currency", Kind: usecase.KindString, Description: "The money budget's ISO 4217 code."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read; 0 before the first."},
		},
		Audit: usecase.AuditDeclaration{Action: BudgetSetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			expected, ok := intField(in, "expected_version")
			if !ok {
				return nil, invalid("/expected_version", "time.version_required")
			}
			b, err := h.Execute(ctx, actor, domain.Budget{
				PlanID: id, Minutes: int64(in.Int("minutes")), AmountMinor: int64(in.Int("amount_minor")), Currency: in.String("currency"),
			}, expected)
			if err != nil {
				return nil, err
			}
			return BurnOutput(domain.Burn{Budget: b}, true), nil
		}),
	}, budgetTarget)
}

// RemoveBudget removes a hub's or a collection's budget; its burn stays readable.
type RemoveBudget struct{ Keeper Keeper }

// Execute removes it held to the version read.
func (h RemoveBudget) Execute(ctx context.Context, actor appshared.ActorContext, planID shared.ID, expected int) error {
	at, err := h.Keeper.planPlace(ctx, actor, planID)
	if err != nil {
		return err
	}
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, costsRequest(BudgetRemovedAction, itemsWrite, budgetTarget, at.path(), planID)); err != nil {
		return err
	}
	now := h.Keeper.Clock.Now()
	return h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Keeper.Resources.RemoveBudget(ctx, planID, expected); err != nil {
			return err
		}
		if err := h.Keeper.budgetChanged(ctx, actor, planID, nil); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, BudgetRemovedAction, budgetTarget, planID, now)
	})
}

// Descriptor is the catalogue entry.
func (h RemoveBudget) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemoveBudgetName, Permission: service.PermissionManageCosts,
		Summary:     "Removes a hub's or a collection's budget, held to the version read. Needs MANAGE_COSTS or MANAGE_MEMBERS at the plan.",
		SideEffects: "Removes the budget and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or the collection."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		},
		Audit: usecase.AuditDeclaration{Action: BudgetRemovedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			expected, ok := intField(in, "expected_version")
			if !ok {
				return nil, invalid("/expected_version", "time.version_required")
			}
			if err := h.Execute(ctx, actor, id, expected); err != nil {
				return nil, err
			}
			return usecase.Output{"removed": true}, nil
		}),
	}, budgetTarget)
}
