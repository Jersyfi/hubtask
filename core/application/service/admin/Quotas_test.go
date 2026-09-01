// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	quotarepo "github.com/Jersyfi/hubtask/core/application/repository/quota"
	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

type quotaStoreFake struct {
	overrides quotarepo.Overrides
	versions  []int
	written   bool
	writeOK   bool
}

func (s *quotaStoreFake) Overrides(context.Context) (quotarepo.Overrides, error) {
	return s.overrides, nil
}

func (s *quotaStoreFake) SetOverrides(
	_ context.Context, overrides quotarepo.Overrides, expectedVersion int, _ time.Time,
) (bool, error) {
	s.versions = append(s.versions, expectedVersion)
	if !s.writeOK {
		return false, nil
	}
	s.overrides = overrides
	s.written = true
	return true, nil
}

type quotaUsageFake struct{}

func (quotaUsageFake) Items(context.Context) (int64, error)          { return 3, nil }
func (quotaUsageFake) MediaBytes(context.Context) (int64, error)     { return 0, nil }
func (quotaUsageFake) WebhookTargets(context.Context) (int64, error) { return 0, nil }
func (quotaUsageFake) LiveExports(context.Context) (int64, error)    { return 0, nil }
func (quotaUsageFake) AutomationRunsSince(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func hold(v int64) *int64 { return &v }

type quotasFixture struct {
	handler UpdateTenantQuotas
	tenants *tenantsStore
	store   *quotaStoreFake
	audit   *auditSink
	work    *unitOfWork
}

func newQuotasFixture() *quotasFixture {
	f := &quotasFixture{
		tenants: &tenantsStore{record: adminrepo.TenantRecord{
			ID: lifecycleTenant, Slug: "acme", Status: domain.TenantActive, Version: 7,
		}},
		store: &quotaStoreFake{writeOK: true, overrides: quotarepo.Overrides{Items: hold(100)}},
		audit: &auditSink{}, work: &unitOfWork{},
	}
	f.handler = UpdateTenantQuotas{
		Tenants: f.tenants, Store: f.store, Usage: quotaUsageFake{}, Audit: f.audit,
		UnitOfWork: f.work, Clock: clock.Fixed(now), Tenancy: env.TenancyMulti,
	}
	return f
}

// The write is partial by design: a value sets, a nil clears, an absent key stays untouched -
// and the trail records the movement field by field.
func TestTheQuotaWriteIsPartialAndAudited(t *testing.T) {
	f := newQuotasFixture()
	f.store.overrides = quotarepo.Overrides{
		Items: hold(100), WebhookTargets: hold(5),
	}

	standings, err := f.handler.Execute(t.Context(), operator(), UpdateTenantQuotasCommand{
		TenantID: lifecycleTenant,
		Changes: []QuotaChange{
			{Quota: quotaservice.Items, Value: hold(200)}, // moved
			{Quota: quotaservice.WebhookTargets},          // cleared back to the default
		},
	})
	if err != nil {
		t.Fatalf("writing: %v", err)
	}

	if f.store.overrides.Items == nil || *f.store.overrides.Items != 200 {
		t.Errorf("items override %v", f.store.overrides.Items)
	}
	if f.store.overrides.WebhookTargets != nil {
		t.Error("the cleared override survived")
	}
	if len(f.store.versions) != 1 || f.store.versions[0] != 7 {
		t.Errorf("version guard %v", f.store.versions)
	}

	byName := map[string]quotaservice.Standing{}
	for _, standing := range standings {
		byName[standing.Quota] = standing
	}
	if byName[quotaservice.Items].Limit != 200 || !byName[quotaservice.Items].Configured {
		t.Errorf("items standing %+v", byName[quotaservice.Items])
	}
	if byName[quotaservice.WebhookTargets].Limit != 50 || byName[quotaservice.WebhookTargets].Configured {
		t.Errorf("webhook standing %+v, want the multi default back", byName[quotaservice.WebhookTargets])
	}

	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantQuotasChangedAction {
		t.Fatalf("audit %+v", f.audit.entries)
	}
}

func TestTheQuotaWriteRefusesTheUnknownAndTheNegative(t *testing.T) {
	f := newQuotasFixture()
	var domainErr *shared.Error

	_, err := f.handler.Execute(t.Context(), operator(), UpdateTenantQuotasCommand{
		TenantID: lifecycleTenant,
		Changes:  []QuotaChange{{Quota: "warp_cores", Value: hold(1)}},
	})
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "admin.quota_unknown" {
		t.Errorf("answer %v, want admin.quota_unknown", err)
	}

	_, err = f.handler.Execute(t.Context(), operator(), UpdateTenantQuotasCommand{
		TenantID: lifecycleTenant,
		Changes:  []QuotaChange{{Quota: quotaservice.Items, Value: hold(-1)}},
	})
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "admin.quota_negative" {
		t.Errorf("answer %v, want admin.quota_negative", err)
	}
	if f.store.written {
		t.Error("a refused request wrote")
	}
}

func TestTheQuotaWriteIsGuardedOnTheVersion(t *testing.T) {
	f := newQuotasFixture()
	f.store.writeOK = false

	_, err := f.handler.Execute(t.Context(), operator(), UpdateTenantQuotasCommand{
		TenantID: lifecycleTenant,
		Changes:  []QuotaChange{{Quota: quotaservice.Items, Value: hold(1)}},
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("answer %v, want the version conflict", err)
	}
	if len(f.audit.entries) != 0 {
		t.Error("a lost race left an audit entry")
	}
}

// The registry round trip carries the tri-state: a number sets, an explicit null clears.
func TestTheQuotaWriteRoundTripsThroughTheRegistry(t *testing.T) {
	f := newQuotasFixture()
	registry, err := usecase.NewRegistry(nil, f.handler.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	out, err := registry.Invoke(t.Context(), UpdateTenantQuotasName, operator(), usecase.Input{
		"tenant_id": lifecycleTenant.String(),
		"quotas": map[string]any{
			quotaservice.ExportJobs: float64(4),
			quotaservice.Items:      nil,
		},
	})
	if err != nil {
		t.Fatalf("writing through the registry: %v", err)
	}
	if f.store.overrides.ExportJobs == nil || *f.store.overrides.ExportJobs != 4 {
		t.Errorf("export override %v", f.store.overrides.ExportJobs)
	}
	if f.store.overrides.Items != nil {
		t.Error("the explicit null did not clear")
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 6 {
		t.Errorf("output %v", out)
	}
}
