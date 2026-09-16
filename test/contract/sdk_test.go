// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/rest"
	"github.com/Jersyfi/hubtask/sdk/go/hubtask"
)

// The Go SDK against the server it is generated for (P-02). Not a test of the generator - that is
// oapi-codegen's - but of the two halves meeting: the client's path, method and body reach the
// router the same specification generated, and the answer the router writes decodes into the
// type the client expects. A drift between the client's generation configuration and the
// server's would show here first.
func sdkServer(t *testing.T, out catalogue.Output) *httptest.Server {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = fixedCatalogue{out: out}
	routes := controller.Routes()
	actor := appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	}
	// The routes are served without the authentication middleware here, as every contract test
	// serves them; the actor the middleware would derive from the bearer is put on the context.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Errorf("the SDK sent no bearer to %s", r.URL.Path)
		}
		routes.ServeHTTP(w, r.WithContext(appshared.ContextWithActor(r.Context(), actor)))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestTheGoSdkReadsAPageAndCreatesAnEntry(t *testing.T) {
	page := catalogue.Output{
		"data": []catalogue.Output{containerProjection()},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}
	server := sdkServer(t, page)
	client, err := hubtask.NewClientWithResponses(server.URL+rest.APIBasePath,
		hubtask.WithRequestEditorFn(hubtask.WithBearer("hbt_pat_test")))
	if err != nil {
		t.Fatal(err)
	}

	containers, err := client.ListContainersWithResponse(context.Background(), &hubtask.ListContainersParams{})
	if err := hubtask.Check(err, containers); err != nil {
		t.Fatalf("list: %v", err)
	}
	if containers.JSON200 == nil || len(containers.JSON200.Data) != 1 || containers.JSON200.Data[0].Name != "Private" {
		t.Fatalf("the page did not decode: %+v", containers.JSON200)
	}
	if containers.JSON200.Page.HasMore {
		t.Error("the last page says it has more")
	}
}

func TestTheGoSdkCreatesAndReadsARefusal(t *testing.T) {
	server := sdkServer(t, itemProjection())
	client, err := hubtask.NewClientWithResponses(server.URL+rest.APIBasePath,
		hubtask.WithRequestEditorFn(hubtask.WithBearer("hbt_pat_test")))
	if err != nil {
		t.Fatal(err)
	}

	key := uuid.MustParse("0192f000-0000-7000-8000-000000000ff0")
	created, err := client.CreateWorkItemWithResponse(context.Background(),
		&hubtask.CreateWorkItemParams{IdempotencyKey: &key},
		hubtask.CreateWorkItemJSONRequestBody{Title: "Buy oat milk", Type: "TASK"})
	if err := hubtask.Check(err, created); err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.JSON201 == nil || created.JSON201.Title == "" {
		t.Fatalf("the entry did not decode: %+v", created)
	}

	// A refusal the use case answers: the problem document decodes into the SDK's error, with
	// the field the contract names and the stable code.
	refusing := sdkServer(t, nil)
	refusing.Config.Handler = refuse(t, shared.Validation("/title", "items.title_required", nil))
	refusingClient, err := hubtask.NewClientWithResponses(refusing.URL+rest.APIBasePath,
		hubtask.WithRequestEditorFn(hubtask.WithBearer("hbt_pat_test")))
	if err != nil {
		t.Fatal(err)
	}
	refused, err := refusingClient.CreateWorkItemWithResponse(context.Background(),
		&hubtask.CreateWorkItemParams{}, hubtask.CreateWorkItemJSONRequestBody{Title: "", Type: "TASK"})
	var problem *hubtask.ProblemError
	if checked := hubtask.Check(err, refused); !errors.As(checked, &problem) {
		t.Fatalf("an empty title should be refused: %v", checked)
	}
	if problem.Status != http.StatusUnprocessableEntity || problem.Problem.Code != "validation_failed" {
		t.Errorf("the refusal is not the problem document the contract declares: %+v", problem)
	}
	if problem.Problem.FieldErrors == nil || len(*problem.Problem.FieldErrors) != 1 || *(*problem.Problem.FieldErrors)[0].Path != "/title" {
		t.Errorf("the field error did not travel: %+v", problem.Problem)
	}
}

// refuse serves the routes with a catalogue that answers every invocation with one error.
func refuse(t *testing.T, err error) http.Handler {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = erroringCatalogue{err: err}
	routes := controller.Routes()
	actor := appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routes.ServeHTTP(w, r.WithContext(appshared.ContextWithActor(r.Context(), actor)))
	})
}

type erroringCatalogue struct{ err error }

func (c erroringCatalogue) Invoke(
	context.Context, string, appshared.ActorContext, catalogue.Input,
) (catalogue.Output, error) {
	return nil, c.err
}
