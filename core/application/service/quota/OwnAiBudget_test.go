// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"context"
	"errors"
	"reflect"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

type askingAuthorizer struct {
	asked   []access.Request
	refused error
}

func (a *askingAuthorizer) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.asked = append(a.asked, request)
	return a.refused
}

type trailFake struct{ entries []audit.Entry }

func (s *trailFake) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

func administrator() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.ID("018f2a1b-0000-7000-8000-0000000000t1"),
		Scopes: []string{aiManage},
	}
}

func ownBudgetWorld() (SetOwnAiBudget, *storeFake, *askingAuthorizer, *trailFake) {
	store := &storeFake{zone: "UTC"}
	authorizer := &askingAuthorizer{}
	trail := &trailFake{}
	return SetOwnAiBudget{
		Store: store, Usage: &usageFake{metered: 40}, Authorizer: authorizer, Audit: trail,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), Tenancy: env.TenancyMulti,
	}, store, authorizer, trail
}

// UC-AI-06 check 6: the workspace sets its own model's budget through the registry, as every door
// does; the answer is the two AI rows side by side; a null clears it; the trail says from and to.
func TestAWorkspaceSetsItsOwnModelsBudget(t *testing.T) {
	handler, store, authorizer, trail := ownBudgetWorld()
	registry, err := usecase.NewRegistry(nil, handler.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	out, err := registry.Invoke(t.Context(), SetOwnAiBudgetName, administrator(),
		usecase.Input{"own_tokens_per_day": float64(5000)})
	if err != nil {
		t.Fatalf("setting: %v", err)
	}
	if store.overrides.AiOwnTokensPerDay == nil || *store.overrides.AiOwnTokensPerDay != 5000 {
		t.Errorf("stored %v", store.overrides.AiOwnTokensPerDay)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 2 || rows[0]["quota"] != AiTokensPerDay || rows[1]["quota"] != AiOwnTokensPerDay ||
		rows[1]["limit"] != int64(5000) || rows[1]["used"] != int64(40) {
		t.Errorf("the answer %+v", rows)
	}
	if request := authorizer.asked[0]; request.Permission != service.PermissionStructure ||
		request.Alternative != "" || request.TokenScope != aiManage {
		t.Errorf("asked for %+v - setting a budget is an administrator's, not an auditor's", request)
	}

	if _, err := registry.Invoke(t.Context(), SetOwnAiBudgetName, administrator(),
		usecase.Input{"own_tokens_per_day": nil}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if store.overrides.AiOwnTokensPerDay != nil {
		t.Errorf("a null left %v standing", *store.overrides.AiOwnTokensPerDay)
	}
	if len(trail.entries) != 2 || trail.entries[1].Action != OwnAiBudgetSetAction ||
		trail.entries[1].Severity != audit.SeverityNotice {
		t.Fatalf("trail %+v", trail.entries)
	}
	change, _ := trail.entries[1].Changes[AiOwnTokensPerDay].(map[string]any)
	if change["from"] != "5000" || change["to"] != "none" {
		t.Errorf("the trail records %+v", trail.entries[1].Changes)
	}
}

func TestTheOwnBudgetRefusesNoughtAndTheUnpermitted(t *testing.T) {
	handler, store, authorizer, _ := ownBudgetWorld()
	nought := int64(0)
	_, err := handler.Execute(t.Context(), administrator(), &nought)
	if !errors.Is(err, shared.ErrValidation) || shared.AsError(err).DetailCode != "ai.budget_invalid" {
		t.Errorf("nought answered %v", err)
	}

	authorizer.refused = shared.ErrForbidden.WithDetail("access.not_permitted")
	tokens := int64(10)
	if _, err := handler.Execute(t.Context(), administrator(), &tokens); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a member answered %v", err)
	}
	if !reflect.DeepEqual(store.overrides, repository.Overrides{}) {
		t.Errorf("a refused request wrote %+v", store.overrides)
	}
}
