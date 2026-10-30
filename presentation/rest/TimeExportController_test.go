// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/csv"
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
