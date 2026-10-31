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

// timeRecordsWriter hands time records to accounting (UC-TIM-05) as JSON, a CSV file or a sheet,
// by the Accept header (`x-hubtask-rest-writer`): the use case selects and caps the rows, and a
// wire format is an adapter's business.
func (c *RestController) timeRecordsWriter(r *http.Request, _ map[string]any) (outputWriter, error) {
	switch {
	case acceptsXlsx(r):
		return writeTimeXlsx, nil
	case acceptsCSV(r):
		return writeTimeCSV, nil
	}
	return func(w http.ResponseWriter, r *http.Request, out usecase.Output) {
		var answer openapi.TimeExport
		if err := remarshal(out, &answer); err != nil {
			WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
			return
		}
		writeJSON(w, r, http.StatusOK, answer)
	}, nil
}

// writeTimeCSV answers the columns and rows as a CSV file.
func writeTimeCSV(w http.ResponseWriter, r *http.Request, out usecase.Output) {
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

// writeTimeXlsx answers the same columns and rows as one sheet (UC-TIM-05): minutes and booleans
// keep their type, a text is never a formula.
func writeTimeXlsx(w http.ResponseWriter, r *http.Request, out usecase.Output) {
	columns, _ := out["columns"].([]string)
	rows, _ := out["rows"].([]usecase.Output)
	if truncated, _ := out["truncated"].(bool); truncated {
		w.Header().Set(exportTruncatedHeader, "true")
	}
	w.Header().Set("Content-Type", xlsxType)
	w.Header().Set("Content-Disposition", `attachment; filename="hubtask-time.xlsx"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	err := writeXlsx(w, "Time", columns, func(yield func([]any) error) error {
		cells := make([]any, len(columns))
		for _, row := range rows {
			for i, column := range columns {
				cells[i] = row[column]
			}
			if err := yield(cells); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		reportExportFailure(r, err)
	}
}

// acceptsXlsx reports whether the request asks for a spreadsheet before CSV or JSON.
func acceptsXlsx(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		switch strings.TrimSpace(strings.SplitN(part, ";", 2)[0]) {
		case xlsxType:
			return true
		case "text/csv", "application/json", "*/*", "application/*":
			return false
		}
	}
	return false
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
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}
