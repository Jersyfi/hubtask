// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/csv"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/calendar"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The view export (D-08). The use case selects the rows and caps them; which file they become is
// decided here, because a wire format is an adapter's business - and the ICS rendering is the same
// package the calendar feed serves from, which is why the export rides that task.

const exportViewUseCase = "ExportView"

// exportTruncatedHeader tells a caller that the file is the first max_export_rows of a longer
// answer. A header rather than a body field, because two of the three formats have no body field
// to put it in - and a truncation the caller is told about is honest where a silent one is not.
const exportTruncatedHeader = "Export-Truncated"

// exportColumns is the CSV file's shape, and it is fixed rather than derived from the view's
// visible_fields: a spreadsheet somebody built a formula on must not gain or lose a column
// because a client changed which ones it draws.
var exportColumns = []string{
	"id", "type", "title", "notes", "collection_id", "parent_id", "depth",
	"is_completed", "completed_at", "start_at", "due_at", "due_date_only", "due_time_zone",
	"assignee_id", "bucket_id", "order_key", "created_at", "updated_at",
}

// ExportView answers POST /views/{viewId}:export.
func (c *RestController) ExportView(
	w http.ResponseWriter, r *http.Request, viewID openapi.ViewId,
	_ openapi.ExportViewParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.ViewExport
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	zone, err := exportZone(r, body.TimeZone)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), exportViewUseCase, actorOf(r),
		usecase.Input{"view_id": viewID.String()})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["rows"].([]usecase.Output)
	if truncated, _ := out["truncated"].(bool); truncated {
		w.Header().Set(exportTruncatedHeader, "true")
	}

	switch body.Format {
	case openapi.JSON:
		writeJSON(w, r, http.StatusOK, openapi.ViewExportDocument{
			ViewId:      uuidValue(out.String("view_id")),
			GeneratedAt: timeValue(out["generated_at"]),
			Count:       out.Int("count"),
			Truncated:   out["truncated"] == true,
			Rows:        workItemResponses(rows),
		})
	case openapi.CSV:
		writeCSV(w, r, rows, zone)
	case openapi.ICS:
		writeCalendar(w, r, calendar.Calendar{
			Name:        out.String("view_name"),
			GeneratedAt: timeValue(out["generated_at"]),
			Events:      c.eventsOf(rows, zone),
		})
	default:
		WriteProblem(w, exportFormatUnknown(string(body.Format)), requestID)
	}
}

func workItemResponses(rows []usecase.Output) []openapi.WorkItem {
	items := make([]openapi.WorkItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, workItemResponse(row))
	}
	return items
}

// writeCSV streams the rows. The header line is always written, so an export with no rows is an
// empty spreadsheet rather than an empty file - which is what a person looking at a filter that
// matched nothing needs to see.
func writeCSV(w http.ResponseWriter, r *http.Request, rows []usecase.Output, zone *time.Location) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="hubtask-export.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	writer := csv.NewWriter(w)
	record := make([]string, len(exportColumns))
	if err := writer.Write(exportColumns); err != nil {
		reportExportFailure(r, err)
		return
	}
	for _, row := range rows {
		for i, column := range exportColumns {
			record[i] = csvValue(row, column, zone)
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

// csvValue renders one cell. Times are written in the caller's zone, because a spreadsheet is
// read by a person rather than by a machine; identifiers and flags are written as they are.
func csvValue(row usecase.Output, column string, zone *time.Location) string {
	switch column {
	case "is_completed":
		completion, _ := row["completion"].(map[string]any)
		return boolCell(completion["is_completed"])
	case "completed_at":
		completion, _ := row["completion"].(map[string]any)
		return timeCell(completion["completed_at"], zone)
	case "due_date_only":
		return boolCell(row["due_date_only"])
	case "depth":
		return strconv.Itoa(row.Int("depth"))
	case "start_at", "due_at", "created_at", "updated_at":
		return timeCell(row[column], zone)
	default:
		return row.String(column)
	}
}

func boolCell(value any) string {
	if flag, ok := value.(bool); ok && flag {
		return "true"
	}
	return "false"
}

func timeCell(value any, zone *time.Location) string {
	at := timeValue(value)
	if at.IsZero() {
		return ""
	}
	return at.In(zone).Format(time.RFC3339)
}

// writeCalendar answers the ICS rendering, with the headers §"Which response headers" of the task
// decided on: a calendar document is not a page, so it is delivered as one - and never cached by
// anything in between, because on the feed route the URL is itself the credential.
func writeCalendar(w http.ResponseWriter, r *http.Request, document calendar.Calendar) {
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="hubtask.ics"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(calendar.Render(document)); err != nil {
		reportExportFailure(r, err)
	}
}

// eventsOf turns the rows into calendar entries. An entry with no due date is not a calendar
// entry and does not appear: a calendar is a set of moments, and "somewhere on the list" is not
// one.
func (c *RestController) eventsOf(rows []usecase.Output, zone *time.Location) []calendar.Event {
	events := make([]calendar.Event, 0, len(rows))

	for _, row := range rows {
		due := timeValue(row["due_at"])
		if due.IsZero() {
			continue
		}

		id := row.String("id")
		event := calendar.Event{
			UID:          id + "@hubtask",
			Summary:      row.String("title"),
			Start:        due,
			URL:          c.itemURL(id),
			LastModified: timeValue(row["updated_at"]),
		}
		if allDay, _ := row["due_date_only"].(bool); allDay {
			// The day it is due, read in the entry's own zone - or the caller's, for an entry
			// stored without one - and carried as that day's wall clock, which is the shape the
			// renderer takes a DATE in.
			local := zone
			if named := row.String("due_time_zone"); named != "" {
				if loaded, err := time.LoadLocation(named); err == nil {
					local = loaded
				}
			}
			day := due.In(local)
			event.AllDay = true
			event.Start = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		}
		events = append(events, event)
	}
	return events
}

// itemURL is the link back into the product. Relative when the installation names no base URL,
// for the reason feedURL is.
func (c *RestController) itemURL(id string) string {
	if c.BaseURL == "" {
		return ""
	}
	return strings.TrimRight(c.BaseURL, "/") + "/items/" + id
}

// exportZone decides which zone the dates are written in: what the request named, else the
// caller's own, else UTC. A zone this installation does not know is refused rather than silently
// replaced - a file of timestamps in the wrong zone is worse than an error.
func exportZone(r *http.Request, named *string) (*time.Location, error) {
	wanted := ""
	if named != nil {
		wanted = *named
	}
	if wanted == "" {
		wanted = actorOf(r).TimeZone
	}
	if wanted == "" {
		return time.UTC, nil
	}

	zone, err := time.LoadLocation(wanted)
	if err != nil {
		return nil, shared.ErrValidation.
			WithDetail("views.export_zone_unknown").
			WithParams(map[string]string{"time_zone": wanted}).
			WithFields(shared.FieldError{Path: "/time_zone", Code: "views.export_zone_unknown"})
	}
	return zone, nil
}

func exportFormatUnknown(format string) error {
	return shared.ErrValidation.
		WithDetail("views.export_format_unknown").
		WithParams(map[string]string{"value": format}).
		WithFields(shared.FieldError{Path: "/format", Code: "views.export_format_unknown"})
}

// reportExportFailure logs a body that could not be written. The status is already on the wire,
// so there is nothing left to tell the client - but a file that stopped halfway is a defect
// rather than a network event.
func reportExportFailure(r *http.Request, err error) {
	slog.WarnContext(r.Context(), "writing the export body failed",
		slog.String("error", err.Error()))
}
