// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	sampleContainerID = "0192f000-0000-7000-8000-00000000000b"
	sampleItemID      = "0192f000-0000-7000-8000-00000000000e"
)

// get issues a GET through the router, so that the path parameters are bound the way production binds
// them rather than passed to a handler by hand.
func get(t *testing.T, registry UseCaseRegistry, path string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder,
		httptest.NewRequestWithContext(ctx, http.MethodGet, APIBasePath+path, nil))
	return recorder
}

func readItem() usecase.Output {
	at := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	return usecase.Output{
		"id":            sampleItemID,
		"type":          "TASK",
		"collection_id": sampleContainerID,
		"parent_id":     nil,
		"path":          "/" + sampleItemID + "/",
		"depth":         1,
		"title":         "Buy milk",
		"completion": map[string]any{
			"is_completed": false, "completed_at": nil, "completed_by": nil,
		},
		"order_key":   "a0",
		"archived_at": nil,
		"deleted_at":  nil,
		"created_by":  "0192f000-0000-7000-8000-00000000000d",
		"created_at":  at,
		"updated_at":  at,
		"version":     3,
	}
}

func pageOf(rows []usecase.Output, cursor any, hasMore bool) usecase.Output {
	return usecase.Output{
		"data": rows,
		"page": map[string]any{"next_cursor": cursor, "has_more": hasMore},
	}
}

// The ETag is the whole point of a single-object read beyond the body: it is the version the client
// sends back as If-Match, and without it there is no optimistic locking to have
// (api-guidelines.md §5).
func TestASingleReadCarriesTheVersionAsAnETag(t *testing.T) {
	cases := map[string]struct {
		path string
		out  usecase.Output
	}{
		"a container": {"/containers/" + sampleContainerID, createdContainer()},
		"an item":     {"/items/" + sampleItemID, readItem()},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			out := c.out
			out["version"] = 7

			response := get(t, &catalogue{out: out}, c.path)
			if response.Code != http.StatusOK {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}
			// Strong and quoted, because If-Match requires strong comparison (RFC 9110 §13.1.1).
			if got := response.Header().Get("ETag"); got != `"7"` {
				t.Errorf("ETag %q, want \"7\"", got)
			}
		})
	}
}

// A page is not an entity with a version, so it carries no entity tag: there would be nothing for a
// client to write back against, and a tag over a list would change whenever any row in it did.
func TestAPageCarriesNoETag(t *testing.T) {
	for name, path := range map[string]string{
		"containers": "/containers",
		"items":      "/items?collection_id=" + sampleContainerID,
	} {
		t.Run(name, func(t *testing.T) {
			response := get(t, &catalogue{out: pageOf(nil, nil, false)}, path)
			if response.Code != http.StatusOK {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}
			if tag := response.Header().Get("ETag"); tag != "" {
				t.Errorf("a page answered with the ETag %q", tag)
			}
		})
	}
}

func TestReadingAContainerCallsTheCatalogueWithTheIdentifier(t *testing.T) {
	registry := &catalogue{out: createdContainer()}

	if response := get(t, registry, "/containers/"+sampleContainerID); response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if registry.name != getContainerUseCase {
		t.Errorf("the handler ran %q", registry.name)
	}
	if registry.in["container_id"] != sampleContainerID {
		t.Errorf("the catalogue was asked for %v", registry.in["container_id"])
	}
}

// The query parameters reach the catalogue as the use case's own field names, and an absent one is
// absent rather than a zero: every channel then sees the same input, and the use case's default
// applies instead of the adapter's.
func TestTheListParametersReachTheCatalogue(t *testing.T) {
	t.Run("everything given", func(t *testing.T) {
		registry := &catalogue{out: pageOf(nil, nil, false)}
		response := get(t, registry, "/containers?parent_id="+sampleContainerID+
			"&type=COLLECTION&include_archived=true&cursor=opaque&size=25")
		if response.Code != http.StatusOK {
			t.Fatalf("status %d: %s", response.Code, response.Body)
		}

		want := map[string]any{
			"parent_id": sampleContainerID, "type": "COLLECTION",
			"include_archived": true, "cursor": "opaque", "size": 25,
		}
		for field, value := range want {
			if registry.in[field] != value {
				t.Errorf("%s reached the catalogue as %v (%T), want %v", field, registry.in[field], registry.in[field], value)
			}
		}
	})

	t.Run("nothing given", func(t *testing.T) {
		registry := &catalogue{out: pageOf(nil, nil, false)}
		if response := get(t, registry, "/containers"); response.Code != http.StatusOK {
			t.Fatalf("status %d: %s", response.Code, response.Body)
		}

		for _, field := range []string{"parent_id", "include_archived", "cursor", "size"} {
			if registry.in[field] != nil {
				t.Errorf("an absent %s reached the catalogue as %v", field, registry.in[field])
			}
		}
		// Absent entirely rather than an empty string: the catalogue refuses a value outside the enum,
		// and "" is outside it.
		if _, present := registry.in["type"]; present {
			t.Error("an absent type reached the catalogue as an entry")
		}
	})
}

// collection_id is required in the contract, so the generated binding refuses a request without it
// before any handler runs. Asserted here because that refusal is the endpoint's contract, not an
// implementation detail: an unanchored list would be an unindexed scan.
func TestListingItemsWithoutACollectionIsRefused(t *testing.T) {
	registry := &catalogue{out: pageOf(nil, nil, false)}

	response := get(t, registry, "/items")
	if response.Code == http.StatusOK {
		t.Fatalf("a list with no collection answered 200: %s", response.Body)
	}
	if registry.invoked {
		t.Error("the catalogue was called without a collection")
	}
}

// The page shape of api-guidelines.md §4, read off the wire rather than off the generated struct: what
// a client sees is the JSON.
func TestThePageBodyCarriesTheDataAndTheWalkState(t *testing.T) {
	cases := map[string]struct {
		path       string
		rows       []usecase.Output
		cursor     any
		hasMore    bool
		wantCursor any
	}{
		"containers, with a successor": {
			"/containers", []usecase.Output{createdContainer()}, "opaque", true, "opaque",
		},
		"items, on the last page": {
			"/items?collection_id=" + sampleContainerID, []usecase.Output{readItem()}, nil, false, nil,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			response := get(t, &catalogue{out: pageOf(c.rows, c.cursor, c.hasMore)}, c.path)
			if response.Code != http.StatusOK {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}

			var body struct {
				Data []map[string]any `json:"data"`
				Page struct {
					NextCursor *string `json:"next_cursor"`
					HasMore    bool    `json:"has_more"`
				} `json:"page"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("the body is not a page: %v", err)
			}

			if len(body.Data) != 1 {
				t.Fatalf("the page carries %d rows, want 1", len(body.Data))
			}
			if body.Page.HasMore != c.hasMore {
				t.Errorf("has_more is %v, want %v", body.Page.HasMore, c.hasMore)
			}
			switch want := c.wantCursor.(type) {
			case nil:
				if body.Page.NextCursor != nil {
					t.Errorf("the last page carries the cursor %q", *body.Page.NextCursor)
				}
			case string:
				if body.Page.NextCursor == nil || *body.Page.NextCursor != want {
					t.Errorf("next_cursor is %v, want %q", body.Page.NextCursor, want)
				}
			}
		})
	}
}

// An empty page is `"data": []`, never `"data": null`. A client that iterates the field would have to
// nil-check it otherwise, and half of them will not.
func TestAnEmptyPageCarriesAnEmptyArray(t *testing.T) {
	response := get(t, &catalogue{out: pageOf(nil, nil, false)}, "/containers")

	var body struct {
		Data *[]map[string]any `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("the body is not a page: %v", err)
	}
	if body.Data == nil {
		t.Fatal("data is null rather than an empty array")
	}
	if len(*body.Data) != 0 {
		t.Errorf("an empty page carries %d rows", len(*body.Data))
	}
}

// `expand` is in the contract and has no implementation yet. Refused rather than ignored: a client that
// asked for children and got an item without them cannot tell that from an item that has none.
func TestAnUnsupportedExpandIsRefusedRatherThanIgnored(t *testing.T) {
	registry := &catalogue{out: readItem()}

	response := get(t, registry, "/items/"+sampleItemID+"?expand=children:1")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", response.Code, response.Body)
	}
	if registry.invoked {
		t.Error("the catalogue was called for a request that could not be answered as asked")
	}

	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.DetailCode != "items.expand_not_supported" {
		t.Errorf("detail code %q", problem.DetailCode)
	}
}

// An empty expand is not a request for anything, so it is not refused - a client that always sends the
// parameter and leaves it empty is asking for the plain item.
func TestAnEmptyExpandIsNotRefused(t *testing.T) {
	if response := get(t, &catalogue{out: readItem()}, "/items/"+sampleItemID+"?expand="); response.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", response.Code, response.Body)
	}
}

// A refusal from the application layer reaches the client as the problem document it is, with nothing
// of the read in the body.
func TestARefusedReadAnswersTheProblem(t *testing.T) {
	registry := &catalogue{err: shared.ErrForbidden.WithDetail("access.not_permitted")}

	response := get(t, registry, "/containers/"+sampleContainerID)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403: %s", response.Code, response.Body)
	}
	if tag := response.Header().Get("ETag"); tag != "" {
		t.Errorf("a refused read answered with the ETag %q", tag)
	}
}

// A malformed identifier in the path is refused by the generated binding, before the catalogue.
func TestAMalformedIdentifierInThePathIsRefused(t *testing.T) {
	registry := &catalogue{out: createdContainer()}

	if response := get(t, registry, "/containers/not-a-uuid"); response.Code == http.StatusOK {
		t.Fatalf("a malformed identifier answered 200: %s", response.Body)
	}
	if registry.invoked {
		t.Error("the catalogue was called with a malformed identifier")
	}
}

// Every read goes through the catalogue, so an unwired controller is a defect in the composition root
// and answers as one rather than panicking.
func TestAnUnwiredReadAnswers500(t *testing.T) {
	controller := NewRestController()

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, APIBasePath+"/containers", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: %s", recorder.Code, recorder.Body)
	}
}
