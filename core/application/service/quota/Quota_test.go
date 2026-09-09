// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var now = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func ceiling(v int64) *int64 { return &v }

// The two columns of §4, and the resolution over them: an override wins, absence keeps the
// default, and a configured 0 is "unlimited" - three states, held apart.
func TestTheResolutionLaysOverridesOverTheModesColumn(t *testing.T) {
	multi := Defaults(env.TenancyMulti)
	if multi.APIRequestsPerMinute != 600 || multi.AutomationRunsPerHour != 1_000 ||
		multi.WebhookTargets != 50 || multi.ExportJobs != 2 {
		t.Errorf("multi defaults %+v", multi)
	}
	single := Defaults(env.TenancySingle)
	if single.APIRequestsPerMinute != 6_000 || single.WebhookTargets != Unlimited ||
		single.ExportJobs != 5 || single.AutomationRunsPerHour != 100_000 {
		t.Errorf("single defaults %+v", single)
	}
	if multi.Items != Unlimited || multi.MediaBytes != Unlimited {
		t.Errorf("the plan-bound rows default bounded: %+v", multi)
	}

	resolved := Resolve(repository.Overrides{
		Items:          ceiling(1_000),
		WebhookTargets: ceiling(Unlimited),
	}, env.TenancyMulti)
	if resolved.Items != 1_000 {
		t.Errorf("the override lost: %d", resolved.Items)
	}
	if resolved.WebhookTargets != Unlimited {
		t.Errorf("a configured unlimited lost to the default: %d", resolved.WebhookTargets)
	}
	if resolved.ExportJobs != 2 {
		t.Errorf("an absent key moved the default: %d", resolved.ExportJobs)
	}
	for _, name := range Names() {
		if resolved.Of(name) < 0 {
			t.Errorf("%s resolved negative", name)
		}
	}
}

// The capacity refusal: 422-shaped, naming the quota and the ceiling.
func TestTheCapacityRefusalNamesTheQuotaAndTheCeiling(t *testing.T) {
	err := Exceeded(Items, 100, 100)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("not a domain error: %v", err)
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("category %v, want the 422 shape - waiting does not help", err)
	}
	if domainErr.DetailCode != "capacity.items" {
		t.Errorf("code %q", domainErr.DetailCode)
	}
	if domainErr.Params["limit"] != "100" || domainErr.Params["quota"] != Items {
		t.Errorf("params %v", domainErr.Params)
	}
}

// ---- fakes ----

type storeFake struct {
	overrides repository.Overrides
}

func (s *storeFake) Overrides(context.Context) (repository.Overrides, error) {
	return s.overrides, nil
}

func (s *storeFake) SetOverrides(
	_ context.Context, overrides repository.Overrides, _ int, _ time.Time,
) (bool, error) {
	s.overrides = overrides
	return true, nil
}

type usageFake struct {
	items, media, webhooks, runs, exports int64
	// metered is what the ledger holds, and the three below record how it was asked - which is
	// the interesting half for a budget measured over a period (J-15).
	metered       int64
	meteredCalls  int
	meteredMetric string
	meteredSince  time.Time
}

func (u *usageFake) Items(context.Context) (int64, error)          { return u.items, nil }
func (u *usageFake) MediaBytes(context.Context) (int64, error)     { return u.media, nil }
func (u *usageFake) WebhookTargets(context.Context) (int64, error) { return u.webhooks, nil }
func (u *usageFake) LiveExports(context.Context) (int64, error)    { return u.exports, nil }
func (u *usageFake) AutomationRunsSince(context.Context, time.Time) (int64, error) {
	return u.runs, nil
}

func (u *usageFake) MeteredSince(
	_ context.Context, metric string, since time.Time,
) (int64, error) {
	u.meteredCalls++
	u.meteredMetric, u.meteredSince = metric, since
	return u.metered, nil
}

type meterFake struct{ added map[string]int64 }

func (m *meterFake) Add(_ context.Context, metric string, _ time.Time, amount int64) error {
	if m.added == nil {
		m.added = map[string]int64{}
	}
	m.added[metric] += amount
	return nil
}

type signalsFake struct{ ratios map[string]float64 }

func (s *signalsFake) QuotaUsage(_ context.Context, quota, _ string, ratio float64) {
	if s.ratios == nil {
		s.ratios = map[string]float64{}
	}
	s.ratios[quota] = ratio
}

func guardWith(store *storeFake, usage *usageFake, signals *signalsFake) Guard {
	return Guard{Store: store, Usage: usage, Meter: &meterFake{}, Signals: signals, Tenancy: env.TenancyMulti}
}

// The guard refuses at the wall, reports the approach ratio, and an unlimited quota neither
// counts nor refuses.
func TestTheGuardRefusesAtTheWallAndReportsTheApproach(t *testing.T) {
	store := &storeFake{overrides: repository.Overrides{Items: ceiling(10)}}
	usage := &usageFake{items: 9}
	signals := &signalsFake{}
	guard := guardWith(store, usage, signals)

	if err := guard.Items(t.Context(), "t1", 1); err != nil {
		t.Fatalf("the tenth item was refused below the wall: %v", err)
	}
	if signals.ratios[Items] != 0.9 {
		t.Errorf("ratio %v, want 0.9 - A-18's number", signals.ratios[Items])
	}

	usage.items = 10
	err := guard.Items(t.Context(), "t1", 1)
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "capacity.items" {
		t.Errorf("at the wall: %v, want capacity.items", err)
	}

	// Unlimited: no count, no refusal, no ratio.
	store.overrides = repository.Overrides{}
	signals.ratios = nil
	if err := guard.Items(t.Context(), "t1", 1); err != nil {
		t.Fatalf("an unlimited quota refused: %v", err)
	}
	if len(signals.ratios) != 0 {
		t.Errorf("an unlimited quota reported a ratio: %v", signals.ratios)
	}
}

func TestEveryCapacityQuotaSharesTheOneShape(t *testing.T) {
	store := &storeFake{overrides: repository.Overrides{
		MediaBytes: ceiling(100), WebhookTargets: ceiling(1), ExportJobs: ceiling(1),
	}}
	usage := &usageFake{media: 90, webhooks: 1, exports: 1}
	guard := guardWith(store, usage, &signalsFake{})

	if err := guard.MediaBytes(t.Context(), "t1", 20); err == nil {
		t.Error("110 bytes fit under a 100-byte ceiling")
	}
	if err := guard.MediaBytes(t.Context(), "t1", 10); err != nil {
		t.Errorf("100 bytes were refused under a 100-byte ceiling: %v", err)
	}
	if err := guard.WebhookTargets(t.Context(), "t1"); err == nil {
		t.Error("a second webhook target fit under a ceiling of one")
	}
	if err := guard.ExportJobs(t.Context(), "t1"); err == nil {
		t.Error("a second export fit under a ceiling of one")
	}
}

// The automation budget answers a verdict rather than an error - the engine's vocabulary for
// "over budget" is a THROTTLED run - and every run is metered into the ledger either way.
func TestTheAutomationBudgetAnswersAVerdictAndMeters(t *testing.T) {
	store := &storeFake{overrides: repository.Overrides{AutomationRunsPerHour: ceiling(2)}}
	usage := &usageFake{runs: 1}
	meter := &meterFake{}
	guard := Guard{Store: store, Usage: usage, Meter: meter, Signals: &signalsFake{}, Tenancy: env.TenancyMulti}

	allowed, err := guard.AutomationRuns(t.Context(), "t1", now)
	if err != nil || !allowed {
		t.Fatalf("below budget: (%v, %v)", allowed, err)
	}
	usage.runs = 2
	allowed, err = guard.AutomationRuns(t.Context(), "t1", now)
	if err != nil || allowed {
		t.Fatalf("at budget: (%v, %v)", allowed, err)
	}
	if meter.added[AutomationRunsPerHour] != 2 {
		t.Errorf("the ledger counted %d, want both attempts", meter.added[AutomationRunsPerHour])
	}
}

// ---- the read use case ----

type authorizerFake struct{ refused error }

func (a authorizerFake) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return a.refused
}

type unitOfWork struct{ scopes []persistence.Scope }

func (u *unitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, scope, fn)
}

func reader() appshared.ActorContext {
	return appshared.ActorContext{
		Kind:     appshared.ActorUser,
		TenantID: shared.ID("018f2a1b-0000-7000-8000-0000000000t1"),
		Scopes:   []string{quotasRead},
	}
}

func TestTheStandingAnswersEveryQuotaInOrder(t *testing.T) {
	handler := ReadQuotas{
		Store:      &storeFake{overrides: repository.Overrides{Items: ceiling(100)}},
		Usage:      &usageFake{items: 25, media: 1024, webhooks: 3, runs: 7, exports: 1},
		Authorizer: authorizerFake{}, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), Tenancy: env.TenancyMulti,
	}

	standings, err := handler.Execute(t.Context(), reader())
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(standings) != len(Names()) {
		t.Fatalf("%d standings, want %d", len(standings), len(Names()))
	}
	byName := map[string]Standing{}
	for _, standing := range standings {
		byName[standing.Quota] = standing
	}

	items := byName[Items]
	if items.Limit != 100 || !items.Configured || items.Used == nil || *items.Used != 25 {
		t.Errorf("items %+v", items)
	}
	if ratio := items.Ratio(); ratio == nil || *ratio != 0.25 {
		t.Errorf("items ratio %v", ratio)
	}

	api := byName[APIRequestsPerMinute]
	if api.Used != nil || api.Ratio() != nil {
		t.Errorf("the request rate answered a live count: %+v", api)
	}
	if api.Limit != 600 || api.Configured {
		t.Errorf("api %+v", api)
	}

	if runs := byName[AutomationRunsPerHour]; runs.Used == nil || *runs.Used != 7 {
		t.Errorf("runs %+v", runs)
	}
}

func TestTheReadIsRefusedWithoutThePermission(t *testing.T) {
	handler := ReadQuotas{
		Store: &storeFake{}, Usage: &usageFake{},
		Authorizer: authorizerFake{refused: shared.ErrForbidden.WithDetail("access.not_permitted")},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), Tenancy: env.TenancyMulti,
	}
	if _, err := handler.Execute(t.Context(), reader()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("answer %v, want forbidden", err)
	}
}

func TestTheQuotasRoundTripThroughTheRegistry(t *testing.T) {
	handler := ReadQuotas{
		Store: &storeFake{}, Usage: &usageFake{items: 1},
		Authorizer: authorizerFake{}, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), Tenancy: env.TenancySingle,
	}
	registry, err := usecase.NewRegistry(nil, handler.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	out, err := registry.Invoke(t.Context(), ReadQuotasName, reader(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading through the registry: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != len(Names()) {
		t.Fatalf("output %v", out)
	}
	if rows[0].String("quota") != APIRequestsPerMinute || rows[0]["limit"] != int64(6_000) {
		t.Errorf("first row %v", rows[0])
	}
}

// The AI budget (J-15): §4's newest row, resolved and enforced by the same machinery as the rest.
func TestTheAiBudgetDefaultsToTheModesNumber(t *testing.T) {
	if single := Defaults(env.TenancySingle).AiTokensPerDay; single != Unlimited {
		t.Errorf("single mode defaults the AI budget to %d, want unlimited - a self-hoster's "+
			"tokens are their own", single)
	}
	if multi := Defaults(env.TenancyMulti).AiTokensPerDay; multi == Unlimited {
		t.Error("multi mode defaults the AI budget to unlimited, and AI is the one feature whose " +
			"marginal cost leaves the installation")
	}
}

// The guard reports room against what the day has spent, and reports the ratio on the way - the
// same metric A-18 watches for every other quota, which is why the alert needed no new rule.
func TestTheAiBudgetIsMeasuredAgainstTheDaysSpend(t *testing.T) {
	for name, c := range map[string]struct {
		limit, spent int64
		room         bool
	}{
		"well under":    {limit: 1000, spent: 10, room: true},
		"one short":     {limit: 1000, spent: 999, room: true},
		"exactly at it": {limit: 1000, spent: 1000, room: false},
		"well past it":  {limit: 1000, spent: 9000, room: false},
		"unlimited":     {limit: Unlimited, spent: 9_000_000, room: true},
	} {
		t.Run(name, func(t *testing.T) {
			usage := &usageFake{metered: c.spent}
			signals := &signalsFake{}
			guard := Guard{
				Store:   &storeFake{overrides: repository.Overrides{AiTokensPerDay: &c.limit}},
				Usage:   usage,
				Signals: signals,
				Tenancy: env.TenancySingle,
			}

			room, err := guard.AiTokens(t.Context(), "a-tenant",
				time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("asking the budget: %v", err)
			}
			if room != c.room {
				t.Errorf("room = %v, want %v", room, c.room)
			}
			if c.limit == Unlimited {
				// Nothing is counted and nothing is reported: an unlimited budget is not a wall
				// somebody is approaching.
				if usage.meteredCalls != 0 {
					t.Error("an unlimited budget counted the ledger anyway")
				}
				return
			}
			if _, reported := signals.ratios[AiTokensPerDay]; !reported {
				t.Errorf("the ratio was not reported: %v", signals.ratios)
			}
		})
	}
}

// The day is a UTC calendar day, which is what the ledger is keyed on - so what the guard measures
// and what `GET /quotas` shows are the same number.
func TestTheAiBudgetMeasuresACalendarDay(t *testing.T) {
	limit := int64(1000)
	usage := &usageFake{}
	guard := Guard{
		Store:   &storeFake{overrides: repository.Overrides{AiTokensPerDay: &limit}},
		Usage:   usage,
		Tenancy: env.TenancySingle,
	}

	if _, err := guard.AiTokens(t.Context(), "a-tenant",
		time.Date(2026, 9, 9, 23, 59, 0, 0, time.UTC)); err != nil {
		t.Fatalf("asking the budget: %v", err)
	}

	want := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	if !usage.meteredSince.Equal(want) {
		t.Errorf("the budget counted from %s, want the start of the UTC day %s",
			usage.meteredSince, want)
	}
	if usage.meteredMetric != AiTokensPerDay {
		t.Errorf("the budget counted %q", usage.meteredMetric)
	}
}
