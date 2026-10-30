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

	"github.com/Jersyfi/hubtask/core/application/service/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	pages "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type publicReader struct{ asked int }

func (p *publicReader) Execute(_ context.Context, _ integration.PageLinkToken) (knowledge.PublicPage, error) {
	p.asked++
	return knowledge.PublicPage{Page: pages.Page{
		ID: shared.MustParseID("0192f000-0000-7000-8000-000000000401"), ParentID: shared.MustParseID("0192f000-0000-7000-8000-000000000402"),
		Title: "Opening <hours>", Body: "We open at **nine**.",
	}}, nil
}

// UC-DOC-08 check 5: the public route answers the page as an inert HTML document without a bearer,
// naming no page around it; a malformed token is not even asked about.
func TestAPublicLinkAnswersAnInertDocument(t *testing.T) {
	reader := &publicReader{}
	controller := NewRestController()
	controller.PublicPages = reader
	token, err := integration.NewPageLinkToken(shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), bytes.Repeat([]byte{3}, integration.PageLinkTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	get := func(presented string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/public/pages/"+presented, nil))
		return recorder
	}
	answer := get(token.Secret())
	if answer.Code != http.StatusOK {
		t.Fatalf("status %d: %s", answer.Code, answer.Body)
	}
	body := answer.Body.String()
	if !strings.Contains(body, "Opening &lt;hours&gt;") || !strings.Contains(body, "<strong>nine</strong>") {
		t.Fatalf("body %s", body)
	}
	if strings.Contains(body, "000000000402") {
		t.Fatal("the document names the page above")
	}
	if policy := answer.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "default-src 'none'") {
		t.Fatalf("policy %q", policy)
	}
	if answer.Header().Get("Cache-Control") != "no-store" || answer.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("headers %v", answer.Header())
	}
	if get("hbt_page_nonsense").Code != http.StatusNotFound || reader.asked != 1 {
		t.Fatalf("a malformed token was asked about (%d)", reader.asked)
	}
}
