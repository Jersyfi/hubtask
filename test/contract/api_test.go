// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	usecase "github.com/Jersyfi/hubtask/core/application/service/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
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

// The classification decides which routes are refused under load (H-11), and it is written out in
// the REST layer because the specification does not carry a class. A typo in it would be a line
// that silently never matches - the route would keep being served, the overload test would keep
// passing, and nothing would ever be shed. So every entry has to be a route that exists.
func TestTheDeferrableRoutesAreRoutesTheRouterServes(t *testing.T) {
	declared := declaredRoutes(t)
	if len(rest.DeferrableRoutes) == 0 {
		t.Fatal("no route is deferrable, so load shedding can never engage")
	}
	for route := range rest.DeferrableRoutes {
		if _, found := declared[route]; !found {
			t.Errorf("%q is classified as deferrable and is not a route the specification declares", route)
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
		Roles: []usecase.RoleDescription{
			{
				Role:        identity.RoleContributor,
				Permissions: []service.Permission{service.PermissionRead, service.PermissionWriteItems},
				ItemAccess: map[service.ItemAction]service.ItemAccess{
					service.ItemRead: service.AccessAll, service.ItemCreate: service.AccessAll,
					service.ItemChange:  service.AccessAssigned,
					service.ItemComment: service.AccessAssigned,
				},
			},
			{
				Role:        identity.RoleGuest,
				Permissions: []service.Permission{service.PermissionRead},
				ItemAccess: map[service.ItemAction]service.ItemAccess{
					service.ItemRead: service.AccessAll, service.ItemCreate: service.AccessNone,
					service.ItemChange: service.AccessNone, service.ItemComment: service.AccessAll,
				},
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
		"an unknown route": {rest.APIBasePath + "/nothing-here", http.StatusNotFound},
		// The example moves as the milestone does: it has to be an operation that genuinely
		// has no use case yet. /backup-targets was it until E-03 served it.
		"a pending operation":   {rest.APIBasePath + "/sync/devices", http.StatusNotFound},
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

// The read side's responses, judged by the schemas in api/openapi.yaml rather than by a copy of them
// here (B-04). Four of them: the two single objects, and the two pages - the page schemas are named
// components for exactly this reason, since an inline schema in a path item is one this validator
// cannot resolve.
func readResponse(t *testing.T, path string, out catalogue.Output) (int, []byte) {
	t.Helper()

	controller := rest.NewRestController()
	controller.UseCases = fixedCatalogue{out: out}

	ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, httptest.NewRequestWithContext(
		ctx, http.MethodGet, rest.APIBasePath+path, nil))
	return response.Code, response.Body.Bytes()
}

// The update's response is a WorkItem like every other, and it is worth judging separately because
// it is the one that arrives through a different content type: a merge patch, whose body the router
// has to accept before the response can be judged at all.
func TestTheUpdateResponseMatchesTheSchema(t *testing.T) {
	spec := contractSpec(t)
	itemID := "0192f000-0000-7000-8000-00000000000e"

	renamed := itemProjection()
	renamed["title"] = "Buy oat milk"
	renamed["version"] = 4

	controller := rest.NewRestController()
	controller.UseCases = fixedCatalogue{out: renamed}

	ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})

	request := httptest.NewRequestWithContext(ctx, http.MethodPatch,
		rest.APIBasePath+"/items/"+itemID, strings.NewReader(`{"title":"Buy oat milk"}`))
	request.Header.Set("Content-Type", "application/merge-patch+json")
	request.Header.Set("If-Match", `"3"`)

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	// The entity tag a client writes its next update against (api-guidelines.md §5).
	if tag := response.Header().Get("ETag"); tag != `"4"` {
		t.Errorf("ETag = %q", tag)
	}
	problems, err := spec.validateAgainst("WorkItem", response.Body.Bytes())
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("WorkItem: %s", problem)
	}
}

// fixedCatalogue answers every invocation with one output. What the use case would decide is not this
// test's subject - the shape of what the adapter writes is.
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

func containerProjection() catalogue.Output {
	at := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	return catalogue.Output{
		"id":          "0192f000-0000-7000-8000-00000000000b",
		"type":        "HUB",
		"parent_id":   nil,
		"name":        "Private",
		"description": "Everything personal",
		"icon":        "home",
		"color_token": "blue",
		"order_key":   "a0",
		"archived_at": nil,
		"deleted_at":  nil,
		"created_at":  at,
		"updated_at":  at,
		"version":     1,
	}
}

func itemProjection() catalogue.Output {
	at := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	completedAt := at.Add(time.Hour)

	return catalogue.Output{
		"id":            "0192f000-0000-7000-8000-00000000000e",
		"type":          "TASK",
		"collection_id": "0192f000-0000-7000-8000-00000000000b",
		"parent_id":     nil,
		"path":          "/0192f000-0000-7000-8000-00000000000e/",
		"depth":         1,
		"title":         "Buy milk",
		"notes":         "Semi-skimmed, two litres",
		// Completed and archived, which is the state a create never produces: the read side returns
		// this projection and the schema has to accept it.
		"completion": map[string]any{
			"is_completed": true,
			"completed_at": completedAt,
			"completed_by": "0192f000-0000-7000-8000-00000000000d",
		},
		"order_key":   "a0",
		"archived_at": completedAt,
		"deleted_at":  nil,
		"created_by":  "0192f000-0000-7000-8000-00000000000d",
		"created_at":  at,
		"updated_at":  at,
		"version":     3,
	}
}

func TestTheReadResponsesMatchTheirSchemas(t *testing.T) {
	spec := contractSpec(t)
	collection := "0192f000-0000-7000-8000-00000000000b"

	cases := map[string]struct {
		path   string
		out    catalogue.Output
		schema string
	}{
		"a container": {
			"/containers/" + collection, containerProjection(), "Container",
		},
		"an item": {
			"/items/0192f000-0000-7000-8000-00000000000e", itemProjection(), "WorkItem",
		},
		"a page of containers": {
			"/containers",
			catalogue.Output{
				"data": []catalogue.Output{containerProjection()},
				"page": map[string]any{"next_cursor": "an-opaque-cursor", "has_more": true},
			},
			"ContainerPage",
		},
		"a page of items": {
			"/items?collection_id=" + collection,
			catalogue.Output{
				"data": []catalogue.Output{itemProjection()},
				"page": map[string]any{"next_cursor": nil, "has_more": false},
			},
			"WorkItemPage",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			status, body := readResponse(t, c.path, c.out)
			if status != http.StatusOK {
				t.Fatalf("status %d: %s", status, body)
			}

			problems, err := spec.validateAgainst(c.schema, body)
			if err != nil {
				t.Fatalf("%v", err)
			}
			for _, problem := range problems {
				t.Errorf("%s: %s", c.schema, problem)
			}
		})
	}
}

// The query language's wire format, pinned in both of its shapes (B-12).
//
// Both matter separately. The ungrouped answer is a page like any other; the grouped one is the
// board projection, and it is the only response in this contract that nests a page inside a row -
// a shape that a client renders columns from, and that has to keep meaning what it means.
func TestTheQueryResponsesMatchTheSchema(t *testing.T) {
	spec := contractSpec(t)
	collection := "0192f000-0000-7000-8000-00000000000b"
	body := `{"scope": {"container_id": "` + collection + `"}}`

	cases := map[string]catalogue.Output{
		"a page of entries": {
			"data":   []catalogue.Output{itemProjection()},
			"groups": []catalogue.Output{},
			"page":   map[string]any{"next_cursor": "an-opaque-cursor", "has_more": true},
			"total":  nil,
		},
		"a board": {
			"data": []catalogue.Output{},
			"groups": []catalogue.Output{
				{
					"key":   "0192f000-0000-7000-8000-0000000000b1",
					"count": 12,
					"data":  []catalogue.Output{itemProjection()},
					"page":  map[string]any{"next_cursor": "an-opaque-cursor", "has_more": true},
				},
				{
					// The entries on no column at all: a group with a null key, which a board draws
					// at the end.
					"key":   nil,
					"count": 0,
					"data":  []catalogue.Output{},
					"page":  map[string]any{"next_cursor": nil, "has_more": false},
				},
			},
			"page":  map[string]any{"next_cursor": nil, "has_more": false},
			"total": 12,
		},
	}

	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			status, answered := queryResponse(t, body, out)
			if status != http.StatusOK {
				t.Fatalf("status %d: %s", status, answered)
			}

			problems, err := spec.validateAgainst("ItemQueryResult", answered)
			if err != nil {
				t.Fatalf("%v", err)
			}
			for _, problem := range problems {
				t.Errorf("ItemQueryResult: %s", problem)
			}
		})
	}
}

// Both collections are always present, and always arrays: the schema requires them, and a client
// that had to check for null before iterating would check on every response for a case that arises
// in only one of them.
func TestAQueryAnswersBothCollectionsWhicheverShapeItTook(t *testing.T) {
	collection := "0192f000-0000-7000-8000-00000000000b"

	_, answered := queryResponse(t, `{"scope": {"container_id": "`+collection+`"}}`, catalogue.Output{
		"data":   []catalogue.Output{itemProjection()},
		"groups": []catalogue.Output{},
		"page":   map[string]any{"next_cursor": nil, "has_more": false},
		"total":  nil,
	})

	body := decode(t, answered)
	for _, field := range []string{"data", "groups"} {
		if _, ok := body[field].([]any); !ok {
			t.Errorf("%s is %v, want an array", field, body[field])
		}
	}
	if value, present := body["total"]; !present || value != nil {
		t.Errorf("total is %v, want an explicit null", body["total"])
	}
}

func queryResponse(t *testing.T, body string, out catalogue.Output) (int, []byte) {
	t.Helper()

	controller := rest.NewRestController()
	controller.UseCases = fixedCatalogue{out: out}

	ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})

	request := httptest.NewRequestWithContext(
		ctx, http.MethodPost, rest.APIBasePath+"/items:query", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)
	return response.Code, response.Body.Bytes()
}

// PageInfo requires both of its fields, so the last page carries an explicit null rather than no
// field: a client reads next_cursor unconditionally, and an omitted one would make "no more pages"
// indistinguishable from "this server does not page".
func TestTheLastPageCarriesAnExplicitNullCursor(t *testing.T) {
	_, body := readResponse(t, "/containers", catalogue.Output{
		"data": []catalogue.Output{},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	})

	page, ok := decode(t, body)["page"].(map[string]any)
	if !ok {
		t.Fatalf("the body carries no page: %s", body)
	}
	value, present := page["next_cursor"]
	if !present {
		t.Error("next_cursor is absent rather than null")
	}
	if value != nil {
		t.Errorf("next_cursor is %v, want null", value)
	}
}

// bucketProjection is a column as a use case returns it: the field names of the contract, with the
// optional values as explicit nulls (B-09).
func bucketProjection() catalogue.Output {
	return catalogue.Output{
		"id":             "0192f000-0000-7000-8000-0000000000b1",
		"collection_id":  "0192f000-0000-7000-8000-00000000000b",
		"name":           "Doing",
		"order_key":      "a1",
		"wip_limit":      nil,
		"is_done_bucket": false,
		"color_token":    nil,
		"version":        1,
	}
}

// A board is a plain array rather than a page, so it is judged against the item schema row by row:
// the contract declares `type: array` with `items: Bucket`, and the validator judges a named schema.
func TestABoardMatchesTheBucketSchema(t *testing.T) {
	spec := contractSpec(t)
	collection := "0192f000-0000-7000-8000-00000000000b"

	status, body := readResponse(t, "/containers/"+collection+"/buckets",
		catalogue.Output{"data": []catalogue.Output{bucketProjection()}})
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}

	var board []json.RawMessage
	if err := json.Unmarshal(body, &board); err != nil {
		t.Fatalf("the board is not an array: %v (%s)", err, body)
	}
	if len(board) != 1 {
		t.Fatalf("%d columns, want 1", len(board))
	}

	problems, err := spec.validateAgainst("Bucket", board[0])
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("Bucket: %s", problem)
	}
}

// The optional values a board renders are present as null rather than absent. A client that had to
// tell "no limit" from "this server does not know about limits" would have to fetch the column.
func TestAColumnCarriesItsOptionalValuesAsNull(t *testing.T) {
	collection := "0192f000-0000-7000-8000-00000000000b"

	_, body := readResponse(t, "/containers/"+collection+"/buckets",
		catalogue.Output{"data": []catalogue.Output{bucketProjection()}})

	var board []map[string]json.RawMessage
	if err := json.Unmarshal(body, &board); err != nil {
		t.Fatalf("the board is not an array: %v (%s)", err, body)
	}
	for _, field := range []string{"wip_limit", "color_token"} {
		raw, present := board[0][field]
		if !present {
			t.Errorf("%s is absent rather than null", field)
			continue
		}
		if string(raw) != "null" {
			t.Errorf("%s is %s, want null", field, raw)
		}
	}
}

// labelProjection is a label as a use case returns it: the field names of the contract, with the
// description as an explicit null (B-09).
func labelProjection() catalogue.Output {
	return catalogue.Output{
		"id":            "0192f000-0000-7000-8000-0000000000c1",
		"collection_id": "0192f000-0000-7000-8000-00000000000b",
		"name":          "Urgent",
		"color_token":   "accent.red",
		"description":   nil,
		"version":       1,
	}
}

// A vocabulary is a plain array, so it is judged against the item schema row by row.
func TestAVocabularyMatchesTheLabelSchema(t *testing.T) {
	spec := contractSpec(t)
	collection := "0192f000-0000-7000-8000-00000000000b"

	status, body := readResponse(t, "/containers/"+collection+"/labels",
		catalogue.Output{"data": []catalogue.Output{labelProjection()}})
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}

	var vocabulary []json.RawMessage
	if err := json.Unmarshal(body, &vocabulary); err != nil {
		t.Fatalf("the vocabulary is not an array: %v (%s)", err, body)
	}
	if len(vocabulary) != 1 {
		t.Fatalf("%d labels, want 1", len(vocabulary))
	}

	problems, err := spec.validateAgainst("Label", vocabulary[0])
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("Label: %s", problem)
	}
}

// The labels an entry carries, as adding and removing report them (B-09).
func TestTheItemLabelResponseMatchesTheSchema(t *testing.T) {
	spec := contractSpec(t)
	itemID := "0192f000-0000-7000-8000-00000000000e"

	controller := rest.NewRestController()
	controller.UseCases = fixedCatalogue{out: catalogue.Output{
		"item_id":   itemID,
		"label_ids": []string{"0192f000-0000-7000-8000-0000000000c1"},
	}}

	ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, httptest.NewRequestWithContext(ctx, http.MethodPut,
		rest.APIBasePath+"/items/"+itemID+"/labels/0192f000-0000-7000-8000-0000000000c1", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	problems, err := spec.validateAgainst("ItemLabels", response.Body.Bytes())
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("ItemLabels: %s", problem)
	}
}
