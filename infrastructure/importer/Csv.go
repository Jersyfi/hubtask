// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CSV is the simplest kind and the one backup-restore.md §9 names first: a header row, one entry
// per line, the columns found by name - `title`, `notes`, `due`, `completed`, `labels`, `bucket`,
// `parent` - or by the mapping the request declares where the header says something else. One
// collection, named after the file, under the hub; a bucket per distinct `bucket` value; a label
// per distinct name in `labels` (separated by `;` or `|`); a parent by the parent row's title or
// its number, which has to come first.
type CSV struct{}

var _ service.Converter = CSV{}

func (CSV) Kind() domain.Kind { return domain.KindCSV }

// The fields a column can carry, and the header names taken as them without a mapping.
var csvFields = map[string]string{
	"title": "title", "name": "title", "summary": "title", "subject": "title",
	"notes": "notes", "description": "notes", "body": "notes", "details": "notes",
	"due": "due", "due_date": "due", "due date": "due", "deadline": "due", "due_at": "due",
	"completed": "completed", "done": "completed", "status": "completed", "is_completed": "completed",
	"labels": "labels", "tags": "labels", "label": "labels",
	"bucket": "bucket", "list": "bucket", "column": "bucket", "state": "bucket",
	"parent":     "parent",
	"collection": "collection", "project": "collection", "board": "collection",
}

// refusal is one row the converter could not read.
type refusal struct {
	row  int
	code string
}

func (r refusal) refusal() domain.Refusal { return domain.Refusal{Row: r.row, Code: r.code} }

func (CSV) Convert(_ context.Context, source service.Source) (service.Result, error) {
	raw, err := io.ReadAll(source.Content)
	if err != nil {
		return service.Result{}, err
	}
	if !utf8.Valid(raw) {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeEncodingInvalid)
	}
	raw = []byte(strings.TrimPrefix(string(raw), "\ufeff"))
	if len(strings.TrimSpace(string(raw))) == 0 {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileEmpty)
	}

	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	if strings.Count(firstLine(string(raw)), ";") > strings.Count(firstLine(string(raw)), ",") {
		// A spreadsheet in a German locale writes semicolons; the header says which.
		reader.Comma = ';'
	}
	header, err := reader.Read()
	if err != nil {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}
	columns, err := csvColumns(header, source.Mapping)
	if err != nil {
		return service.Result{}, err
	}
	if _, ok := columns["title"]; !ok {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind).
			WithFields(shared.FieldError{Path: "/mapping/title", Code: domain.CodeRowTitleMissing})
	}

	b := newBuilder(source, source.Digest)
	var refused []refusal
	collections := map[string]shared.ID{}
	buckets := map[string]shared.ID{}
	labels := map[string]shared.ID{}
	zone := zoneOf(source.Zone)
	rowNumber := 1
	for {
		record, err := reader.Read()
		rowNumber++
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			refused = append(refused, refusal{row: rowNumber, code: domain.CodeRowUnreadable})
			continue
		}
		cell := func(field string) string {
			index, ok := columns[field]
			if !ok || index >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		title := cell("title")
		if title == "" {
			if strings.TrimSpace(strings.Join(record, "")) == "" {
				continue // a blank line is not a row
			}
			refused = append(refused, refusal{row: rowNumber, code: domain.CodeRowTitleMissing})
			continue
		}

		collectionName := cell("collection")
		if collectionName == "" {
			collectionName = "Imported"
		}
		collection, ok := collections[collectionName]
		if !ok {
			collection = b.collection("collection:"+collectionName, collectionName, "")
			collections[collectionName] = collection
		}

		item := Item{Key: "row:" + itoa(rowNumber), Collection: collection, Title: title, Notes: cell("notes")}
		if bucketName := cell("bucket"); bucketName != "" {
			key := collectionName + "/" + bucketName
			bucket, ok := buckets[key]
			if !ok {
				bucket = b.bucket("bucket:"+key, collection, bucketName, isDoneWord(bucketName))
				buckets[key] = bucket
			}
			item.Bucket = bucket
		}
		if due := cell("due"); due != "" {
			at, dateOnly, err := parseDue(due, zone)
			if err != nil {
				refused = append(refused, refusal{row: rowNumber, code: domain.CodeRowDateInvalid})
				continue
			}
			item.DueAt, item.DueDateOnly = &at, dateOnly
			if dateOnly {
				item.DueZone = source.Zone
			}
		}
		if isTrue(cell("completed")) {
			at := source.Now
			item.CompletedAt = &at
		}
		if parent := cell("parent"); parent != "" {
			key, ok := csvParentKey(parent, b)
			if !ok {
				refused = append(refused, refusal{row: rowNumber, code: domain.CodeRowParentUnknown})
				continue
			}
			item.ParentKey = key
		}
		id, ok := b.item(item)
		if !ok {
			refused = append(refused, refusal{row: rowNumber, code: domain.CodeRowUnreadable})
			continue
		}
		b.titles(title, item.Key)
		for _, name := range splitLabels(cell("labels")) {
			key := collectionName + "/" + name
			label, ok := labels[key]
			if !ok {
				label = b.label("label:"+key, collection, name, tokenFor(name))
				labels[key] = label
			}
			b.link(id, label)
		}
	}
	return b.result(refused), nil
}

// csvColumns maps each field to its column index: the mapping first, then the header's own
// names. A mapping that names a header the file does not have is refused by name.
func csvColumns(header []string, mapping map[string]string) (map[string]int, error) {
	index := map[string]int{}
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\ufeff")))] = i
	}
	columns := map[string]int{}
	for i, name := range header {
		if field, ok := csvFields[strings.ToLower(strings.TrimSpace(name))]; ok {
			if _, taken := columns[field]; !taken {
				columns[field] = i
			}
		}
	}
	for field, name := range mapping {
		if _, known := csvFields[field]; !known || csvFields[field] != field {
			return nil, shared.ErrValidation.WithDetail(domain.CodeMappingUnknown).
				WithParams(map[string]string{"field": field}).
				WithFields(shared.FieldError{Path: "/mapping/" + field, Code: domain.CodeMappingUnknown})
		}
		i, ok := index[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return nil, shared.ErrValidation.WithDetail(domain.CodeMappingUnknown).
				WithParams(map[string]string{"field": field, "column": name}).
				WithFields(shared.FieldError{Path: "/mapping/" + field, Code: domain.CodeMappingUnknown})
		}
		columns[field] = i
	}
	return columns, nil
}

// csvParentKey resolves a parent cell: a row number, or an earlier row's title.
func csvParentKey(parent string, b *builder) (string, bool) {
	if n := atoi(parent); n > 1 {
		if _, ok := b.entries["row:"+parent]; ok {
			return "row:" + parent, true
		}
	}
	key, ok := b.byTitle[strings.ToLower(parent)]
	return key, ok
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func splitLabels(cell string) []string {
	if cell == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(cell, func(r rune) bool { return r == ';' || r == '|' }) {
		name := strings.TrimSpace(part)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}

func isTrue(cell string) bool {
	switch strings.ToLower(strings.TrimSpace(cell)) {
	case "1", "true", "yes", "y", "x", "done", "completed", "complete", "ja", "erledigt", "closed":
		return true
	}
	return false
}

func isDoneWord(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "done", "completed", "complete", "erledigt", "fertig", "closed":
		return true
	}
	return false
}

// parseDue reads the shapes a spreadsheet writes: RFC 3339, a date, a date with a time, and the
// two locales' day-first forms. A date without a time is an all-day due date.
func parseDue(cell string, zone *time.Location) (time.Time, bool, error) {
	cell = strings.TrimSpace(cell)
	if at, err := time.Parse(time.RFC3339, cell); err == nil {
		return at.UTC(), false, nil
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02 15:04:05", "02.01.2006 15:04", "01/02/2006 15:04"} {
		if at, err := time.ParseInLocation(layout, cell, zone); err == nil {
			return at.UTC(), false, nil
		}
	}
	for _, layout := range []string{"2006-01-02", "02.01.2006", "01/02/2006", "2006/01/02"} {
		if at, err := time.ParseInLocation(layout, cell, zone); err == nil {
			return at.UTC(), true, nil
		}
	}
	return time.Time{}, false, errors.New("unparseable date")
}

func zoneOf(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	if loaded, err := time.LoadLocation(name); err == nil {
		return loaded
	}
	return time.UTC
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
