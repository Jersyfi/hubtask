// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type satisfactionAnswers struct{ scores []int }

func (a *satisfactionAnswers) Execute(_ context.Context, _ integration.SatisfactionLinkToken, score int) error {
	if score == 0 {
		return shared.ErrConflict.WithDetail("satisfaction.link_used")
	}
	a.scores = append(a.scores, score)
	return nil
}

// UC-SVC-07 check 6: a satisfaction link is answered without a bearer with 204; a used link is a
// conflict, a malformed token is not even asked about, and the answers count per client address.
func TestASatisfactionLinkIsAnsweredWithoutABearer(t *testing.T) {
	answers := &satisfactionAnswers{}
	controller := NewRestController()
	controller.Feeds = &FeedDoor{Satisfaction: answers}
	token, err := integration.NewSatisfactionLinkToken(shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), bytes.Repeat([]byte{3}, integration.SatisfactionLinkTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	serve := func(presented, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/public/satisfaction/"+presented, strings.NewReader(body))
		controller.Routes().ServeHTTP(recorder, request)
		return recorder
	}
	if got := serve(token.Secret(), `{"score":4}`); got.Code != http.StatusNoContent || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d: %s", got.Code, got.Body)
	}
	if got := serve(token.Secret(), `{"score":0}`); got.Code != http.StatusConflict {
		t.Errorf("a used link answered %d", got.Code)
	}
	if got := serve(token.Secret(), `{"score":3,"comment":"x"}`); got.Code != http.StatusUnprocessableEntity {
		t.Errorf("an unknown member answered %d", got.Code)
	}
	if got := serve("hbt_rate_nonsense", `{"score":4}`); got.Code != http.StatusNotFound || len(answers.scores) != 1 {
		t.Fatalf("a malformed token answered %d and was asked about: %v", got.Code, answers.scores)
	}
	if !PublicRoutes[http.MethodPost+" "+APIBasePath+"/public/satisfaction/{token}"] {
		t.Error("the route asks for a bearer")
	}
	post := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/public/satisfaction/hbt_rate_x", nil)
	post.RemoteAddr = "203.0.113.7:4711"
	if got := PublicFormBucket(5, 2)(post); got.Key != "203.0.113.7" || got.Limit != 5 {
		t.Errorf("an answer is not counted per address: %+v", got)
	}
}
