// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"strconv"

	timerepo "github.com/Jersyfi/hubtask/core/application/repository/timekeeping"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListBillingRatesName  = "ListBillingRates"
	SetBillingRateName    = "SetBillingRate"
	RemoveBillingRateName = "RemoveBillingRate"

	billingTarget = "time_billing_rate"

	// maxBillingRatesKept bounds one plan's billing rates: a rate a month for eight years.
	maxBillingRatesKept = 100
)

// The stable audit codes (audit.md §2).
const (
	BillingRateSetAction     audit.Action = "time_billing_rate.set"
	BillingRateRemovedAction audit.Action = "time_billing_rate.removed"
	BillingRatesReadAction   audit.Action = "time_billing_rate.read"
)

// ListBillingRates answers a hub's or a collection's billing rates (UC-TIM-08): what its client is
// charged is read by nobody without MANAGE_COSTS there, and its amount never reaches the trail.
type ListBillingRates struct{ Keeper Keeper }

// Execute reads them.
func (h ListBillingRates) Execute(ctx context.Context, actor appshared.ActorContext, planID shared.ID) ([]domain.BillingRate, error) {
	at, err := h.Keeper.planPlace(ctx, actor, planID)
	if err != nil {
		return nil, err
	}
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, costsRequest(BillingRatesReadAction, itemsRead, billingTarget, at.path(), planID)); err != nil {
		return nil, err
	}
	var out []domain.BillingRate
	err = h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Keeper.Resources.BillingRates(ctx, []shared.ID{planID}, maxBillingRatesKept)
		return err
	})
	return out, err
}

// BillingRateOutput is a billing rate on the wire.
func BillingRateOutput(r domain.BillingRate) usecase.Output {
	out := usecase.Output{
		"id": r.ID.String(), "plan_id": r.PlanID.String(), "hour_minor": r.HourMinor, "currency": r.Currency,
		"from": r.From.String(), "to": nil, "version": r.Version,
	}
	if r.To != nil {
		out["to"] = r.To.String()
	}
	return out
}

// Descriptor is the catalogue entry.
func (h ListBillingRates) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListBillingRatesName, Permission: service.PermissionManageCosts,
		Summary: "Lists a hub's or a collection's billing rates - what an hour of billable time there is charged to " +
			"its client, in a currency's minor unit, from a day to a day or on; a collection's own rate comes before " +
			"its hub's. Needs MANAGE_COSTS or MANAGE_MEMBERS at the plan.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or the collection."},
		},
		Audit: usecase.AuditDeclaration{Action: BillingRatesReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			list, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(list))
			for _, r := range list {
				data = append(data, BillingRateOutput(r))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, billingTarget)
}

// SetBillingRate writes a hub's or a collection's billing rate; its days never share one with
// another rate of the same plan, so that a day of a plan is billed at one rate.
type SetBillingRate struct{ Keeper Keeper }

// Execute writes a new rate (expected 0, no id) or changes one of the plan held to the version
// read; a rate of another plan is not found, so the plan in the path is the one authorised.
func (h SetBillingRate) Execute(ctx context.Context, actor appshared.ActorContext, r domain.BillingRate, expected int) (domain.BillingRate, error) {
	at, err := h.Keeper.planPlace(ctx, actor, r.PlanID)
	if err != nil {
		return domain.BillingRate{}, err
	}
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, costsRequest(BillingRateSetAction, itemsWrite, billingTarget, at.path(), r.PlanID)); err != nil {
		return domain.BillingRate{}, err
	}
	if err := domain.CheckBillingRate(r); err != nil {
		return domain.BillingRate{}, err
	}
	now := h.Keeper.Clock.Now()
	err = h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if expected > 0 {
			before, err := h.Keeper.Resources.FindBillingRate(ctx, r.ID)
			if err != nil {
				return err
			}
			if before.PlanID != r.PlanID {
				return shared.ErrNotFound.WithDetail("time.billing_rate_not_found")
			}
		} else {
			r.ID = h.Keeper.IDs.NewID()
		}
		existing, err := h.Keeper.Resources.BillingRates(ctx, []shared.ID{r.PlanID}, maxBillingRatesKept+1)
		if err != nil {
			return err
		}
		if expected == 0 && len(existing) >= maxBillingRatesKept {
			return shared.ErrValidation.WithDetail("time.too_many_rates").
				WithParams(map[string]string{"maximum": strconv.Itoa(maxBillingRatesKept)})
		}
		if err := domain.CheckBillingOverlap(existing, r); err != nil {
			return err
		}
		if err := h.Keeper.Resources.PutBillingRate(ctx, r, expected, actor.AccountID, now); err != nil {
			return err
		}
		r.Version = expected + 1
		return h.Keeper.record(ctx, actor, BillingRateSetAction, billingTarget, r.ID, now,
			audit.Change{Field: "plan_id", Classification: audit.Open, To: r.PlanID.String()},
			audit.Change{Field: "currency", Classification: audit.Open, To: r.Currency},
			audit.Change{Field: "from", Classification: audit.Open, To: r.From.String()},
			audit.Change{Field: "to", Classification: audit.Open, To: dayText(r.To)})
	})
	return r, err
}

// Descriptor is the catalogue entry.
func (h SetBillingRate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetBillingRateName, Permission: service.PermissionManageCosts,
		Summary: "Writes a hub's or a collection's billing rate - an hour's charge in a currency's minor unit and an " +
			"ISO 4217 code - from a day to a day or on; the days of two rates of one plan never meet. Without a " +
			"rate_id it adds one; with one it changes its amount and days. Needs MANAGE_COSTS or MANAGE_MEMBERS at " +
			"the plan.",
		SideEffects: "Writes the rate and an audit entry without its amount.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or the collection."},
			{Name: "rate_id", Kind: usecase.KindID, Description: "The rate to change; none adds one."},
			{Name: "hour_minor", Kind: usecase.KindInt, Required: true, Description: "An hour's charge in the currency's minor unit."},
			{Name: "currency", Kind: usecase.KindString, Required: true, Description: "An ISO 4217 code, three capitals."},
			{Name: "from", Kind: usecase.KindString, Required: true, Description: "The first day it holds, YYYY-MM-DD."},
			{Name: "to", Kind: usecase.KindString, Description: "The last day it holds, YYYY-MM-DD; none holds on."},
			{Name: "expected_version", Kind: usecase.KindAny, CallerOnly: true, Description: "The version read, with a rate_id."},
		},
		Audit: usecase.AuditDeclaration{Action: BillingRateSetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			plan, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			id, err := optionalID(in, "rate_id")
			if err != nil {
				return nil, err
			}
			from, err := dayField(in, "from", true)
			if err != nil {
				return nil, err
			}
			r := domain.BillingRate{ID: id, PlanID: plan, HourMinor: int64(in.Int("hour_minor")), Currency: in.String("currency"), From: from}
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
			return BillingRateOutput(out), nil
		}),
	}, billingTarget)
}

// RemoveBillingRate removes a billing rate of a plan.
type RemoveBillingRate struct{ Keeper Keeper }

// Execute removes it held to the version read; a rate of another plan is not found.
func (h RemoveBillingRate) Execute(ctx context.Context, actor appshared.ActorContext, planID, id shared.ID, expected int) error {
	at, err := h.Keeper.planPlace(ctx, actor, planID)
	if err != nil {
		return err
	}
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, costsRequest(BillingRateRemovedAction, itemsWrite, billingTarget, at.path(), planID)); err != nil {
		return err
	}
	now := h.Keeper.Clock.Now()
	return h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := h.Keeper.Resources.FindBillingRate(ctx, id)
		if err != nil {
			return err
		}
		if before.PlanID != planID {
			return shared.ErrNotFound.WithDetail("time.billing_rate_not_found")
		}
		if err := h.Keeper.Resources.RemoveBillingRate(ctx, id, expected); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, BillingRateRemovedAction, billingTarget, id, now,
			audit.Change{Field: "plan_id", Classification: audit.Open, From: planID.String()})
	})
}

// Descriptor is the catalogue entry.
func (h RemoveBillingRate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemoveBillingRateName, Permission: service.PermissionManageCosts,
		Summary:     "Removes a hub's or a collection's billing rate, held to the version read. Needs MANAGE_COSTS or MANAGE_MEMBERS at the plan.",
		SideEffects: "Removes the rate and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or the collection."},
			{Name: "rate_id", Kind: usecase.KindID, Required: true, Description: "The rate."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		},
		Audit: usecase.AuditDeclaration{Action: BillingRateRemovedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			plan, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			id, err := in.ID("rate_id")
			if err != nil {
				return nil, err
			}
			expected, ok := intField(in, "expected_version")
			if !ok {
				return nil, invalid("/expected_version", "time.version_required")
			}
			if err := h.Execute(ctx, actor, plan, id, expected); err != nil {
				return nil, err
			}
			return usecase.Output{"removed": true}, nil
		}),
	}, billingTarget)
}

// billingOf bills each exported record into its Cost: the rate of its collection, else of its
// hub, on its day, and its billable value - 0 where the record is not billable.
func billingOf(placed []timerepo.Placed, costs []Cost, rates domain.BillingRates) {
	for i, p := range placed {
		rate, ok := rates.For(p.HubID, p.CollectionID, p.Day)
		if !ok {
			continue
		}
		costs[i].Bill = Billing{Billed: true, HourMinor: rate.HourMinor, Currency: rate.Currency,
			ValueMinor: domain.BillableValueMinor(p.Minutes, p.Billable, rate.HourMinor)}
	}
}

// plansOf answers the hubs and collections the records lie in, each once.
func plansOf(placed []timerepo.Placed) []shared.ID {
	seen := map[shared.ID]bool{}
	var out []shared.ID
	for _, p := range placed {
		for _, plan := range []shared.ID{p.HubID, p.CollectionID} {
			if !plan.IsZero() && !seen[plan] {
				seen[plan] = true
				out = append(out, plan)
			}
		}
	}
	return out
}

// Billing is what one exported record is billed: the billing rate holding on its day, in that
// rate's currency, and its billable value; none where no billing rate holds.
type Billing struct {
	Billed     bool
	HourMinor  int64
	Currency   string
	ValueMinor int64
}
