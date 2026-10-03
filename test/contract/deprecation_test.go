// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// SC-28: a deprecated field says so. Every member the contract marks deprecated carries the day it
// was and the major version it goes with - what the manifest lists and the header is set from.
func TestEveryDeprecatedFieldSaysWhenAndUntilWhen(t *testing.T) {
	spec := contractSpec(t)
	found := 0
	for name, schema := range spec.Components.Schemas {
		for field, property := range schema.Properties {
			if property == nil || !property.Deprecated {
				continue
			}
			found++
			if _, err := time.Parse(time.DateOnly, property.Since); err != nil {
				t.Errorf("%s.%s is deprecated without a day in x-deprecated-since (%q)", name, field, property.Since)
			}
			if !strings.HasPrefix(property.RemovedIn, "v") {
				t.Errorf("%s.%s is deprecated without the major version in x-removed-in (%q)", name, field, property.RemovedIn)
			}
		}
	}
	if found == 0 {
		t.Fatal("no deprecated field found - the reading of the specification is broken")
	}
}

// The header is the answer to a request that sends the field, and to no other: "what you just did is
// going away", not "something on this route is".
func TestARequestThatSendsADeprecatedFieldHearsSo(t *testing.T) {
	controller := rest.NewRestController()
	controller.UseCases = &recordingCatalogue{refusal: shared.ErrValidation.WithDetail("usecase.input_invalid")}
	ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	send := func(body string) http.Header {
		t.Helper()
		request := httptest.NewRequestWithContext(ctx, http.MethodPost,
			rest.APIBasePath+"/auth/mfa:disable", bytes.NewReader([]byte(body)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		controller.Routes().ServeHTTP(response, request)
		return response.Header()
	}

	with := send(`{"password":"the old proof"}`)
	since := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC).Unix()
	if got := with.Get("Deprecation"); got != "@"+itoa(since) {
		t.Errorf("sending the deprecated field answered Deprecation %q, want @%d", got, since)
	}
	if got := with.Get("Sunset"); got != "" {
		t.Errorf("no day is set, and the answer carries Sunset %q", got)
	}
	if got := send(`{}`).Get("Deprecation"); got != "" {
		t.Errorf("a request without the field answered Deprecation %q", got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
