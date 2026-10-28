// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// movingLinks is the workspace's main address, which a change of address moves between two
// requests; it notes whose workspace each request asked about.
type movingLinks struct {
	base  string
	asked []shared.ID
}

func (l *movingLinks) For(_ context.Context, tenant shared.ID) (string, error) {
	l.asked = append(l.asked, tenant)
	return l.base, nil
}

var linksTenant = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")

// changedBetween runs request once on the old main address and once after the change, and answers
// both bodies.
func changedBetween(t *testing.T, links *movingLinks, request func() *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	links.base = "https://anna.hub.example"
	before := request()
	links.base = "https://tasks.anna.example"
	after := request()
	for _, r := range []*httptest.ResponseRecorder{before, after} {
		if r.Code >= 300 {
			t.Fatalf("status %d: %s", r.Code, r.Body)
		}
	}
	for _, tenant := range links.asked {
		if tenant != linksTenant {
			t.Errorf("asked for workspace %s, want the request's own", tenant)
		}
	}
	if len(links.asked) == 0 {
		t.Error("the link was not read from the workspace's address")
	}
	return before.Body.String(), after.Body.String()
}

func asMember(r *http.Request) *http.Request {
	return r.WithContext(appshared.ContextWithActor(r.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: linksTenant,
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"), TimeZone: "Europe/Berlin",
	}))
}

// UC-INS-19 check 9: the address a calendar client is handed is the workspace's main row as it
// stands - after a change of address, the new one.
func TestACalendarFeedsAddressFollowsTheMainRow(t *testing.T) {
	links := &movingLinks{}
	controller := NewRestController()
	controller.Links = links
	controller.UseCases = &catalogue{out: usecase.Output{
		"id": "0192f000-0000-7000-8000-0000000000f1", "account_id": "0192f000-0000-7000-8000-00000000000d",
		"view_id": exportViewID, "token": "the-token",
	}}
	urlOf := func(body string) string {
		var feed openapi.CalendarFeedSecret
		if err := json.Unmarshal([]byte(body), &feed); err != nil {
			t.Fatalf("not a feed: %v", err)
		}
		return feed.Url
	}
	before, after := changedBetween(t, links, func() *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			APIBasePath+"/integrations/calendar-feeds", strings.NewReader(`{"view_id":"`+exportViewID+`"}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, asMember(request))
		return recorder
	})
	if got := urlOf(before); got != "https://anna.hub.example"+APIBasePath+"/calendar/the-token.ics" {
		t.Errorf("before the change the feed is at %q", got)
	}
	if got := urlOf(after); got != "https://tasks.anna.example"+APIBasePath+"/calendar/the-token.ics" {
		t.Errorf("after the change the feed is at %q, want the new main row", got)
	}
}

// UC-INS-19 check 9: an entry's link in a subscribed feed names the feed's own workspace's main
// row, which the token names - after a change of address, the new one.
func TestAFeedsEntryLinksFollowTheMainRow(t *testing.T) {
	links := &movingLinks{}
	controller := NewRestController()
	controller.Links = links
	controller.CalendarFeeds = &feedReader{out: feedFixture()}
	token := wellFormedToken(t)
	before, after := changedBetween(t, links, func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(),
			http.MethodGet, APIBasePath+"/calendar/"+token+".ics", nil))
		return recorder
	})
	assertMoved(t, before, after)
}

// UC-INS-19 check 9: an export's entry links - of a view and of a query - name the workspace's
// main row as it stands.
func TestAnExportsItemLinksFollowTheMainRow(t *testing.T) {
	for name, path := range map[string]string{
		"a view":  APIBasePath + "/views/" + exportViewID + ":export",
		"a query": APIBasePath + "/items:export",
	} {
		t.Run(name, func(t *testing.T) {
			links := &movingLinks{}
			controller := NewRestController()
			controller.Links = links
			controller.UseCases = &catalogue{out: exportedRows()}
			before, after := changedBetween(t, links, func() *httptest.ResponseRecorder {
				request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path,
					strings.NewReader(`{"format":"ICS"}`))
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				controller.Routes().ServeHTTP(recorder, asMember(request))
				return recorder
			})
			assertMoved(t, before, after)
		})
	}
}

func assertMoved(t *testing.T, before, after string) {
	t.Helper()
	if !strings.Contains(before, "URL:https://anna.hub.example/items/") {
		t.Errorf("before the change the entries link:\n%s", before)
	}
	if !strings.Contains(after, "URL:https://tasks.anna.example/items/") || strings.Contains(after, "anna.hub.example") {
		t.Errorf("after the change the entries link:\n%s", after)
	}
}
