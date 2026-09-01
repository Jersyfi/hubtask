// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The use case names of the control plane (H-06). All four are written out longhand for
// ListServiceAccounts' reason: the identity helper's closure gives the linter nothing to trace
// the context through.
const (
	listTenantsUseCase           = "ListTenants"
	provisionTenantUseCase       = "ProvisionTenant"
	suspendTenantUseCase         = "SuspendTenant"
	resumeTenantUseCase          = "ResumeTenant"
	requestTenantDeletionUseCase = "RequestTenantDeletion"
	exportTenantUseCase          = "ExportTenant"
)

// ListTenants answers GET /admin/tenants.
func (c *RestController) ListTenants(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), listTenantsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	rows, _ := out["data"].([]usecase.Output)
	tenants := make([]openapi.AdminTenant, 0, len(rows))
	for _, row := range rows {
		tenants = append(tenants, adminTenantResponse(row))
	}
	writeJSON(w, r, http.StatusOK, tenants)
}

// ProvisionTenant answers POST /admin/tenants.
func (c *RestController) ProvisionTenant(
	w http.ResponseWriter, r *http.Request, _ openapi.ProvisionTenantParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.TenantProvision
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), provisionTenantUseCase, actorOf(r), usecase.Input{
		"slug":               body.Slug,
		"display_name":       body.DisplayName,
		"default_locale":     optionalStringField(body.DefaultLocale),
		"default_time_zone":  optionalStringField(body.DefaultTimeZone),
		"owner_email":        string(body.OwnerEmail),
		"owner_display_name": optionalStringField(body.OwnerDisplayName),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	tenant := adminTenantResponse(out)
	provisioned := openapi.ProvisionedTenant{
		Id: tenant.Id, Slug: tenant.Slug, DisplayName: tenant.DisplayName,
		Status:        openapi.ProvisionedTenantStatus(tenant.Status),
		DefaultLocale: tenant.DefaultLocale, DefaultTimeZone: tenant.DefaultTimeZone,
		CreatedAt:           tenant.CreatedAt,
		OwnerAccountId:      uuidValue(out.String("owner_account_id")),
		DefaultHubId:        uuidValue(out.String("default_hub_id")),
		ExampleCollectionId: uuidValue(out.String("example_collection_id")),
		// The one response the owner's way in ever appears in (T-18's "shown once").
		OwnerRedemptionToken: out.String("owner_redemption_token"),
	}
	w.Header().Set("Location", APIBasePath+"/admin/tenants")
	writeJSON(w, r, http.StatusCreated, provisioned)
}

// SuspendTenant answers POST /admin/tenants/{tenantId}:suspend.
func (c *RestController) SuspendTenant(
	w http.ResponseWriter, r *http.Request, tenantID openapi.AdminTenantId,
) {
	c.lifecycleShift(w, r, suspendTenantUseCase, tenantID)
}

// ResumeTenant answers POST /admin/tenants/{tenantId}:resume.
func (c *RestController) ResumeTenant(
	w http.ResponseWriter, r *http.Request, tenantID openapi.AdminTenantId,
) {
	c.lifecycleShift(w, r, resumeTenantUseCase, tenantID)
}

func (c *RestController) lifecycleShift(
	w http.ResponseWriter, r *http.Request, useCase string, tenantID openapi.AdminTenantId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	_, err := c.UseCases.Invoke(r.Context(), useCase, actorOf(r), usecase.Input{
		"tenant_id": tenantID.String(),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if locale := row.String("default_locale"); locale != "" {
		tenant.DefaultLocale = &locale
	}
	if zone := row.String("default_time_zone"); zone != "" {
		tenant.DefaultTimeZone = &zone
	}
	return tenant
}

// RequestTenantDeletion answers POST /admin/tenants/{tenantId}:delete. Written out longhand for
// the same reason as its siblings above.
func (c *RestController) RequestTenantDeletion(
	w http.ResponseWriter, r *http.Request,
	tenantID openapi.AdminTenantId, params openapi.RequestTenantDeletionParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.TenantDeletionRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), requestTenantDeletionUseCase, actorOf(r), usecase.Input{
		"tenant_id":     tenantID.String(),
		"confirmation":  body.Confirmation,
		"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusAccepted, openapi.TenantDeletionScheduled{
		TenantId:   uuidValue(out.String("tenant_id")),
		PurgeAfter: timeValue(out["purge_after"]),
	})
}

// ExportTenant answers POST /admin/tenants/{tenantId}:export. Written out longhand for the same
// reason as its siblings above.
func (c *RestController) ExportTenant(
	w http.ResponseWriter, r *http.Request, tenantID openapi.AdminTenantId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.TenantExportRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), exportTenantUseCase, actorOf(r), usecase.Input{
		"tenant_id": tenantID.String(),
		"target_id": body.TargetId.String(),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	// No result_url: the archive is at somebody else's machine (the audit export's reasoning).
	writeJSON(w, r, http.StatusAccepted, openapi.JobRef{
		JobId:  uuidValue(out.String("job_id")),
		Status: openapi.JobStatusQUEUED,
	})
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
		standing := openapi.QuotaStanding{
			Quota: openapi.QuotaStandingQuota(row.String("quota")),
		}
		if limit, held := row["limit"].(int64); held {
			standing.Limit = limit
		}
		if configured, _ := row["configured"].(bool); configured {
			standing.Configured = &configured
		}
		if used, held := row["used"].(int64); held {
			standing.Used = &used
		}
		if ratio, held := row["ratio"].(float64); held {
			float := float32(ratio)
			standing.Ratio = &float
		}
		standings = append(standings, standing)
	}
	return standings
}
