// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The three renderings, at the layer that decides them (D-08). The rows are the catalogue's; what
// they become - a spreadsheet, a document, a calendar - is this layer's, so this is where it is
// tested.

const exportViewID = "0192f000-0000-7000-8000-000000000081"

// moment is how the catalogue answers a time: as one, not as text. Every projection in the
// application layer does, which is what lets this layer render it in whichever zone was asked for.
func moment(text string) time.Time {
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		panic(err)
	}
	return at
}

func exportedRows() usecase.Output {
	dated := createdItem()
	dated["title"] = "Pack the kitchen, carefully"
	dated["due_at"] = moment("2026-10-22T14:30:00Z")
	dated["due_date_only"] = false
	dated["updated_at"] = moment("2026-10-19T14:30:00Z")

	allDay := createdItem()
	allDay["id"] = "0192f000-0000-7000-8000-000000000302"
	allDay["title"] = "Hand back the keys"
	allDay["due_at"] = moment("2026-10-22T22:00:00Z")
	allDay["due_date_only"] = true
	allDay["due_time_zone"] = "Europe/Berlin"

	undated := createdItem()
	undated["id"] = "0192f000-0000-7000-8000-000000000303"
	undated["title"] = "Somewhere on the list"

	return usecase.Output{
		"view_id":      exportViewID,
		"view_name":    "This week",
		"generated_at": moment("2026-10-20T06:00:00Z"),
		"count":        3,
		"truncated":    false,
		"rows":         []usecase.Output{dated, allDay, undated},
	}
}

func postExport(t *testing.T, registry UseCaseRegistry, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry
	controller.BaseURL = "https://hubtask.example.com/"

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		TimeZone:  "Europe/Berlin",
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost,
		APIBasePath+"/views/"+exportViewID+":export", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestTheJSONExportCarriesTheRowsAndWhatItKnowsAboutItself(t *testing.T) {
	recorder := postExport(t, &catalogue{out: exportedRows()}, `{"format":"JSON"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	var document struct {
		ViewID    string `json:"view_id"`
		Count     int    `json:"count"`
		Truncated bool   `json:"truncated"`
		Rows      []struct {
			Title string `json:"title"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
		t.Fatalf("the body is not the document: %v", err)
	}
	if document.ViewID != exportViewID || document.Count != 3 || len(document.Rows) != 3 {
		t.Errorf("the document is %+v", document)
	}
	if document.Truncated {
		t.Error("an export that fits reported itself truncated")
	}
	if recorder.Header().Get(exportTruncatedHeader) != "" {
		t.Error("the header claims a truncation that did not happen")
	}
}

// A spreadsheet is a fixed set of columns, whatever a client draws, and the times are written in
// the caller's own zone because a person reads them.
func TestTheCSVExportIsAFixedShapeInTheCallersZone(t *testing.T) {
	recorder := postExport(t, &catalogue{out: exportedRows()}, `{"format":"CSV"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if kind := recorder.Header().Get("Content-Type"); !strings.HasPrefix(kind, "text/csv") {
		t.Errorf("content type %q", kind)
	}
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.Contains(
		disposition, "attachment") {
		t.Errorf("content disposition %q", disposition)
	}

	records, err := csv.NewReader(strings.NewReader(recorder.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("the body is not a spreadsheet: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("the file has %d lines", len(records))
	}
	if len(records[0]) != len(exportColumns) || records[0][2] != "title" {
		t.Errorf("the header line is %v", records[0])
	}

	// A title with a comma in it survives, because the writer quotes it rather than this code
	// pasting strings together.
	if records[1][2] != "Pack the kitchen, carefully" {
		t.Errorf("the title came out as %q", records[1][2])
	}
	// 14:30Z is 16:30 in Berlin, which is what the person exporting sees.
	if !strings.HasPrefix(records[1][10], "2026-10-22T16:30:00+02:00") {
		t.Errorf("the due date came out as %q", records[1][10])
	}
	// And the entry with no date is in the spreadsheet, with an empty cell.
	if records[3][10] != "" {
		t.Errorf("an undated entry carries %q", records[3][10])
	}
}

// The calendar rendering: only the dated entries, the all-day one as a date in its own zone, and
// the document delivered as a document.
func TestTheICSExportIsTheDatedEntriesOnly(t *testing.T) {
	recorder := postExport(t, &catalogue{out: exportedRows()}, `{"format":"ICS"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if kind := recorder.Header().Get("Content-Type"); !strings.HasPrefix(kind, "text/calendar") {
		t.Errorf("content type %q", kind)
	}
	body := recorder.Body.String()

	if strings.Count(body, "BEGIN:VEVENT") != 2 {
		t.Errorf("the calendar has %d entries and three rows went in",
			strings.Count(body, "BEGIN:VEVENT"))
	}
	if strings.Contains(body, "Somewhere on the list") {
		t.Error("an entry with no due date became a calendar entry")
	}
	if !strings.Contains(body, "X-WR-CALNAME:This week") {
		t.Error("the calendar carries no name")
	}
	// The timed one as its instant, the all-day one as the day it is in Berlin - 22:00Z on the
	// 22nd is midnight on the 23rd there.
	if !strings.Contains(body, "DTSTART:20261022T143000Z") {
		t.Error("the timed entry is not its instant")
	}
	if !strings.Contains(body, "DTSTART;VALUE=DATE:20261023") {
		t.Error("the all-day entry is not the day it falls on in its own zone")
	}
	if !strings.Contains(body, "URL:https://hubtask.example.com/items/") {
		t.Error("the entries carry no link back")
	}
}

// A truncation is announced in the header, on every format.
func TestATruncatedExportSaysSoInTheHeader(t *testing.T) {
	out := exportedRows()
	out["truncated"] = true

	for _, format := range []string{"JSON", "CSV", "ICS"} {
		t.Run(format, func(t *testing.T) {
			recorder := postExport(t, &catalogue{out: out}, `{"format":"`+format+`"}`)
			if recorder.Header().Get(exportTruncatedHeader) != "true" {
				t.Errorf("the %s export did not announce the truncation", format)
			}
		})
	}
}

// A zone this installation does not know is refused rather than silently replaced: a file of
// timestamps in the wrong zone is worse than an error.
func TestAnUnknownZoneIsRefusedBeforeAnythingIsRead(t *testing.T) {
	cat := &catalogue{out: exportedRows()}
	recorder := postExport(t, cat, `{"format":"CSV","time_zone":"Mars/Olympus_Mons"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if cat.invoked {
		t.Error("the rows were read for an export that could not be written")
	}
}

// The zone the request names wins over the caller's own.
func TestTheRequestedZoneDecidesHowTheTimesRead(t *testing.T) {
	recorder := postExport(t, &catalogue{out: exportedRows()},
		`{"format":"CSV","time_zone":"UTC"}`)

	records, err := csv.NewReader(strings.NewReader(recorder.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("the body is not a spreadsheet: %v", err)
	}
	if records[1][10] != "2026-10-22T14:30:00Z" {
		t.Errorf("the due date came out as %q", records[1][10])
	}
}
