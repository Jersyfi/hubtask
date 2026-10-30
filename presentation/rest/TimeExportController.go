// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// ExportTimeRecords hands time records to accounting (UC-TIM-05): the use case selects and caps
// the rows; whether they leave as JSON or as a CSV file is this adapter's, by the Accept header,
// because a wire format is an adapter's business.
func (c *RestController) ExportTimeRecords(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	var body openapi.ExportTimeRecordsJSONRequestBody
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	values, err := members(body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{}
	for name, value := range values {
		if value != nil {
			in[name] = value
		}
	}
	out, ok := c.read(w, r, "ExportTimeRecords", in)
	if !ok {
		return
	}
	if !acceptsCSV(r) {
		var answer openapi.TimeExport
		if err := remarshal(out, &answer); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
		writeJSON(w, r, http.StatusOK, answer)
		return
	}
	columns, _ := out["columns"].([]string)
	rows, _ := out["rows"].([]usecase.Output)
	if truncated, _ := out["truncated"].(bool); truncated {
		w.Header().Set(exportTruncatedHeader, "true")
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="hubtask-time.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	writer := csv.NewWriter(w)
	if err := writer.Write(columns); err != nil {
		reportExportFailure(r, err)
		return
	}
	record := make([]string, len(columns))
	for _, row := range rows {
		for i, column := range columns {
			record[i] = timeRecordCell(row[column])
		}
		if err := writer.Write(record); err != nil {
			reportExportFailure(r, err)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		reportExportFailure(r, err)
	}
}

// acceptsCSV reports whether the request asks for a CSV file before JSON.
func acceptsCSV(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		kind := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		switch kind {
		case "text/csv":
			return true
		case "application/json", "*/*", "application/*":
			return false
		}
	}
	return false
}

// timeRecordCell writes one cell of a time record; a text a spreadsheet would read as a formula - a
// note starting with "=" - is prefixed with a quote mark, so that it never runs.
func timeRecordCell(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
			return "'" + v
		}
		return v
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}
