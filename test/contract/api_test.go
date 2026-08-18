// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	usecase "github.com/Jersyfi/hubtask/core/application/service/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

func contractSpec(t *testing.T) *specification {
	t.Helper()
	spec, err := loadSpec()
	if err != nil {
		t.Fatalf("%v", err)
	}
	return spec
}

func declaredRoutes(t *testing.T) map[string]*operation {
	t.Helper()
	routes, err := contractSpec(t).Routes(rest.APIBasePath)
	if err != nil {
		t.Fatalf("reading the specification's paths: %v", err)
	}
	return routes
}

// The router is generated from the specification, so the two cannot disagree by accident - but
// they can disagree after a hand-written registration, a base path change, or an operation
// removed from the document and not from the code. This is the test that would catch it
// (ADR-0004).
func TestTheRouterServesExactlyTheSpecifiedRoutes(t *testing.T) {
	declared := declaredRoutes(t)
	registered := rest.NewRestController().Routes().Routes()

	if len(declared) == 0 {
		t.Fatal("the specification declares no operations - the parser no longer matches")
	}

	for _, route := range registered {
		if _, found := declared[route]; !found {
			t.Errorf("the router serves %q, which the specification does not declare", route)
		}
	}
	for route := range declared {
		if !slices.Contains(registered, route) {
			t.Errorf("the specification declares %q, which the router does not serve", route)
		}
	}
}

// The generated code does not carry the security requirement, so the list of public routes is
// written out in the REST layer. This is what keeps it honest: a route the specification exempts
// and the code does not is a client locked out, and the reverse is an endpoint open to everyone.
func TestThePublicRoutesAreTheOnesTheSpecificationExempts(t *testing.T) {
	for route, operation := range declaredRoutes(t) {
		public := rest.PublicRoutes[route]

		switch {
		case operation.IsPublic() && !public:
			t.Errorf("%s (%s) is public in the specification and authenticated in the code",
				route, operation.OperationID)
		case !operation.IsPublic() && public:
			t.Errorf("%s (%s) needs a credential in the specification and is public in the code",
				route, operation.OperationID)
		}
	}
}

type manifest struct{ result usecase.Capabilities }

func (m manifest) Execute(context.Context, appshared.ActorContext) (usecase.Capabilities, error) {
	return m.result, nil
}

func fetchCapabilities(t *testing.T, result usecase.Capabilities) (int, []byte) {
	t.Helper()
	controller := rest.NewRestController()
	controller.Capabilities = manifest{result: result}

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, rest.APIBasePath+"/meta/capabilities", nil))
	return response.Code, response.Body.Bytes()
}

// The acceptance criterion of A-06: the response is judged by the schema in api/openapi.yaml,
// not by a copy of it in a test.
func TestTheCapabilityManifestMatchesTheSchema(t *testing.T) {
	status, body := fetchCapabilities(t, usecase.Capabilities{
		ProductVersion: "0.1.0",
		APIVersion:     usecase.APIVersion,
		TenancyMode:    "single",
		ItemTypes: []work.CapabilityProfile{
			{
				Type: work.ItemTask,
				Capabilities: []work.Capability{
					work.CapabilityCompletion, work.CapabilityCover, work.CapabilityRecurrence,
				},
				AllowedChildTypes: []work.ItemType{work.ItemWorkPackage},
				MaxDepth:          3,
			},
			{
				Type:              work.ItemActivity,
				Capabilities:      []work.Capability{work.CapabilityCompletion},
				AllowedChildTypes: []work.ItemType{},
				MaxDepth:          1,
			},
		},
		Limits:   map[string]int64{"max_body_bytes": 1 << 20},
		Features: map[string]bool{"mail": false, "tracing": true},
	})

	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}

	problems, err := contractSpec(t).validateAgainst("Capabilities", body)
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("Capabilities: %s", problem)
	}
}

// An item type the specification does not know is a contract break dressed as data: the enum in
// the document is what a client generates its own types from.
func TestAnUnknownItemTypeIsCaughtByTheSchema(t *testing.T) {
	_, body := fetchCapabilities(t, usecase.Capabilities{
		ItemTypes: []work.CapabilityProfile{{Type: work.ItemType("MILESTONE"), MaxDepth: 1}},
	})

	problems, err := contractSpec(t).validateAgainst("Capabilities", body)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(problems) == 0 {
		t.Error("an item type outside the specification's enum was accepted")
	}
}

// fetchProblem goes through the observability wrapper rather than straight to the router,
// because the request ID a problem document carries is put there by that wrapper. A test that
// skipped it would assert on a field production sets and the test never did.
func fetchProblem(t *testing.T, path string) (int, []byte) {
	t.Helper()
	routes := rest.NewRestController().Routes()

	response := httptest.NewRecorder()
	rest.Observed{
		Router: routes,
		Tracer: noop.NewTracerProvider().Tracer("contract"),
		Role:   "api",
	}.ServeHTTP(response, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, path, nil))
	return response.Code, response.Body.Bytes()
}

// Every error of this API is a problem document, including the ones the router answers for
// itself. They are part of the contract like any payload (api-guidelines.md §6).
func TestEveryErrorMatchesTheProblemSchema(t *testing.T) {
	spec := contractSpec(t)

	cases := map[string]struct {
		path       string
		wantStatus int
	}{
		"an unknown route":      {rest.APIBasePath + "/nothing-here", http.StatusNotFound},
		"a pending operation":   {rest.APIBasePath + "/containers", http.StatusNotFound},
		"outside the base path": {"/nothing-here", http.StatusNotFound},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			status, body := fetchProblem(t, c.path)
			if status != c.wantStatus {
				t.Fatalf("status %d, want %d: %s", status, c.wantStatus, body)
			}

			problems, err := spec.validateAgainst("Problem", body)
			if err != nil {
				t.Fatalf("%v", err)
			}
			for _, problem := range problems {
				t.Errorf("Problem: %s", problem)
			}

			// The two fields a client and a support request depend on.
			decoded := decode(t, body)
			if decoded["code"] == "" {
				t.Error("the problem carries no stable code")
			}
			if _, ok := decoded["request_id"]; !ok {
				t.Error("the problem carries no request ID")
			}
		})
	}
}

// The codes of the error model are part of the contract, and the document's own example list is
// where a client reads them. This keeps the mapping honest in the direction that matters: a
// status the adapter produces for a category has to be the one the guidelines name.
func TestTheErrorModelMapsOntoTheDocumentedStatuses(t *testing.T) {
	cases := map[error]int{
		shared.ErrValidation:       http.StatusUnprocessableEntity,
		shared.ErrMalformedRequest: http.StatusBadRequest,
		shared.ErrUnauthenticated:  http.StatusUnauthorized,
		shared.ErrForbidden:        http.StatusForbidden,
		shared.ErrNotFound:         http.StatusNotFound,
		shared.ErrConflict:         http.StatusConflict,
		shared.ErrVersionConflict:  http.StatusConflict,
		shared.ErrGone:             http.StatusGone,
		shared.ErrRateLimited:      http.StatusTooManyRequests,
		shared.ErrUnavailable:      http.StatusServiceUnavailable,
		shared.ErrInternal:         http.StatusInternalServerError,
	}

	spec := contractSpec(t)
	for err, wantStatus := range cases {
		response := httptest.NewRecorder()
		rest.WriteProblem(response, err, "01J9TESTREQUESTID")

		if response.Code != wantStatus {
			t.Errorf("%v answered %d, want %d (api-guidelines.md §6)", err, response.Code, wantStatus)
		}
		problems, verr := spec.validateAgainst("Problem", response.Body.Bytes())
		if verr != nil {
			t.Fatalf("%v", verr)
		}
		for _, problem := range problems {
			t.Errorf("%v: %s", err, problem)
		}
	}
}

// The completion actions answer with a WorkItem, and the state they answer with is the one a create never
// produces: completed, with both fields of the completion answered (B-07).
func TestTheCompletionResponsesMatchTheSchema(t *testing.T) {
	spec := contractSpec(t)
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	completedAt := at.Add(time.Hour)
	itemID := "0192f000-0000-7000-8000-00000000000e"

	completed := catalogue.Output{
		"id":            itemID,
		"type":          "ACTIVITY",
		"collection_id": "0192f000-0000-7000-8000-00000000000b",
		"parent_id":     "0192f000-0000-7000-8000-00000000000f",
		"path":          "/0192f000-0000-7000-8000-00000000000f/" + itemID + "/",
		"depth":         2,
		"title":         "Order the cable",
		"completion": map[string]any{
			"is_completed": true,
			"completed_at": completedAt,
			"completed_by": "0192f000-0000-7000-8000-00000000000d",
		},
		"order_key":  "a0",
		"created_by": "0192f000-0000-7000-8000-00000000000d",
		"created_at": at,
		"updated_at": completedAt,
		"version":    4,
	}
	reopened := catalogue.Output{}
	for field, value := range completed {
		reopened[field] = value
	}
	reopened["completion"] = map[string]any{
		"is_completed": false, "completed_at": nil, "completed_by": nil,
	}
	reopened["version"] = 5

	for name, c := range map[string]struct {
		action string
		out    catalogue.Output
	}{
		"complete": {"complete", completed},
		"reopen":   {"reopen", reopened},
	} {
		t.Run(name, func(t *testing.T) {
			controller := rest.NewRestController()
			controller.UseCases = fixedCatalogue{out: c.out}

			ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
				Kind:      appshared.ActorUser,
				TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
				AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
			})

			response := httptest.NewRecorder()
			controller.Routes().ServeHTTP(response, httptest.NewRequestWithContext(
				ctx, http.MethodPost, rest.APIBasePath+"/items/"+itemID+":"+c.action, nil))

			if response.Code != http.StatusOK {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}
			problems, err := spec.validateAgainst("WorkItem", response.Body.Bytes())
			if err != nil {
				t.Fatalf("%v", err)
			}
			for _, problem := range problems {
				t.Errorf("WorkItem: %s", problem)
			}
		})
	}
}

// fixedCatalogue answers every invocation with one output. What a use case would decide is not this test's
// subject - the shape of what the adapter writes is.
type fixedCatalogue struct{ out catalogue.Output }

func (c fixedCatalogue) Invoke(
	context.Context, string, appshared.ActorContext, catalogue.Input,
) (catalogue.Output, error) {
	return c.out, nil
}
