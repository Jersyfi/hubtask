// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func exportedTime() usecase.Output {
	return usecase.Output{
		"columns": []string{"record_id", "day", "minutes", "hours", "billable", "corrects", "note"},
		"rows": []usecase.Output{{
			"record_id": "0192f000-0000-7000-8000-000000000401", "day": "2026-11-02", "minutes": 90,
			"hours": 1.5, "billable": true, "corrects": nil, "note": "=HYPERLINK(\"x\")",
		}},
		"truncated": true,
	}
}

func postTimeExport(t *testing.T, registry UseCaseRegistry, accept string) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, APIBasePath+"/time-records:export",
		strings.NewReader(`{"from":"2026-11-02","to":"2026-11-08","mine":true}`))
	request.Header.Set("Content-Type", "application/json")
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// UC-TIM-05: asked as text/csv, the export is a file of its columns with a formula defused; asked
// as JSON - or without a preference - the rows.
func TestTheTimeExportIsACSVFileWhenAskedForOne(t *testing.T) {
	use := &catalogue{out: exportedTime()}
	recorder := postTimeExport(t, use, "text/csv")
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("status %d %s: %s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body)
	}
	if use.name != "ExportTimeRecords" || use.in["mine"] != true || use.in["from"] != "2026-11-02" {
		t.Fatalf("the use case was asked %s %v", use.name, use.in)
	}
	if recorder.Header().Get(exportTruncatedHeader) != "true" {
		t.Error("a truncated file does not say so")
	}
	lines, err := csv.NewReader(recorder.Body).ReadAll()
	if err != nil || len(lines) != 2 {
		t.Fatalf("csv = %v %v", lines, err)
	}
	if strings.Join(lines[0], ",") != "record_id,day,minutes,hours,billable,corrects,note" {
		t.Errorf("header = %v", lines[0])
	}
	if got := lines[1]; got[2] != "90" || got[3] != "1.5" || got[4] != "true" || got[5] != "" || got[6] != `'=HYPERLINK("x")` {
		t.Errorf("row = %q", got)
	}

	recorder = postTimeExport(t, &catalogue{out: exportedTime()}, "")
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("without a preference: %d %s", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	if !strings.Contains(recorder.Body.String(), `"rows"`) {
		t.Errorf("the JSON answer = %s", recorder.Body)
	}
}

// UC-TIM-05: asked as a spreadsheet, the export is one sheet a spreadsheet opens - its parts, the
// header row, the minutes as a number, billable as a boolean and the note as a text, never a formula.
func TestTheTimeExportIsASpreadsheetWhenAskedForOne(t *testing.T) {
	recorder := postTimeExport(t, &catalogue{out: exportedTime()}, xlsxType)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != xlsxType {
		t.Fatalf("status %d %s: %s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body)
	}
	if recorder.Header().Get(exportTruncatedHeader) != "true" {
		t.Error("a truncated file does not say so")
	}
	body := recorder.Body.Bytes()
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("not a zip package: %v", err)
	}
	parts := map[string]string{}
	for _, f := range archive.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(r)
		parts[f.Name] = string(content)
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/worksheets/sheet1.xml"} {
		if parts[name] == "" {
			t.Errorf("the package has no %s", name)
		}
	}
	sheet := parts["xl/worksheets/sheet1.xml"]
	var parsed struct{}
	if err := xml.Unmarshal([]byte(sheet), &parsed); err != nil {
		t.Fatalf("the sheet is not XML: %v", err)
	}
	for _, want := range []string{
		`<c r="A1" t="inlineStr"><is><t xml:space="preserve">record_id</t></is></c>`,
		`<c r="C2"><v>90</v></c>`, `<c r="D2"><v>1.5</v></c>`, `<c r="E2" t="b"><v>1</v></c>`,
		`<c r="G2" t="inlineStr"><is><t xml:space="preserve">=HYPERLINK(&#34;x&#34;)</t></is></c>`,
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("the sheet lacks %s:\n%s", want, sheet)
		}
	}
	if strings.Contains(sheet, "<f>") {
		t.Error("the sheet carries a formula")
	}
}

func TestTheSpreadsheetColumnsAreLettered(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 701: "ZZ", 702: "AAA"} {
		if got := xlsxColumn(i); got != want {
			t.Errorf("column %d is %s, want %s", i, got, want)
		}
	}
}

// UC-TIM-05 check 5: an export's costs - amounts in the minor unit - are numbers in the CSV file
// and in the sheet, and an unpriced record's cost an empty cell.
func TestTheTimeExportCarriesItsCostsInBothFiles(t *testing.T) {
	priced := func() usecase.Output {
		return usecase.Output{
			"columns": []string{"minutes", "rate_minor", "currency", "cost_minor"},
			"rows": []usecase.Output{
				{"minutes": -30, "rate_minor": int64(6000), "currency": "EUR", "cost_minor": int64(-3000)},
				{"minutes": 30, "rate_minor": nil, "currency": "", "cost_minor": nil},
			},
		}
	}
	lines, err := csv.NewReader(postTimeExport(t, &catalogue{out: priced()}, "text/csv").Body).ReadAll()
	if err != nil || len(lines) != 3 {
		t.Fatalf("csv = %v %v", lines, err)
	}
	if got := strings.Join(lines[1], ","); got != "-30,6000,EUR,-3000" {
		t.Errorf("a priced row = %s", got)
	}
	if got := strings.Join(lines[2], ","); got != "30,,," {
		t.Errorf("an unpriced row = %s", got)
	}
	body := postTimeExport(t, &catalogue{out: priced()}, xlsxType).Body.Bytes()
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("not a zip package: %v", err)
	}
	for _, f := range archive.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		r, _ := f.Open()
		sheet, _ := io.ReadAll(r)
		if !strings.Contains(string(sheet), `<c r="D2"><v>-3000</v></c>`) || strings.Contains(string(sheet), `r="D3"`) {
			t.Errorf("the sheet's costs:\n%s", sheet)
		}
	}
}
