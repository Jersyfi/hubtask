// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	boardCollection = "0192f000-0000-7000-8000-00000000000c"
	firstBucket     = "0192f000-0000-7000-8000-0000000000b1"
)

func storedBucket(name, orderKey string) usecase.Output {
	return usecase.Output{
		"id":             firstBucket,
		"collection_id":  boardCollection,
		"name":           name,
		"order_key":      orderKey,
		"wip_limit":      nil,
		"is_done_bucket": false,
		"color_token":    nil,
		"version":        1,
	}
}

func bucketRequest(
	t *testing.T, registry UseCaseRegistry, method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, reader)
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestCreatingABucketAnswers201WithTheColumn(t *testing.T) {
	registry := &catalogue{out: storedBucket("Doing", "a1")}

	recorder := bucketRequest(t, registry, http.MethodPost,
		"/containers/"+boardCollection+"/buckets", `{"name":"Doing","wip_limit":4}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status %d, want 201: %s", recorder.Code, recorder.Body)
	}
	if registry.name != createBucketUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	// The collection comes from the path rather than from the body: the route already says which
	// board this is, and a body that could disagree with it would be a second answer.
	if registry.in["collection_id"] != boardCollection {
		t.Errorf("collection_id is %v, want the one in the path", registry.in["collection_id"])
	}
	if registry.in["wip_limit"] != 4 {
		t.Errorf("wip_limit is %v", registry.in["wip_limit"])
	}

	want := APIBasePath + "/containers/" + boardCollection + "/buckets/" + firstBucket
	if location := recorder.Header().Get("Location"); location != want {
		t.Errorf("Location is %q, want %q", location, want)
	}
	// Strong rather than weak: If-Match compares strongly, and optimistic locking is built on it.
	if tag := recorder.Header().Get("ETag"); tag != `"1"` {
		t.Errorf("ETag is %q", tag)
	}
}

// A field the contract does not declare is refused rather than ignored: a client that misspells
// `wip_limit` and receives a 201 has created a column it cannot see is wrong.
func TestCreatingABucketRefusesAnUnknownField(t *testing.T) {
	registry := &catalogue{out: storedBucket("Doing", "a1")}

	recorder := bucketRequest(t, registry, http.MethodPost,
		"/containers/"+boardCollection+"/buckets", `{"name":"Doing","wip_limitt":4}`)

	if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want a refusal: %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Error("the use case ran on a body the contract does not describe")
	}
}

// A plain array rather than a page, as the contract declares: a board has as many columns as fit
// on a screen.
func TestListingABoardAnswersAnArray(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"data": []usecase.Output{
		storedBucket("Todo", "a0"),
	}}}

	recorder := bucketRequest(t, registry, http.MethodGet,
		"/containers/"+boardCollection+"/buckets", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != listBucketsUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}

	var board []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &board); err != nil {
		t.Fatalf("the body is not an array: %v (%s)", err, recorder.Body)
	}
	if len(board) != 1 || board[0]["name"] != "Todo" {
		t.Fatalf("the board is %+v", board)
	}
	// The optional values are explicit nulls rather than absent: a board renders them, and a field
	// that appeared only once somebody had set it is one a client cannot read unconditionally.
	if _, present := board[0]["wip_limit"]; !present {
		t.Error("wip_limit is missing rather than null")
	}
}

// An empty board is an empty array, not null: a client that iterates the answer should not have to
// nil-check it.
func TestAnEmptyBoardIsAnEmptyArray(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"data": []usecase.Output{}}}

	recorder := bucketRequest(t, registry, http.MethodGet,
		"/containers/"+boardCollection+"/buckets", "")

	if body := strings.TrimSpace(recorder.Body.String()); body != "[]" {
		t.Errorf("the body is %q, want an empty array", body)
	}
}

// The handler holds no rules, so a refusal from the catalogue reaches the client unchanged.
func TestABucketRefusalIsReportedAsAProblem(t *testing.T) {
	registry := &catalogue{err: shared.ErrConflict.WithDetail("buckets.name_taken")}

	recorder := bucketRequest(t, registry, http.MethodPost,
		"/containers/"+boardCollection+"/buckets", `{"name":"Doing"}`)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", recorder.Code, recorder.Body)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(
		contentType, "application/problem+json") {
		t.Errorf("content type %q", contentType)
	}
}

// A merge patch says "leave it alone" by omission, and the handler passes on only what arrived: a
// handler that sent every field would clear the colour of every client that only meant to rename
// something.
func TestUpdatingABucketPassesOnOnlyWhatArrived(t *testing.T) {
	registry := &catalogue{out: storedBucket("In progress", "a1")}

	recorder := bucketRequest(t, registry, http.MethodPatch,
		"/containers/"+boardCollection+"/buckets/"+firstBucket, `{"name":"In progress"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != updateBucketUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if _, sent := registry.in["color_token"]; sent {
		t.Error("a field the client did not send reached the use case")
	}
	if registry.in["bucket_id"] != firstBucket {
		t.Errorf("bucket_id is %v", registry.in["bucket_id"])
	}
	if tag := recorder.Header().Get("ETag"); tag != `"1"` {
		t.Errorf("ETag is %q", tag)
	}
}

// Null and 0 are the same instruction - remove the limit - because zero is not a limit anybody
// could satisfy. Reading them as one saves the layers below a second flag beside the number.
func TestNullAndZeroBothRemoveTheLimit(t *testing.T) {
	for _, body := range []string{`{"wip_limit":null}`, `{"wip_limit":0}`} {
		t.Run(body, func(t *testing.T) {
			registry := &catalogue{out: storedBucket("Doing", "a1")}

			recorder := bucketRequest(t, registry, http.MethodPatch,
				"/containers/"+boardCollection+"/buckets/"+firstBucket, body)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
			}
			if registry.in["wip_limit"] != 0 {
				t.Errorf("wip_limit reached the use case as %v, want 0", registry.in["wip_limit"])
			}
		})
	}
}

// The If-Match is what optimistic locking is built on, and it has to reach the use case.
func TestTheIfMatchOfABucketUpdateReachesTheUseCase(t *testing.T) {
	registry := &catalogue{out: storedBucket("Doing", "a1")}

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPatch,
		APIBasePath+"/containers/"+boardCollection+"/buckets/"+firstBucket,
		strings.NewReader(`{"name":"In progress"}`))
	request.Header.Set("Content-Type", "application/merge-patch+json")
	request.Header.Set("If-Match", `"3"`)

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.in["expected_version"] != 3 {
		t.Errorf("expected_version is %v", registry.in["expected_version"])
	}
}

// No body at all is a position like any other: the right hand end.
func TestReorderingWithNoBodyMeansTheEnd(t *testing.T) {
	registry := &catalogue{out: storedBucket("Doing", "a9")}

	recorder := bucketRequest(t, registry, http.MethodPost,
		"/containers/"+boardCollection+"/buckets/"+firstBucket+":reorder", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != reorderBucketUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if _, sent := registry.in["before_bucket_id"]; sent {
		t.Error("an anchor nobody sent reached the use case")
	}
}

func TestReorderingPassesOnTheAnchor(t *testing.T) {
	registry := &catalogue{out: storedBucket("Doing", "a0")}
	anchor := "0192f000-0000-7000-8000-0000000000b2"

	recorder := bucketRequest(t, registry, http.MethodPost,
		"/containers/"+boardCollection+"/buckets/"+firstBucket+":reorder",
		`{"before_bucket_id":"`+anchor+`"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if registry.in["before_bucket_id"] != anchor {
		t.Errorf("before_bucket_id is %v", registry.in["before_bucket_id"])
	}
}

// 200 with a body rather than 204, unlike a container's deletion: what became of the entries that
// were in the column is not derivable from the request, and a client that received no content would
// have to reload the whole board to find out where its cards are.
func TestDeletingABucketAnswersWithWhatBecameOfTheEntries(t *testing.T) {
	target := "0192f000-0000-7000-8000-0000000000b2"
	registry := &catalogue{out: usecase.Output{
		"bucket_id": firstBucket, "target_bucket_id": target, "moved_items": 3,
	}}

	recorder := bucketRequest(t, registry, http.MethodDelete,
		"/containers/"+boardCollection+"/buckets/"+firstBucket, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != deleteBucketUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}

	var deletion map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &deletion); err != nil {
		t.Fatalf("the body is not an object: %v (%s)", err, recorder.Body)
	}
	if deletion["target_bucket_id"] != target {
		t.Errorf("target_bucket_id is %v", deletion["target_bucket_id"])
	}
	if deletion["moved_items"] != float64(3) {
		t.Errorf("moved_items is %v", deletion["moved_items"])
	}
}

// The last column of a board has nowhere to send its entries, and the answer says so with a null
// rather than by leaving the field out.
func TestDeletingTheLastBucketAnswersWithANullTarget(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"bucket_id": firstBucket, "target_bucket_id": nil, "moved_items": 0,
	}}

	recorder := bucketRequest(t, registry, http.MethodDelete,
		"/containers/"+boardCollection+"/buckets/"+firstBucket, "")

	var deletion map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &deletion); err != nil {
		t.Fatalf("the body is not an object: %v (%s)", err, recorder.Body)
	}
	raw, present := deletion["target_bucket_id"]
	if !present || string(raw) != "null" {
		t.Errorf("target_bucket_id is %s, want null", raw)
	}
}

// Null and the empty string are one instruction for the colour: clear it. That is what makes them
// a different request from omitting the field, which leaves the colour alone.
func TestANullColourClearsAColumnsColour(t *testing.T) {
	registry := &catalogue{out: storedBucket("Doing", "a1")}

	recorder := bucketRequest(t, registry, http.MethodPatch,
		"/containers/"+boardCollection+"/buckets/"+firstBucket, `{"color_token":null}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	value, sent := registry.in["color_token"]
	if !sent || value != "" {
		t.Errorf("color_token reached the use case as %v, want the empty string", value)
	}
}
