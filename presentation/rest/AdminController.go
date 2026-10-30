// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// provisionedTenantResponse maps a new workspace with the one answer the owner's way in ever appears in (T-18's shown once).
func provisionedTenantResponse(out usecase.Output) openapi.ProvisionedTenant {
	tenant := adminTenantResponse(out)
	return openapi.ProvisionedTenant{
		Id: tenant.Id, Slug: tenant.Slug, DisplayName: tenant.DisplayName,
		Status:        openapi.ProvisionedTenantStatus(tenant.Status),
		DefaultLocale: tenant.DefaultLocale, DefaultTimeZone: tenant.DefaultTimeZone,
		CreatedAt:            tenant.CreatedAt,
		OwnerAccountId:       uuidValue(out.String("owner_account_id")),
		DefaultHubId:         uuidValue(out.String("default_hub_id")),
		ExampleCollectionId:  uuidValue(out.String("example_collection_id")),
		OwnerRedemptionToken: out.String("owner_redemption_token"),
	}
}

func adminTenantResponse(row usecase.Output) openapi.AdminTenant {
	tenant := openapi.AdminTenant{
		Id:          uuidValue(row.String("id")),
		Slug:        row.String("slug"),
		DisplayName: row.String("display_name"),
		Status:      openapi.AdminTenantStatus(row.String("status")),
		CreatedAt:   timeValue(row["created_at"]),
		PurgeAfter:  optionalTimeField(row["purge_after"]),
	}
	if started, begun := row["purge_started_at"].(time.Time); begun {
		tenant.PurgeStartedAt = &started
	}
	if held, answered := row["legal_hold"].(bool); answered {
		tenant.LegalHold = &held
	}
	if locale := row.String("default_locale"); locale != "" {
		tenant.DefaultLocale = &locale
	}
	if zone := row.String("default_time_zone"); zone != "" {
		tenant.DefaultTimeZone = &zone
	}
	if opening, held := row["password_opening"].(usecase.Output); held {
		tenant.PasswordOpening = passwordOpeningResponse(opening)
	}
	if name := row.String("plan"); name != "" {
		tenant.Plan = &name
	}
	if reference := row.String("external_reference"); reference != "" {
		tenant.ExternalReference = &reference
	}
	if version := row.Int("version"); version > 0 {
		tenant.Version = &version
	}
	return tenant
}

// passwordOpeningResponse maps an operator's opening of the password (ADR-0078 §3), which the
// listing, the two acts on it and the workspace's own read all answer in one shape.
func passwordOpeningResponse(opening usecase.Output) *openapi.PasswordOpening {
	return &openapi.PasswordOpening{
		Until:     timeValue(opening["until"]),
		Requester: opening.String("requester"),
		Reason:    opening.String("reason"),
	}
}

// UpdateTenantQuotas answers PATCH /admin/tenants/{tenantId}/quotas. The body is read as a raw
// document rather than the generated struct: "absent", "null" and "0" are three different
// instructions, and a Go pointer can only carry two of them.
func (c *RestController) UpdateTenantQuotas(
	w http.ResponseWriter, r *http.Request, tenantID openapi.AdminTenantId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), updateTenantQuotasUseCase, actorOf(r), usecase.Input{
		"tenant_id": tenantID.String(),
		"quotas":    body,
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, quotaStandingsResponse(out))
}

func quotaStandingsResponse(out usecase.Output) []openapi.QuotaStanding {
	rows, _ := out["data"].([]usecase.Output)
	standings := make([]openapi.QuotaStanding, 0, len(rows))
	for _, row := range rows {
		standings = append(standings, quotaStandingResponse(row))
	}
	return standings
}

func quotaStandingResponse(row usecase.Output) openapi.QuotaStanding {
	standing := openapi.QuotaStanding{
		Quota: openapi.QuotaStandingQuota(row.String("quota")),
	}
	if limit, held := row["limit"].(int64); held {
		standing.Limit = limit
	}
	if configured, _ := row["configured"].(bool); configured {
		standing.Configured = &configured
	}
	if locked, _ := row["locked"].(bool); locked {
		standing.Locked = &locked
	}
	if source := row.String("source"); source != "" {
		answered := openapi.QuotaSource(source)
		standing.Source = &answered
	}
	if lock := row.String("lock"); lock != "" {
		answered := openapi.PolicyLock(lock)
		standing.Lock = &answered
	}
	if used, held := row["used"].(int64); held {
		standing.Used = &used
	}
	if ratio, held := row["ratio"].(float64); held {
		float := float32(ratio)
		standing.Ratio = &float
	}
	return standing
}
