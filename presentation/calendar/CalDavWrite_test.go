// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// recording performs nothing and remembers what it was asked, answering a version one higher
// each time - the shape a use case answers with, without a use case.
type recording struct {
	calls   []string
	inputs  []usecase.Input
	version int
	refuse  error
}

func (r *recording) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	if actor.AccountID.IsZero() {
		return nil, shared.ErrForbidden
	}
	r.calls = append(r.calls, name)
	r.inputs = append(r.inputs, in)
	if r.refuse != nil {
		return nil, r.refuse
	}
	r.version++
	return usecase.Output{"version": r.version, "id": in.String("id")}, nil
}

func writable() (*Controller, *recording) {
	c, _ := controller()
	rec := &recording{version: 3}
	c.UseCases = rec
	return c, rec
}

func put(t *testing.T, c *Controller, target, ifMatch string, body []byte) *httpResponse {
	t.Helper()
	r := newRequest(t, http.MethodPut, target, body)
	r.Header.Set("Content-Type", "text/calendar")
	if ifMatch != "" {
		r.Header.Set("If-Match", ifMatch)
	}
	return serve(c, r)
}

func TestACompletionAndADateWrittenBackLandThroughTheUseCases(t *testing.T) {
	c, rec := writable()
	me := accountID.String()
	w := put(t, c, memberPath(me, feedID.String(), itemID.String()), `"3"`, fixture(t, "reminders-put-completed.ics"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	// The date moved from the 20th to the 24th as an all-day date, and the box was ticked; the
	// title did not change and is not written; the VALARM and the X- lines are the client's.
	if strings.Join(rec.calls, ",") != "SetDueDate,CompleteWorkItem" {
		t.Fatalf("calls = %v", rec.calls)
	}
	due := rec.inputs[0]
	if due["item_id"] != itemID.String() || due["expected_version"] != 3 || due["due_date_only"] != true || due["due_time_zone"] != "Europe/Berlin" {
		t.Errorf("SetDueDate input = %v", due)
	}
	if !strings.HasPrefix(due.String("due_at"), "2026-09-24T00:00:00+02:00") {
		t.Errorf("a DATE is the day in the account's zone: %s", due.String("due_at"))
	}
	if rec.inputs[1]["expected_version"] != 4 {
		t.Errorf("the second write carries the version the first answered: %v", rec.inputs[1])
	}
	if etag := w.Header().Get("ETag"); etag != `"5"` {
		t.Errorf("the answer carries the version afterwards: %s", etag)
	}
}

func TestARenameAndAReopenAndAClearedDate(t *testing.T) {
	c, rec := writable()
	me := accountID.String()
	body := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTODO\r\nUID:x\r\nSUMMARY:Renamed\r\nSTATUS:NEEDS-ACTION\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	// The child is completed and has a due date; the client sends it open, renamed, undated.
	w := put(t, c, memberPath(me, feedID.String(), childID.String()), `"1"`, []byte(body))
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	if strings.Join(rec.calls, ",") != "UpdateWorkItem,ClearDueDate,ReopenWorkItem" {
		t.Fatalf("calls = %v", rec.calls)
	}
	if rec.inputs[0]["title"] != "Renamed" {
		t.Errorf("the title travels: %v", rec.inputs[0])
	}
}

func TestATimedDueAndAStart(t *testing.T) {
	c, rec := writable()
	me := accountID.String()
	body := "BEGIN:VCALENDAR\r\nBEGIN:VTODO\r\nUID:x\r\nSUMMARY:Write the reference\\, with a comma\r\nDTSTART;TZID=Europe/Berlin:20260920T080000\r\nDUE;TZID=Europe/Berlin:20260921T110000\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	w := put(t, c, memberPath(me, feedID.String(), itemID.String()), `"3"`, []byte(body))
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	if strings.Join(rec.calls, ",") != "UpdateWorkItem,SetDueDate" {
		t.Fatalf("calls = %v", rec.calls)
	}
	if rec.inputs[0].String("start_at") != "2026-09-20T06:00:00Z" {
		t.Errorf("a TZID DATE-TIME is the instant: %v", rec.inputs[0])
	}
	if rec.inputs[1].String("due_at") != "2026-09-21T09:00:00Z" || rec.inputs[1]["due_date_only"] != nil {
		t.Errorf("a timed DUE is the instant, not a date: %v", rec.inputs[1])
	}
}

func TestTheRaceAndTheRefusals(t *testing.T) {
	c, rec := writable()
	me := accountID.String()
	body := fixture(t, "reminders-put-completed.ics")
	target := memberPath(me, feedID.String(), itemID.String())

	if w := put(t, c, target, "", body); w.Code != http.StatusPreconditionRequired {
		t.Errorf("a PUT without If-Match is 428, got %d", w.Code)
	}
	if w := put(t, c, target, `"2"`, body); w.Code != http.StatusPreconditionFailed {
		t.Errorf("a stale If-Match is 412, got %d", w.Code)
	}
	if len(rec.calls) != 0 {
		t.Errorf("nothing was written: %v", rec.calls)
	}

	withNotes := strings.Replace(string(body), "SUMMARY:", "DESCRIPTION:Some notes\r\nPRIORITY:1\r\nSUMMARY:", 1)
	w := put(t, c, target, `"3"`, []byte(withNotes))
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "property-not-supported") || !strings.Contains(w.Body.String(), "DESCRIPTION,PRIORITY") {
		t.Errorf("an unmodelled property is refused by name: %d %s", w.Code, w.Body.String())
	}
	if w := put(t, c, target, `"3"`, []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")); w.Code != http.StatusBadRequest {
		t.Errorf("no VTODO is 400, got %d", w.Code)
	}

	// A refusal from the use case travels as its status.
	rec.refuse = shared.ErrForbidden.WithDetail("forbidden")
	if w := put(t, c, target, `"3"`, body); w.Code != http.StatusForbidden {
		t.Errorf("a use case's refusal is its status, got %d", w.Code)
	}

	readOnly, _ := controller()
	if w := put(t, readOnly, target, `"3"`, body); w.Code != http.StatusForbidden {
		t.Errorf("a tree without a catalogue is read-only, got %d", w.Code)
	}
}

func TestCreatingThroughPut(t *testing.T) {
	c, rec := writable()
	me := accountID.String()
	body := "BEGIN:VCALENDAR\r\nBEGIN:VTODO\r\nUID:0192f000-0000-7000-8000-0000000000aa\r\nSUMMARY:Made in Reminders\r\nDUE;VALUE=DATE:20260930\r\nSTATUS:COMPLETED\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"

	// The view names no single collection: refused by name.
	w := put(t, c, memberPath(me, feedID.String(), "0192f000-0000-7000-8000-0000000000aa"), "", []byte(body))
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "no-single-collection") {
		t.Fatalf("a hub-wide view refuses a creation: %d %s", w.Code, w.Body.String())
	}

	// The view names one collection: created with the client's identifier, then completed.
	views := c.Views.(*fakeViews)
	views.collection = shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	w = put(t, c, memberPath(me, feedID.String(), "0192f000-0000-7000-8000-0000000000aa"), "", []byte(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if strings.Join(rec.calls, ",") != "CreateWorkItem,CompleteWorkItem" {
		t.Fatalf("calls = %v", rec.calls)
	}
	in := rec.inputs[0]
	if in["id"] != "0192f000-0000-7000-8000-0000000000aa" || in["collection_id"] != "0192f000-0000-7000-8000-0000000000c1" || in["type"] != "TASK" || in["title"] != "Made in Reminders" || in["due_date_only"] != true {
		t.Errorf("CreateWorkItem input = %v", in)
	}

	// A client whose UID is not a UUIDv7 cannot be given its address back.
	rec.calls = nil
	w = put(t, c, memberPath(me, feedID.String(), "8B2C1D2E-0000-4000-8000-000000000000"), "", []byte(body))
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "creation-needs-uuidv7") || len(rec.calls) != 0 {
		t.Errorf("a v4 UID is refused by name: %d %s %v", w.Code, w.Body.String(), rec.calls)
	}
	// If-Match on an address that has no member is a lost race.
	if w := put(t, c, memberPath(me, feedID.String(), "0192f000-0000-7000-8000-0000000000ab"), `"1"`, []byte(body)); w.Code != http.StatusPreconditionFailed {
		t.Errorf("If-Match on a missing member is 412, got %d", w.Code)
	}
}

func TestDeleteTrashes(t *testing.T) {
	c, rec := writable()
	me := accountID.String()
	r := newRequest(t, http.MethodDelete, memberPath(me, feedID.String(), itemID.String()), nil)
	r.Header.Set("If-Match", `"3"`)
	if w := serve(c, r); w.Code != http.StatusNoContent {
		t.Fatalf("DELETE: %d", w.Code)
	}
	if strings.Join(rec.calls, ",") != "TrashWorkItem" || rec.inputs[0]["expected_version"] != 3 {
		t.Errorf("calls = %v %v", rec.calls, rec.inputs)
	}
	r = newRequest(t, http.MethodDelete, memberPath(me, feedID.String(), itemID.String()), nil)
	r.Header.Set("If-Match", `"9"`)
	if w := serve(c, r); w.Code != http.StatusPreconditionFailed {
		t.Errorf("a stale If-Match on DELETE is 412, got %d", w.Code)
	}
	if w := serve(c, newRequest(t, http.MethodDelete, memberPath(me, feedID.String(), "0192f000-0000-7000-8000-0000000000ff"), nil)); w.Code != http.StatusNotFound {
		t.Errorf("DELETE of a member that is not there is 404, got %d", w.Code)
	}
}

func TestParseTodoReadsTheDialect(t *testing.T) {
	parsed, err := ParseTodo(fixture(t, "reminders-put-completed.ics"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.UID != "0192f000-0000-7000-8000-00000000000e@hubtask" || parsed.Summary != "Write the reference, with a comma" {
		t.Errorf("parsed = %+v", parsed)
	}
	if !parsed.DueDate || parsed.Due != time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC) || !parsed.Completed {
		t.Errorf("due/completed = %+v", parsed)
	}
	if len(parsed.Unsupported) != 0 {
		t.Errorf("the VALARM and the X- lines are the client's: %v", parsed.Unsupported)
	}
	folded := "BEGIN:VCALENDAR\r\nBEGIN:VTODO\r\nUID:x\r\nSUMMARY:A title long enough to be folded across two\r\n  lines by the client\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	if parsed, err := ParseTodo([]byte(folded)); err != nil || parsed.Summary != "A title long enough to be folded across two lines by the client" {
		t.Errorf("unfolding: %v %q", err, parsed.Summary)
	}
	if _, err := ParseTodo([]byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")); err == nil {
		t.Error("a document without a VTODO is refused")
	}
}
