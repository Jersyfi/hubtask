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

// The plans (multi-tenancy.md §4.2): five handlers over five use cases, the values in the
// installation level's own shape both ways.
const (
	listPlansUseCase  = "ListPlans"
	readPlanUseCase   = "ReadPlan"
	createPlanUseCase = "CreatePlan"
	changePlanUseCase = "ChangePlan"
	removePlanUseCase = "RemovePlan"
	moveTenantPlanUse = "MoveTenantPlan"
)

// ListPlans answers GET /admin/plans.
func (c *RestController) ListPlans(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), listPlansUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	rows, _ := out["data"].([]usecase.Output)
	plans := make([]openapi.Plan, 0, len(rows))
	for _, row := range rows {
		plans = append(plans, planResponse(row))
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"data": plans})
}

// ReadPlan answers GET /admin/plans/{planName}.
func (c *RestController) ReadPlan(w http.ResponseWriter, r *http.Request, planName string) {
	c.answerPlan(w, r, http.StatusOK, readPlanUseCase, usecase.Input{"name": planName})
}

// CreatePlan answers POST /admin/plans.
func (c *RestController) CreatePlan(w http.ResponseWriter, r *http.Request, _ openapi.CreatePlanParams) {
	var body openapi.PlanCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := planValuesInput(body.DisplayName, body.SignIn, body.Legal, body.Quotas, body.Ai, body.LeftOut)
	in["name"] = body.Name
	c.answerPlan(w, r, http.StatusCreated, createPlanUseCase, in)
}

// ChangePlan answers PUT /admin/plans/{planName}.
func (c *RestController) ChangePlan(
	w http.ResponseWriter, r *http.Request, planName string, params openapi.ChangePlanParams,
) {
	var body openapi.PlanChange
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := planValuesInput(body.DisplayName, body.SignIn, body.Legal, body.Quotas, body.Ai, body.LeftOut)
	in["name"] = planName
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	c.answerPlan(w, r, http.StatusOK, changePlanUseCase, in)
}

// RemovePlan answers DELETE /admin/plans/{planName}.
func (c *RestController) RemovePlan(w http.ResponseWriter, r *http.Request, planName string) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	if _, err := c.UseCases.Invoke(r.Context(), removePlanUseCase, actorOf(r), usecase.Input{"name": planName}); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// answerPlan invokes a use case that answers one plan, and writes it with its version as the tag.
func (c *RestController) answerPlan(
	w http.ResponseWriter, r *http.Request, status int, name string, in usecase.Input,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), name, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	answer := planResponse(out)
	w.Header().Set("ETag", etag(answer.Version))
	writeJSON(w, r, status, answer)
}

func planValuesInput(
	displayName string,
	signIn, legal, quotas, ai *map[string]openapi.InstanceSetting,
	leftOut *[]openapi.PlanFeature,
) usecase.Input {
	in := usecase.Input{"display_name": displayName}
	for area, sent := range map[string]*map[string]openapi.InstanceSetting{
		"sign_in": signIn, "legal": legal, "quotas": quotas, "ai": ai,
	} {
		if sent != nil {
			in[area] = instanceSettingMap(*sent)
		}
	}
	if leftOut != nil {
		features := make([]any, 0, len(*leftOut))
		for _, feature := range *leftOut {
			features = append(features, string(feature))
		}
		in["left_out"] = features
	}
	return in
}

func planResponse(out usecase.Output) openapi.Plan {
	answer := openapi.Plan{
		Name: out.String("name"), DisplayName: out.String("display_name"),
		Version: out.Int("version"), Workspaces: int(int64Of(out["workspaces"])),
	}
	if created, held := out["created_at"].(time.Time); held {
		answer.CreatedAt = created
	}
	if updated, held := out["updated_at"].(time.Time); held {
		answer.UpdatedAt = updated
	}
	for _, area := range []struct {
		name string
		into **map[string]openapi.InstanceSetting
	}{{"sign_in", &answer.SignIn}, {"legal", &answer.Legal}, {"quotas", &answer.Quotas}, {"ai", &answer.Ai}} {
		if settings := instanceSettingsOf(out[area.name]); settings != nil {
			*area.into = &settings
		}
	}
	leftOut := []openapi.PlanFeature{}
	if features, held := out["left_out"].([]any); held {
		for _, feature := range features {
			if name, isString := feature.(string); isString {
				leftOut = append(leftOut, openapi.PlanFeature(name))
			}
		}
	}
	answer.LeftOut = &leftOut
	return answer
}

func int64Of(value any) int64 {
	switch number := value.(type) {
	case int64:
		return number
	case int:
		return int64(number)
	}
	return 0
}

// MoveTenantPlan answers POST /admin/tenants/{tenantId}:move-plan. A POST so that the replay guard
// keeps its answer under an idempotency key, as it does for every control-plane act.
func (c *RestController) MoveTenantPlan(
	w http.ResponseWriter, r *http.Request, tenantID openapi.AdminTenantId, _ openapi.MoveTenantPlanParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.TenantPlanMove
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{"tenant_id": tenantID.String()}
	if body.Plan != nil {
		in["plan"] = *body.Plan
	}
	if body.ExpectedVersion != nil {
		in["expected_version"] = *body.ExpectedVersion
	}
	out, err := c.UseCases.Invoke(r.Context(), moveTenantPlanUse, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	tenant, _ := out["tenant"].(usecase.Output)
	writeJSON(w, r, http.StatusOK, openapi.TenantPlanMoved{
		Tenant:          adminTenantResponse(tenant),
		FeaturesResting: featuresOf(out["features_resting"]),
		ExceptionsKept:  stringsOf(out["exceptions_kept"]),
	})
}

func featuresOf(value any) []openapi.PlanFeature {
	features := []openapi.PlanFeature{}
	if listed, held := value.([]any); held {
		for _, feature := range listed {
			if name, isString := feature.(string); isString {
				features = append(features, openapi.PlanFeature(name))
			}
		}
	}
	return features
}

// UpdateTenant answers PATCH /admin/tenants/{tenantId}: a platform's reference, set or cleared.
func (c *RestController) UpdateTenant(
	w http.ResponseWriter, r *http.Request, tenantID openapi.AdminTenantId, params openapi.UpdateTenantParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.TenantUpdate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{"tenant_id": tenantID.String(), "external_reference": optionalStringField(body.ExternalReference)}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	out, err := c.UseCases.Invoke(r.Context(), "UpdateTenant", actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	answer := adminTenantResponse(out)
	if answer.Version != nil {
		w.Header().Set("ETag", etag(*answer.Version))
	}
	writeJSON(w, r, http.StatusOK, answer)
}
