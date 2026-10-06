// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	usecase "github.com/Jersyfi/hubtask/core/application/service/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// SC-28: a deprecated field says so. Every member the contract marks deprecated is listed in the
// manifest a client reads - judged from the specification itself, independently of the generator -
// with the day it was, the major version it goes with and what replaces it.
func TestEveryDeprecatedFieldIsInTheManifest(t *testing.T) {
	spec := contractSpec(t)
	want := map[string]bool{}
	for path, item := range spec.Paths {
		for _, method := range httpMethods {
			node, ok := item[method]
			if !ok {
				continue
			}
			var op operation
			if err := node.Decode(&op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			if op.RequestBody == nil {
				continue
			}
			body, ok := op.RequestBody.Content["application/json"]
			if !ok || body.Schema == nil || body.Schema.Ref == "" {
				continue
			}
			named := spec.Components.Schemas[strings.TrimPrefix(body.Schema.Ref, "#/components/schemas/")]
			for field, property := range named.Properties {
				if property != nil && property.Deprecated {
					want[op.OperationID+"."+field] = true
				}
			}
		}
	}
	if len(want) == 0 {
		t.Fatal("no deprecated request field found - the reading of the specification is broken")
	}

	status, raw := fetchCapabilities(t, usecase.Capabilities{APIVersion: usecase.APIVersion})
	if status != http.StatusOK {
		t.Fatalf("the manifest answered %d", status)
	}
	var answer struct {
		Deprecations []struct {
			OperationID string   `json:"operation_id"`
			Field       string   `json:"field"`
			Since       string   `json:"since"`
			RemovedIn   string   `json:"removed_in"`
			ReplacedBy  []string `json:"replaced_by"`
		} `json:"deprecations"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatalf("decoding the manifest: %v", err)
	}
	listed := map[string]bool{}
	for _, entry := range answer.Deprecations {
		key := entry.OperationID + "." + entry.Field
		listed[key] = true
		if _, err := time.Parse(time.DateOnly, entry.Since); err != nil {
			t.Errorf("%s is listed without a day (%q)", key, entry.Since)
		}
		if !strings.HasPrefix(entry.RemovedIn, "v") || len(entry.ReplacedBy) == 0 {
			t.Errorf("%s is listed without its version or what replaces it: %+v", key, entry)
		}
		if !want[key] {
			t.Errorf("the manifest lists %s, which the contract does not deprecate", key)
		}
	}
	for key := range want {
		if !listed[key] {
			t.Errorf("the contract deprecates %s and the manifest does not say so", key)
		}
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
	// Read and put back: the use case still receives the field the announcement looked at.
	if !controller.UseCases.(*recordingCatalogue).input.Present("password") {
		t.Error("the announcement swallowed the body: the use case received no password")
	}
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
