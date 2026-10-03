// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// SC-21, ADR-0076 §5: `enabled` on a provider's configuration is deprecated, not removed - a client
// that echoes it keeps working until the next major version - and a changed value is refused as a
// problem the client can read, pointing to the list of ways to sign in.

func TestTheProviderSwitchOnTheFormIsDeprecatedAndStillAccepted(t *testing.T) {
	configuration := contractSpec(t).Components.Schemas["IdentityProviderConfiguration"]
	if configuration == nil {
		t.Fatal("the specification has no IdentityProviderConfiguration")
	}
	enabled, ok := configuration.Properties["enabled"]
	if !ok {
		t.Fatal("`enabled` was removed before the next major version (ADR-0076 §5)")
	}
	if !enabled.Deprecated {
		t.Error("`enabled` is not marked deprecated (ADR-0076 §5)")
	}
}

// recordingCatalogue answers one refusal and remembers what the controller handed it.
type recordingCatalogue struct {
	refusal error
	name    string
	input   catalogue.Input
}

func (c *recordingCatalogue) Invoke(
	_ context.Context, name string, _ appshared.ActorContext, in catalogue.Input,
) (catalogue.Output, error) {
	c.name, c.input = name, in
	return nil, c.refusal
}

func TestAChangedProviderSwitchIsAProblemAndTheFieldReachesTheUseCase(t *testing.T) {
	spec := contractSpec(t)
	recording := &recordingCatalogue{refusal: shared.ErrValidation.
		WithDetail("identity_provider.switch_in_list").
		WithFields(shared.FieldError{Path: "/enabled", Code: "identity_provider.switch_in_list"})}
	controller := rest.NewRestController()
	controller.UseCases = recording

	ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	body := []byte(`{"issuer":"https://login.example.org","client_id":"hubtask","enabled":false}`)
	request := httptest.NewRequestWithContext(ctx, http.MethodPut,
		rest.APIBasePath+"/identity-providers/0192f000-0000-7000-8000-0000000000aa", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	// The deprecated field is not dropped on the way in: the use case decides, so the refusal and
	// the echo are both the use case's to give.
	if given, held := recording.input["enabled"]; !held || given != false {
		t.Errorf("the use case %q was handed enabled = %v (held %v)", recording.name, given, held)
	}
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", response.Code, response.Body.String())
	}
	problems, err := spec.validateAgainst("Problem", response.Body.Bytes())
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("Problem: %s", problem)
	}
	if !strings.Contains(response.Body.String(), "identity_provider.switch_in_list") {
		t.Errorf("the problem does not name the list: %s", response.Body.String())
	}
}

// SC-20, ADR-0076 §2-3: the withdrawal is served, its body is optional - none is the default
// notice - and what it answers is the operator's projection, with the count and the date.

// answeringCatalogue remembers the input and answers one output.
type answeringCatalogue struct {
	answer catalogue.Output
	inputs []catalogue.Input
	names  []string
}

func (c *answeringCatalogue) Invoke(
	_ context.Context, name string, _ appshared.ActorContext, in catalogue.Input,
) (catalogue.Output, error) {
	c.names, c.inputs = append(c.names, name), append(c.inputs, in)
	return c.answer, nil
}

func TestTheWithdrawalIsServedAndAnswersTheOperatorsProjection(t *testing.T) {
	spec := contractSpec(t)
	at := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	answering := &answeringCatalogue{answer: adminservice.InstanceProviderOutput(domain.IdentityProvider{
		ID:     shared.MustParseID("0192f000-0000-7000-8000-0000000000aa"),
		Issuer: "https://login.platform.example", ClientID: "hubtask", DisplayName: "The platform",
		Kind: domain.KindGeneric, Provisioning: domain.ProvisionInvitedOnly, Enabled: true,
		CreatedAt: at.Add(-time.Hour), Version: 2, WithdrawAt: at, OfferedWorkspaces: 12,
	})}
	controller := rest.NewRestController()
	controller.UseCases = answering
	ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	route := rest.APIBasePath + "/admin/identity-providers/0192f000-0000-7000-8000-0000000000aa"

	for _, probe := range []struct {
		path, body string
	}{
		{route + ":withdraw", ``},
		{route + ":withdraw", `{"withdraw_at":"2026-10-02T09:00:00Z","confirm_count":12}`},
		{route + ":cancel-withdrawal", ``},
	} {
		request := httptest.NewRequestWithContext(ctx, http.MethodPost, probe.path,
			strings.NewReader(probe.body))
		if probe.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		controller.Routes().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s answered %d: %s", probe.path, response.Code, response.Body.String())
		}
		problems, err := spec.validateAgainst("IdentityProvider", response.Body.Bytes())
		if err != nil {
			t.Fatalf("%v", err)
		}
		for _, problem := range problems {
			t.Errorf("IdentityProvider: %s", problem)
		}
		for _, field := range []string{`"offered_workspaces":12`, `"withdraw_at":"2026-10-16T09:00:00Z"`} {
			if !strings.Contains(response.Body.String(), field) {
				t.Errorf("%s does not answer %s: %s", probe.path, field, response.Body.String())
			}
		}
	}

	if _, held := answering.inputs[0]["withdraw_at"]; held {
		t.Error("an empty body named a date; none is the default notice")
	}
	if answering.inputs[1]["withdraw_at"] != "2026-10-02T09:00:00Z" || answering.inputs[1]["confirm_count"] != 12 {
		t.Errorf("Withdraw now reached the use case as %v", answering.inputs[1])
	}
	if answering.names[2] != adminservice.CancelInstanceIdentityProviderWithdrawalName {
		t.Errorf("the cancellation reached %q", answering.names[2])
	}
}
