// SPDX-License-Identifier: Apache-2.0
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
	"github.com/Jersyfi/hubtask/presentation/calendar"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The view export. The use case selects the rows and caps them; which file they become is
// decided here, because a wire format is an adapter's business - and the ICS rendering is the same
// package the calendar feed serves from, which is why the export rides that task.

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

// exportViewWriter writes a view's export in the format the body names (`x-hubtask-rest-writer`),
// chosen before the use case runs: a format or a zone it cannot write is refused before anything
// is read or audited.
func (c *RestController) exportViewWriter(r *http.Request, sent map[string]any) (outputWriter, error) {
	format, zone, err := exportChoice(r, sent)
	if err != nil {
		return nil, err
	}
	return func(w http.ResponseWriter, r *http.Request, out usecase.Output) {
		rows := exportRows(w, out)
		switch format {
		case string(openapi.ViewExportFormatJSON):
			writeJSON(w, r, http.StatusOK, openapi.ViewExportDocument{
				ViewId:      uuidValue(out.String("view_id")),
				GeneratedAt: timeValue(out["generated_at"]),
				Count:       out.Int("count"),
				Truncated:   out["truncated"] == true,
				Rows:        workItemResponses(rows),
			})
		case string(openapi.ViewExportFormatCSV):
			writeCSV(w, r, rows, zone)
		default:
			writeCalendar(w, r, calendar.Calendar{
				Name:        out.String("view_name"),
				GeneratedAt: timeValue(out["generated_at"]),
				Events:      c.eventsOf(rows, zone),
			})
		}
	}, nil
}

// exportItemsWriter writes a query's result as a file without a saved view, as the view's
// export writes it.
func (c *RestController) exportItemsWriter(r *http.Request, sent map[string]any) (outputWriter, error) {
	format, zone, err := exportChoice(r, sent)
	if err != nil {
		return nil, err
	}
	return func(w http.ResponseWriter, r *http.Request, out usecase.Output) {
		rows := exportRows(w, out)
		switch format {
		case string(openapi.ItemExportFormatJSON):
			writeJSON(w, r, http.StatusOK, openapi.ItemExportDocument{
				GeneratedAt: timeValue(out["generated_at"]),
				Count:       out.Int("count"),
				Truncated:   out["truncated"] == true,
				Rows:        workItemResponses(rows),
			})
		case string(openapi.ItemExportFormatCSV):
			writeCSV(w, r, rows, zone)
		default:
			writeCalendar(w, r, calendar.Calendar{
				GeneratedAt: timeValue(out["generated_at"]),
				Events:      c.eventsOf(rows, zone),
			})
		}
	}, nil
}

// exportChoice reads the format and the zone an export is written in, and refuses either where it
// is none this adapter writes.
func exportChoice(r *http.Request, sent map[string]any) (string, *time.Location, error) {
	format, _ := sent["format"].(string)
	switch format {
	case "JSON", "CSV", "ICS":
	default:
		return "", nil, exportFormatUnknown(format)
	}
	var named *string
	if text, ok := sent["time_zone"].(string); ok {
		named = &text
	}
	zone, err := exportZone(r, named)
	return format, zone, err
}

// exportRows reads the rows and says on the answer whether they are the first of a longer one.
func exportRows(w http.ResponseWriter, out usecase.Output) []usecase.Output {
	rows, _ := out["rows"].([]usecase.Output)
	if truncated, _ := out["truncated"].(bool); truncated {
		w.Header().Set(exportTruncatedHeader, "true")
	}
	return rows
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
			URL:          itemURL(c.BaseURL, id),
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
func itemURL(base, id string) string {
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/items/" + id
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
