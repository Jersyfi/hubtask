// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// planResponse maps a plan (multi-tenancy.md §4.2): its values in the installation level's own shape.
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

// tenantPlanMovedResponse maps a plan move: the workspace as it stands and what the move leaves resting.
func tenantPlanMovedResponse(out usecase.Output) openapi.TenantPlanMoved {
	tenant, _ := out["tenant"].(usecase.Output)
	return openapi.TenantPlanMoved{
		Tenant:          adminTenantResponse(tenant),
		FeaturesResting: featuresOf(out["features_resting"]),
		ExceptionsKept:  stringsOf(out["exceptions_kept"]),
		LimitsExceeded:  limitsExceededOf(out["limits_exceeded"]),
	}
}

func limitsExceededOf(value any) *[]openapi.LimitExceeded {
	exceeded := []openapi.LimitExceeded{}
	listed, _ := value.([]usecase.Output)
	for _, over := range listed {
		limit, _ := over["limit"].(int64)
		used, _ := over["used"].(int64)
		exceeded = append(exceeded, openapi.LimitExceeded{Quota: over.String("quota"), Limit: limit, Used: used})
	}
	return &exceeded
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
