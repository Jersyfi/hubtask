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
