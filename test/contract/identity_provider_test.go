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

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
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
