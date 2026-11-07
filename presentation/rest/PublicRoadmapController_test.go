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

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type roadmapReader struct {
	asked int
	zone  string
}

func (p *roadmapReader) Execute(_ context.Context, _ integration.RoadmapLinkToken, zone string) (usecase.Output, error) {
	p.asked++
	p.zone = zone
	return usecase.Output{"portfolio_id": "0192f000-0000-7000-8000-000000000501", "name": "Programme", "lanes": []usecase.Output{}}, nil
}

// UC-GOL-03 check 15: the public route answers the roadmap without a bearer, not cached and not
// indexed; a malformed token is not even asked about.
func TestARoadmapLinkAnswersTheRoadmapWithoutABearer(t *testing.T) {
	reader := &roadmapReader{}
	controller := NewRestController()
	controller.Feeds = &FeedDoor{Roadmaps: reader}
	token, err := integration.NewRoadmapLinkToken(shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), bytes.Repeat([]byte{3}, integration.RoadmapLinkTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/public/roadmaps/"+path, nil))
		return recorder
	}
	answer := get(token.Secret() + "?zone=Europe/Berlin")
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"name":"Programme"`) || reader.zone != "Europe/Berlin" {
		t.Fatalf("status %d (zone %q): %s", answer.Code, reader.zone, answer.Body)
	}
	if answer.Header().Get("Cache-Control") != "no-store" || answer.Header().Get("X-Robots-Tag") == "" {
		t.Fatalf("headers %v", answer.Header())
	}
	if get("hbt_page_nonsense").Code != http.StatusNotFound || reader.asked != 1 {
		t.Fatalf("a malformed token was asked about (%d)", reader.asked)
	}
	if !PublicRoutes[http.MethodGet+" "+APIBasePath+"/public/roadmaps/{token}"] {
		t.Fatal("the route asks for a bearer")
	}
}
