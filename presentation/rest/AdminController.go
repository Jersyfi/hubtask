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
	answer := openapi.ProvisionedTenant{
		Id: tenant.Id, Slug: tenant.Slug, DisplayName: tenant.DisplayName,
		Status:        openapi.ProvisionedTenantStatus(tenant.Status),
		DefaultLocale: tenant.DefaultLocale, DefaultTimeZone: tenant.DefaultTimeZone,
		CreatedAt:           tenant.CreatedAt,
		OwnerAccountId:      uuidValue(out.String("owner_account_id")),
		DefaultHubId:        uuidValue(out.String("default_hub_id")),
		ExampleCollectionId: uuidValue(out.String("example_collection_id")),
		OwnerDelivery:       openapi.ProvisionedTenantOwnerDelivery(out.String("owner_delivery")),
	}
	// Only where no mail carries the way in (UC-INS-07 check 3).
	if token := out.String("owner_redemption_token"); token != "" {
		answer.OwnerRedemptionToken = &token
	}
	return answer
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

func quotaStandingsResponse(out usecase.Output) []openapi.QuotaStanding {
	rows, _ := out["data"].([]usecase.Output)
	standings := make([]openapi.QuotaStanding, 0, len(rows))
	for _, row := range rows {
		standings = append(standings, quotaStandingResponse(row))
	}
	return standings
}

// workspaceNearQuotaResponse maps one workspace the census found near a quota.
func workspaceNearQuotaResponse(out usecase.Output) openapi.WorkspaceNearQuota {
	answer := openapi.WorkspaceNearQuota{
		TenantId: uuidValue(out.String("tenant_id")), Slug: out.String("slug"),
		Quotas: []openapi.QuotaStanding{},
	}
	rows, _ := out["quotas"].([]usecase.Output)
	for _, row := range rows {
		answer.Quotas = append(answer.Quotas, quotaStandingResponse(row))
	}
	return answer
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

// changeTenantAddressUseCase is ChangeTenantAddress's use case, written out longhand for
// ListServiceAccounts' reason: the identity helper's closure gives the linter nothing to trace the
// context through.
const changeTenantAddressUseCase = "ChangeTenantAddress"

// ChangeTenantAddress answers POST /admin/tenants/{tenantId}:change-address.
func (c *RestController) ChangeTenantAddress(
	w http.ResponseWriter, r *http.Request,
	tenantID openapi.AdminTenantId, params openapi.ChangeTenantAddressParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.TenantAddressChange
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{
		"tenant_id":     tenantID.String(),
		"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
	}
	if body.Slug != nil {
		in["slug"] = *body.Slug
	}
	if body.Host != nil {
		in["host"] = *body.Host
	}
	if body.DryRun != nil {
		in["dry_run"] = *body.DryRun
	}
	out, err := c.UseCases.Invoke(r.Context(), changeTenantAddressUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	tenant, _ := out["tenant"].(usecase.Output)
	mailed, _ := out["members_mailed"].(int)
	told, _ := out["members_told_at_sign_in"].(int)
	dryRun, _ := out["dry_run"].(bool)
	answer := openapi.TenantAddressChanged{
		Tenant: adminTenantResponse(tenant), From: out.String("from"),
		MembersMailed: mailed, MembersToldAtSignIn: told, DryRun: dryRun,
	}
	if to := out.String("to"); to != "" {
		answer.To = &to
	}
	writeJSON(w, r, http.StatusOK, answer)
}
