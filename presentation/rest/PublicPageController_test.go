// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/knowledge"
	usecasemedia "github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	pages "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

type publicReader struct{ asked int }

var publicPicture = shared.MustParseID("0192f000-0000-7000-8000-000000000501")

func (p *publicReader) Picture(_ context.Context, token integration.PageLinkToken, id shared.ID) (shared.ID, error) {
	if id != publicPicture {
		return "", shared.ErrNotFound.WithDetail("pages.link_not_found")
	}
	return token.TenantID(), nil
}

type pictureBytes struct{ grant usecasemedia.Grant }

func (p *pictureBytes) Send(_ context.Context, grant usecasemedia.Grant) (usecasemedia.Served, error) {
	p.grant = grant
	return usecasemedia.Served{Content: storage.Object{Content: io.NopCloser(strings.NewReader("PNG")), Size: 3, ContentType: "image/png"}}, nil
}

func (p *publicReader) Execute(_ context.Context, _ integration.PageLinkToken) (knowledge.PublicPage, error) {
	p.asked++
	return knowledge.PublicPage{Page: pages.Page{
		ID: shared.MustParseID("0192f000-0000-7000-8000-000000000401"), ParentID: shared.MustParseID("0192f000-0000-7000-8000-000000000402"),
		Title: "Opening <hours>", Body: "We open at **nine**.\n\n![](hubtask:media/" + publicPicture.String() + ")\n",
	}, Pictures: []shared.ID{publicPicture}}, nil
}

// UC-DOC-08 check 4: the public route answers the page as an inert HTML document without a bearer,
// naming no page around it; a malformed token is not even asked about.
func TestAPublicLinkAnswersAnInertDocument(t *testing.T) {
	reader := &publicReader{}
	controller := NewRestController()
	controller.Feeds = &FeedDoor{Pages: reader}
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

// UC-DOC-08 check 5: the public page draws its picture from the link's own route, the policy opens
// pictures to that route alone, and the route streams the picture sandboxed - another object is
// the link's 404.
func TestAPublicPageDrawsItsPicturesThroughTheLink(t *testing.T) {
	reader := &publicReader{}
	sender := &pictureBytes{}
	controller := NewRestController()
	controller.Feeds = &FeedDoor{Pages: reader, Pictures: sender, Links: &movingLinks{base: "https://tasks.example.org"}}
	tenant := shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	token, err := integration.NewPageLinkToken(tenant, bytes.Repeat([]byte{3}, integration.PageLinkTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+path, nil))
		return recorder
	}
	page := get("/public/pages/" + token.Secret())
	if !strings.Contains(page.Body.String(), `<img src="./`+token.Secret()+`/media/`+publicPicture.String()+`" alt="">`) {
		t.Fatalf("no picture drawn: %s", page.Body)
	}
	if strings.Contains(page.Body.String(), "hubtask:") {
		t.Fatalf("an internal address reached the public page: %s", page.Body)
	}
	if policy := page.Header().Get("Content-Security-Policy"); !strings.Contains(policy,
		"img-src data: https://tasks.example.org"+APIBasePath+"/public/pages/"+token.Secret()+"/media/;") {
		t.Fatalf("policy %q", policy)
	}
	picture := get("/public/pages/" + token.Secret() + "/media/" + publicPicture.String())
	if picture.Code != http.StatusOK || picture.Body.String() != "PNG" || sender.grant.TenantID != tenant {
		t.Fatalf("picture %d %q %+v", picture.Code, picture.Body, sender.grant)
	}
	if picture.Header().Get("Content-Type") != "image/png" || !strings.HasPrefix(picture.Header().Get("Content-Security-Policy"), "sandbox") ||
		picture.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers %v", picture.Header())
	}
	if other := get("/public/pages/" + token.Secret() + "/media/0192f000-0000-7000-8000-000000000502"); other.Code != http.StatusNotFound {
		t.Fatalf("another object answered %d", other.Code)
	}
}
