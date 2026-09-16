// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	accountID = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	otherID   = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	feedID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	viewID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	itemID    = shared.MustParseID("0192f000-0000-7000-8000-00000000000e")
	childID   = shared.MustParseID("0192f000-0000-7000-8000-00000000001e")
	doneID    = shared.MustParseID("0192f000-0000-7000-8000-00000000002e")
)

type fakeFeeds struct{ feeds []integration.CalendarFeed }

func (f fakeFeeds) Execute(context.Context, appshared.ActorContext) ([]integration.CalendarFeed, error) {
	return f.feeds, nil
}

type fakeViews struct {
	items      []workmodel.WorkItem
	asked      []shared.ID
	collection shared.ID
}

type httpResponse = httptest.ResponseRecorder

func newRequest(t *testing.T, method, target string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(string(body)))
	return r.WithContext(appshared.ContextWithActor(r.Context(), actor()))
}

func serve(c *Controller, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	return w
}

func (f *fakeViews) Select(_ context.Context, actor appshared.ActorContext, id shared.ID) (work.ExportedView, error) {
	f.asked = append(f.asked, id)
	if id != viewID {
		return work.ExportedView{}, shared.ErrNotFound.WithDetail("views.not_found")
	}
	saved := view.SavedView{ID: viewID, Name: "This week", Query: map[string]any{}}
	if !f.collection.IsZero() {
		saved.Query["collection_id"] = f.collection.String()
	}
	return work.ExportedView{
		View:        saved,
		Items:       f.items,
		TimeZone:    actor.TimeZone,
		GeneratedAt: time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC),
	}, nil
}

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: otherID, AccountID: accountID,
		AccountName: "Anna", TimeZone: "Europe/Berlin", Scopes: []string{"views:read", "items:read"},
	}
}

func items() []workmodel.WorkItem {
	due := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	dayDue := time.Date(2026, 9, 22, 22, 30, 0, 0, time.UTC) // 00:30 on the 23rd in Berlin
	completedAt := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 9, 14, 7, 0, 0, 0, time.UTC)
	return []workmodel.WorkItem{
		{ID: itemID, Title: "Write the reference, with a comma", Due: &workmodel.DueDate{At: due}, UpdatedAt: updated, Version: 3},
		{ID: childID, ParentID: itemID, Title: "A child", Due: &workmodel.DueDate{At: due}, Completion: workmodel.Completion{IsCompleted: true, CompletedAt: &completedAt}, UpdatedAt: updated, Version: 1},
		{ID: doneID, Title: "All day", Due: &workmodel.DueDate{At: dayDue, DateOnly: true}, UpdatedAt: updated, Version: 2},
		{ID: shared.MustParseID("0192f000-0000-7000-8000-00000000003e"), Title: "Undated, not a moment", UpdatedAt: updated, Version: 1},
	}
}

func controller() (*Controller, *fakeViews) {
	views := &fakeViews{items: items()}
	return &Controller{
		Feeds: fakeFeeds{feeds: []integration.CalendarFeed{
			{ID: feedID, AccountID: accountID, ViewID: viewID},
			{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000f2"), AccountID: accountID, ViewID: viewID, RevokedAt: time.Now()},
		}},
		Views:   views,
		BaseURL: "https://hubtask.example",
		Now:     func() time.Time { return time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC) },
	}, views
}

func do(t *testing.T, c *Controller, method, target, depth string, body []byte, as appshared.ActorContext) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	} else {
		reader = strings.NewReader("")
	}
	r := httptest.NewRequestWithContext(t.Context(), method, target, reader)
	if depth != "" {
		r.Header.Set("Depth", depth)
	}
	if !as.AccountID.IsZero() {
		r = r.WithContext(appshared.ContextWithActor(r.Context(), as))
	}
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	return w
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type multistatus struct {
	Responses []struct {
		Href     string `xml:"href"`
		Status   string `xml:"status"`
		Propstat []struct {
			Status string `xml:"status"`
			Prop   struct {
				Inner []byte `xml:",innerxml"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

func parse(t *testing.T, w *httptest.ResponseRecorder) multistatus {
	t.Helper()
	if w.Code != http.StatusMultiStatus {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out multistatus
	if err := xml.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("not a multistatus: %v\n%s", err, w.Body.String())
	}
	return out
}

func found(t *testing.T, ms multistatus, href string) string {
	t.Helper()
	for _, r := range ms.Responses {
		if r.Href != href {
			continue
		}
		for _, ps := range r.Propstat {
			if strings.Contains(ps.Status, "200") {
				return string(ps.Prop.Inner)
			}
		}
		return ""
	}
	t.Fatalf("no response for %s in %+v", href, ms)
	return ""
}

func TestTheWalkFromTheRootToATodo(t *testing.T) {
	c, _ := controller()
	me := accountID.String()

	// The root names the principal (what Reminders asks first).
	root := parse(t, do(t, c, "PROPFIND", Prefix, "0", fixture(t, "reminders-propfind-principal.xml"), actor()))
	if props := found(t, root, Prefix); !strings.Contains(props, principalPath(me)) {
		t.Errorf("the root does not name the principal: %s", props)
	}

	// The principal names the calendar home.
	principal := parse(t, do(t, c, "PROPFIND", principalPath(me), "0", []byte(`<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><C:calendar-home-set/><D:displayname/></D:prop></D:propfind>`), actor()))
	props := found(t, principal, principalPath(me))
	if !strings.Contains(props, homePath(me)) || !strings.Contains(props, "Anna") {
		t.Errorf("the principal does not name the home or the person: %s", props)
	}

	// The home, at depth 1, lists the live feed as a VTODO calendar - and not the revoked one.
	home := parse(t, do(t, c, "PROPFIND", homePath(me), "1", fixture(t, "reminders-propfind-home.xml"), actor()))
	if len(home.Responses) != 2 {
		t.Fatalf("home should answer itself and one calendar, got %d", len(home.Responses))
	}
	cal := found(t, home, calendarPath(me, feedID.String()))
	for _, want := range []string{"This week", `name="VTODO"`, "getctag", "<read", "calendar"} {
		if !strings.Contains(cal, want) {
			t.Errorf("the calendar lacks %q: %s", want, cal)
		}
	}
	// A property nobody has is 404 in its own propstat, never a refusal of the request.
	var sawNotFound bool
	for _, r := range home.Responses {
		for _, ps := range r.Propstat {
			if strings.Contains(ps.Status, "404") && strings.Contains(string(ps.Prop.Inner), "sync-token") {
				sawNotFound = true
			}
		}
	}
	if !sawNotFound {
		t.Error("sync-token should be answered 404, not omitted or claimed")
	}

	// The calendar, at depth 1, lists its members with an ETag each: three dated entries, and
	// not the undated one.
	members := parse(t, do(t, c, "PROPFIND", calendarPath(me, feedID.String()), "1", nil, actor()))
	if len(members.Responses) != 4 {
		t.Fatalf("calendar should answer itself and three members, got %d", len(members.Responses))
	}
	if etag := found(t, members, memberPath(me, feedID.String(), itemID.String())); !strings.Contains(etag, `&#34;3&#34;`) {
		t.Errorf("the member's etag is its version: %s", etag)
	}

	// A todo, fetched.
	got := do(t, c, http.MethodGet, memberPath(me, feedID.String(), itemID.String()), "", nil, actor())
	if got.Code != http.StatusOK || got.Header().Get("ETag") != `"3"` {
		t.Fatalf("GET: %d %v", got.Code, got.Header())
	}
	body := got.Body.String()
	for _, want := range []string{"BEGIN:VTODO", "SUMMARY:Write the reference\\, with a comma", "DUE:20260920T090000Z", "STATUS:NEEDS-ACTION", "PERCENT-COMPLETE:100", "URL:https://hubtask.example/items/" + itemID.String(), "LAST-MODIFIED:20260914T070000Z"} {
		if !strings.Contains(body, want) {
			t.Errorf("the todo lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "COMPLETED:") {
		t.Error("an open todo carries no COMPLETED")
	}
	// If-None-Match with the current tag is 304.
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, memberPath(me, feedID.String(), itemID.String()), nil)
	r.Header.Set("If-None-Match", `"3"`)
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r.WithContext(appshared.ContextWithActor(r.Context(), actor())))
	if w.Code != http.StatusNotModified {
		t.Errorf("If-None-Match should answer 304, got %d", w.Code)
	}

	// The all-day entry is a DATE in the owner's zone, and the completed child carries both.
	day := do(t, c, http.MethodGet, memberPath(me, feedID.String(), doneID.String()), "", nil, actor()).Body.String()
	if !strings.Contains(day, "DUE;VALUE=DATE:20260923") {
		t.Errorf("an all-day due is the day in the owner's zone:\n%s", day)
	}
	child := do(t, c, http.MethodGet, memberPath(me, feedID.String(), childID.String()), "", nil, actor()).Body.String()
	if !strings.Contains(child, "STATUS:COMPLETED") || !strings.Contains(child, "COMPLETED:20260915T080000Z") {
		t.Errorf("a done todo carries STATUS and COMPLETED:\n%s", child)
	}
}

func TestTheCtagMovesWhenAnyEntryDoes(t *testing.T) {
	c, views := controller()
	me := accountID.String()
	before := found(t, parse(t, do(t, c, "PROPFIND", calendarPath(me, feedID.String()), "0", nil, actor())), calendarPath(me, feedID.String()))
	views.items[0].Version = 4
	after := found(t, parse(t, do(t, c, "PROPFIND", calendarPath(me, feedID.String()), "0", nil, actor())), calendarPath(me, feedID.String()))
	ctag := func(s string) string {
		start := strings.Index(s, "<getctag")
		return s[start : start+80]
	}
	if ctag(before) == ctag(after) {
		t.Error("the ctag did not move when an entry's version did")
	}
}

func TestCalendarQueryAppliesTheTimeRangeToDue(t *testing.T) {
	c, _ := controller()
	me := accountID.String()
	ms := parse(t, do(t, c, "REPORT", calendarPath(me, feedID.String()), "1", fixture(t, "reminders-calendar-query.xml"), actor()))
	if len(ms.Responses) != 3 {
		t.Fatalf("three entries are due in September, got %d", len(ms.Responses))
	}
	if data := found(t, ms, memberPath(me, feedID.String(), itemID.String())); !strings.Contains(data, "BEGIN:VTODO") || !strings.Contains(data, "getetag") {
		t.Errorf("a report carries the data and the etag: %s", data)
	}

	october := strings.ReplaceAll(string(fixture(t, "reminders-calendar-query.xml")), `start="20260901T000000Z" end="20261001T000000Z"`, `start="20261001T000000Z" end="20261101T000000Z"`)
	none := parse(t, do(t, c, "REPORT", calendarPath(me, feedID.String()), "1", []byte(october), actor()))
	if len(none.Responses) != 0 {
		t.Errorf("nothing is due in October, got %d", len(none.Responses))
	}
	events := strings.ReplaceAll(string(fixture(t, "reminders-calendar-query.xml")), `name="VTODO"`, `name="VEVENT"`)
	if got := parse(t, do(t, c, "REPORT", calendarPath(me, feedID.String()), "1", []byte(events), actor())); len(got.Responses) != 0 {
		t.Errorf("a query for events answers nothing here, got %d", len(got.Responses))
	}
}

func TestMultigetAnswersWhatItRemembersAnd404ForWhatIsGone(t *testing.T) {
	c, _ := controller()
	me := accountID.String()
	ms := parse(t, do(t, c, "REPORT", calendarPath(me, feedID.String()), "1", fixture(t, "thunderbird-multiget.xml"), actor()))
	if len(ms.Responses) != 2 {
		t.Fatalf("two hrefs, two responses, got %d", len(ms.Responses))
	}
	if !strings.Contains(ms.Responses[1].Status, "404") {
		t.Errorf("the member that is gone should be 404: %+v", ms.Responses[1])
	}
	if data := found(t, ms, memberPath(me, feedID.String(), itemID.String())); !strings.Contains(data, "BEGIN:VTODO") {
		t.Errorf("the member that exists carries its data: %s", data)
	}
}

func TestAnotherAccountsTreeIsAbsentAndAFeedNotOwnedIsAbsent(t *testing.T) {
	c, _ := controller()
	stranger := actor()
	stranger.AccountID = otherID
	if w := do(t, c, "PROPFIND", homePath(accountID.String()), "1", nil, stranger); w.Code != http.StatusNotFound {
		t.Errorf("another account's home should be 404, got %d", w.Code)
	}
	if w := do(t, c, "PROPFIND", calendarPath(accountID.String(), "0192f000-0000-7000-8000-0000000000f9"), "0", nil, actor()); w.Code != http.StatusNotFound {
		t.Errorf("a feed the account does not own should be 404, got %d", w.Code)
	}
	if w := do(t, c, "PROPFIND", calendarPath(accountID.String(), "0192f000-0000-7000-8000-0000000000f2"), "0", nil, actor()); w.Code != http.StatusNotFound {
		t.Errorf("a revoked feed should be 404, got %d", w.Code)
	}
	if w := do(t, c, "PROPFIND", Prefix, "0", nil, appshared.ActorContext{}); w.Code != http.StatusForbidden {
		t.Errorf("no actor should be refused, got %d", w.Code)
	}
}

func TestOptionsAndUnsupportedMethodsAndMalformedXML(t *testing.T) {
	c, _ := controller()
	if w := do(t, c, http.MethodOptions, Prefix, "", nil, actor()); w.Code != http.StatusOK || !strings.Contains(w.Header().Get("DAV"), "calendar-access") {
		t.Errorf("OPTIONS: %d %v", w.Code, w.Header())
	}
	if w := do(t, c, "MKCALENDAR", homePath(accountID.String())+"x/", "", nil, actor()); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("MKCALENDAR is not offered, got %d", w.Code)
	}
	bomb := `<?xml version="1.0"?><!DOCTYPE lolz [<!ENTITY lol "lol"><!ENTITY lol2 "&lol;&lol;&lol;">]><D:propfind xmlns:D="DAV:"><D:prop><D:displayname>&lol2;</D:displayname></D:prop></D:propfind>`
	w := do(t, c, "PROPFIND", Prefix, "0", []byte(bomb), actor())
	// The decoder knows no DTD: the entity is not expanded, and the request is refused as
	// malformed rather than served with an expansion it never made.
	if w.Code != http.StatusBadRequest {
		t.Errorf("an internal entity should be refused, got %d: %s", w.Code, w.Body.String())
	}
	if w := do(t, c, "PROPFIND", Prefix, "0", []byte("<not xml"), actor()); w.Code != http.StatusBadRequest {
		t.Errorf("broken XML should be 400, got %d", w.Code)
	}
}
