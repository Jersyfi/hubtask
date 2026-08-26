// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The public route (D-08). Two things are decided out here and are therefore tested out here: what
// a malformed token gets, and what the .ics answer carries as headers.

// feedReader is the application service's double.
type feedReader struct {
	out     work.ExportedView
	err     error
	asked   integration.FeedToken
	invoked bool
}

func (f *feedReader) Execute(
	_ context.Context, token integration.FeedToken,
) (work.ExportedView, error) {
	f.asked, f.invoked = token, true
	return f.out, f.err
}

func feedFixture() work.ExportedView {
	due := time.Date(2026, 10, 22, 14, 30, 0, 0, time.UTC)
	allDay := time.Date(2026, 10, 22, 22, 0, 0, 0, time.UTC)

	return work.ExportedView{
		View:        view.SavedView{Name: "This week"},
		GeneratedAt: time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC),
		TimeZone:    "Europe/Berlin",
		Items: []workmodel.WorkItem{
			{
				ID:    shared.MustParseID("0192f000-0000-7000-8000-000000000301"),
				Title: "Call the landlord",
				Due:   &workmodel.DueDate{At: due},
			},
			{
				ID:    shared.MustParseID("0192f000-0000-7000-8000-000000000302"),
				Title: "Hand back the keys",
				Due:   &workmodel.DueDate{At: allDay, DateOnly: true, TimeZone: "Europe/Berlin"},
			},
			{
				ID:    shared.MustParseID("0192f000-0000-7000-8000-000000000303"),
				Title: "Somewhere on the list",
			},
		},
	}
}

func fetchFeed(t *testing.T, reader CalendarFeedReader, token string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.CalendarFeeds = reader
	controller.BaseURL = "https://hubtask.example.com"

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/calendar/"+token+".ics", nil)
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func wellFormedToken(t *testing.T) string {
	t.Helper()
	entropy := make([]byte, integration.FeedTokenSecretBytes)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	token, err := integration.NewFeedToken(
		shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), entropy)
	if err != nil {
		t.Fatal(err)
	}
	return token.Secret()
}

func TestTheFeedAnswersACalendarDocument(t *testing.T) {
	reader := &feedReader{out: feedFixture()}

	recorder := fetchFeed(t, reader, wellFormedToken(t))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if kind := recorder.Header().Get("Content-Type"); !strings.HasPrefix(kind, "text/calendar") {
		t.Errorf("content type %q", kind)
	}
	// A calendar document, not a page: delivered as a file, and never held by a cache in
	// between - on this route the URL is itself the credential.
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.Contains(
		disposition, "attachment") {
		t.Errorf("content disposition %q", disposition)
	}
	if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
		t.Errorf("cache control %q", cache)
	}

	body := recorder.Body.String()
	if strings.Count(body, "BEGIN:VEVENT") != 2 {
		t.Errorf("the calendar has %d entries and three rows went in",
			strings.Count(body, "BEGIN:VEVENT"))
	}
	if !strings.Contains(body, "DTSTART:20261022T143000Z") ||
		!strings.Contains(body, "DTSTART;VALUE=DATE:20261023") {
		t.Errorf("the two dated entries came out as:\n%s", body)
	}
	if !strings.Contains(body, "X-WR-CALNAME:This week") {
		t.Error("the subscription carries no name")
	}
}

// A malformed token never reaches the lookup, and answers exactly what an unknown one answers.
func TestAMalformedTokenIsRefusedWithoutALookup(t *testing.T) {
	reader := &feedReader{out: feedFixture()}

	recorder := fetchFeed(t, reader, "not-a-token")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if reader.invoked {
		t.Error("a malformed token cost a lookup")
	}
	if !strings.Contains(recorder.Body.String(), "calendar.feed_not_found") {
		t.Errorf("the refusal is %s", recorder.Body)
	}
	// And the answer says nothing about the token itself.
	if strings.Contains(recorder.Body.String(), "not-a-token") {
		t.Error("the refusal quotes the credential back")
	}
}

// The application layer's refusal reaches the client unchanged, so a revoked feed and an unknown
// one are one answer from out here too.
func TestARefusedFeedAnswersTheSameAsAnUnknownOne(t *testing.T) {
	unknown := fetchFeed(t, &feedReader{
		err: shared.ErrNotFound.WithDetail("calendar.feed_not_found"),
	}, wellFormedToken(t))
	malformed := fetchFeed(t, &feedReader{}, "hbt_cal_short")

	if unknown.Code != malformed.Code {
		t.Fatalf("statuses %d and %d", unknown.Code, malformed.Code)
	}
	if unknownBody, malformedBody := problemWithoutRequestID(unknown.Body.String()),
		problemWithoutRequestID(malformed.Body.String()); unknownBody != malformedBody {
		t.Errorf("two bodies:\n%s\n%s", unknownBody, malformedBody)
	}
}

// problemWithoutRequestID drops the one field that legitimately differs between two answers.
func problemWithoutRequestID(body string) string {
	for _, line := range strings.Split(body, ",") {
		if strings.Contains(line, "request_id") {
			body = strings.Replace(body, line+",", "", 1)
			body = strings.Replace(body, ","+line, "", 1)
		}
	}
	return body
}

// The token in the URL decides the bucket, so one client polling hard sheds itself rather than
// somebody else's calendar behind the same address.
func TestTheFeedBucketIsPerTokenAndAppliesToNothingElse(t *testing.T) {
	bucket := FeedBucket(60, 10)
	token := wellFormedToken(t)

	feed := bucket(httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/calendar/"+token+".ics", nil))
	if feed.Key == "" || feed.Limit != 60 {
		t.Fatalf("the feed bucket is %+v", feed)
	}
	if strings.Contains(feed.Key, token) {
		t.Error("the bucket key is the credential itself, and this map ends up in a heap dump")
	}

	other := bucket(httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/calendar/"+wellFormedTokenWithSeed(t, 0x40)+".ics", nil))
	if other.Key == feed.Key {
		t.Error("two tokens share a bucket")
	}

	elsewhere := bucket(httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/items", nil))
	if elsewhere.Key != "" {
		t.Errorf("the feed bucket applied to %q", "/items")
	}
}

func wellFormedTokenWithSeed(t *testing.T, seed byte) string {
	t.Helper()
	entropy := make([]byte, integration.FeedTokenSecretBytes)
	for i := range entropy {
		entropy[i] = seed + byte(i)
	}
	token, err := integration.NewFeedToken(
		shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), entropy)
	if err != nil {
		t.Fatal(err)
	}
	return token.Secret()
}
