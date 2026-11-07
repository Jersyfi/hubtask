// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package spreadsheet reads the first sheet of an Office Open XML workbook (ECMA-376) into rows of
// text, the shape a CSV file has, so a register kept in a spreadsheet is imported through the same
// mapping and checks as its CSV export. Standard library only, as the XLSX export is written.
package spreadsheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
	"time"
)

// Limits bound what a workbook may make the reader do: a zip expands, and a cell reference can
// name a column far to the right of every other.
const (
	// MaxPartBytes is the most one part of the package may expand to.
	MaxPartBytes = 64 << 20
	// MaxRows and MaxColumns bound the table answered.
	MaxRows    = 100_000
	MaxColumns = 1_000
)

// ErrUnreadable is a file that is no workbook this reader can read, or one past the limits.
var ErrUnreadable = errors.New("spreadsheet: not a readable workbook")

// Rows answers the first sheet's rows, the first one usually the header. A missing row is an empty
// one, so a row's index is its number in the sheet less one; a missing cell is an empty text. A
// number is written as the sheet stores it, never in a locale's format; a cell styled as a date is
// written as 2006-01-02, or 2006-01-02T15:04:05 where it carries a time of day; a boolean as true
// or false; a formula as its last computed value.
func Rows(data []byte) ([][]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrUnreadable
	}
	parts := map[string]*zip.File{}
	for _, f := range archive.File {
		parts[strings.TrimPrefix(f.Name, "/")] = f
	}

	sheetPart, date1904, err := firstSheet(parts)
	if err != nil {
		return nil, err
	}
	shared, err := sharedStrings(parts)
	if err != nil {
		return nil, err
	}
	dates, err := dateStyles(parts)
	if err != nil {
		return nil, err
	}
	sheet, ok := parts[sheetPart]
	if !ok {
		return nil, ErrUnreadable
	}
	body, err := readPart(sheet)
	if err != nil {
		return nil, err
	}
	return sheetRows(body, shared, dates, date1904)
}

func readPart(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, ErrUnreadable
	}
	defer func() { _ = r.Close() }()
	body, err := io.ReadAll(io.LimitReader(r, MaxPartBytes+1))
	if err != nil || len(body) > MaxPartBytes {
		return nil, ErrUnreadable
	}
	return body, nil
}

func decodePart(parts map[string]*zip.File, name string, into any) (bool, error) {
	f, ok := parts[name]
	if !ok {
		return false, nil
	}
	body, err := readPart(f)
	if err != nil {
		return true, err
	}
	if err := xml.Unmarshal(body, into); err != nil {
		return true, ErrUnreadable
	}
	return true, nil
}

// firstSheet follows the workbook to its first sheet's part, as a spreadsheet opens it.
func firstSheet(parts map[string]*zip.File) (string, bool, error) {
	var workbook struct {
		Properties struct {
			Date1904 string `xml:"date1904,attr"`
		} `xml:"workbookPr"`
		Sheets []struct {
			ID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	found, err := decodePart(parts, "xl/workbook.xml", &workbook)
	if err != nil || !found || len(workbook.Sheets) == 0 {
		return "", false, ErrUnreadable
	}
	date1904 := workbook.Properties.Date1904 == "1" || workbook.Properties.Date1904 == "true"

	var rels struct {
		Relationships []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if _, err := decodePart(parts, "xl/_rels/workbook.xml.rels", &rels); err != nil {
		return "", false, err
	}
	for _, rel := range rels.Relationships {
		if rel.ID != workbook.Sheets[0].ID {
			continue
		}
		target := rel.Target
		if strings.HasPrefix(target, "/") {
			return path.Clean(strings.TrimPrefix(target, "/")), date1904, nil
		}
		return path.Clean(path.Join("xl", target)), date1904, nil
	}
	return "xl/worksheets/sheet1.xml", date1904, nil
}

// text is a run of characters as a shared or inline string holds it: plain, or in rich-text runs.
type text struct {
	T    string `xml:"t"`
	Runs []struct {
		T string `xml:"t"`
	} `xml:"r"`
}

func (t text) String() string {
	if len(t.Runs) == 0 {
		return t.T
	}
	var out strings.Builder
	out.WriteString(t.T)
	for _, run := range t.Runs {
		out.WriteString(run.T)
	}
	return out.String()
}

func sharedStrings(parts map[string]*zip.File) ([]string, error) {
	var table struct {
		Items []text `xml:"si"`
	}
	if _, err := decodePart(parts, "xl/sharedStrings.xml", &table); err != nil {
		return nil, err
	}
	out := make([]string, len(table.Items))
	for i, item := range table.Items {
		out[i] = item.String()
	}
	return out, nil
}

// dateStyles answers, per cell style index, whether its number format shows a date.
func dateStyles(parts map[string]*zip.File) ([]bool, error) {
	var styles struct {
		Formats []struct {
			ID   int    `xml:"numFmtId,attr"`
			Code string `xml:"formatCode,attr"`
		} `xml:"numFmts>numFmt"`
		Cells []struct {
			Format int `xml:"numFmtId,attr"`
		} `xml:"cellXfs>xf"`
	}
	if _, err := decodePart(parts, "xl/styles.xml", &styles); err != nil {
		return nil, err
	}
	custom := map[int]bool{}
	for _, f := range styles.Formats {
		custom[f.ID] = dateCode(f.Code)
	}
	out := make([]bool, len(styles.Cells))
	for i, xf := range styles.Cells {
		if isDate, ok := custom[xf.Format]; ok {
			out[i] = isDate
			continue
		}
		out[i] = builtinDate(xf.Format)
	}
	return out, nil
}

// builtinDate is ECMA-376 §18.8.30's date and time formats, and the East Asian ones.
func builtinDate(id int) bool {
	return (id >= 14 && id <= 22) || (id >= 27 && id <= 36) || (id >= 45 && id <= 47) || (id >= 50 && id <= 58)
}

// dateCode reads a custom format as a date when it names a day, a month with a year, or a time,
// outside quoted text and brackets - "0.00" and "#,##0" are numbers, "dd.mm.yyyy" a date.
func dateCode(code string) bool {
	var plain strings.Builder
	quoted, bracket := false, false
	for i := 0; i < len(code); i++ {
		switch c := code[i]; {
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '\\':
			i++
		case c == '[':
			bracket = true
		case c == ']':
			bracket = false
		case bracket:
		default:
			plain.WriteByte(c)
		}
	}
	lower := strings.ToLower(plain.String())
	return strings.ContainsAny(lower, "dyhs")
}

func sheetRows(body []byte, shared []string, dates []bool, date1904 bool) ([][]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var rows [][]string
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, ErrUnreadable
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "row" {
			continue
		}
		var row struct {
			Number int `xml:"r,attr"`
			Cells  []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Style  int    `xml:"s,attr"`
				Value  string `xml:"v"`
				Inline text   `xml:"is"`
			} `xml:"c"`
		}
		if err := decoder.DecodeElement(&row, &start); err != nil {
			return nil, ErrUnreadable
		}
		number := row.Number
		if number == 0 {
			number = len(rows) + 1
		}
		if number < len(rows)+1 || number > MaxRows {
			return nil, ErrUnreadable
		}
		for len(rows) < number-1 {
			rows = append(rows, nil)
		}
		var cells []string
		for _, cell := range row.Cells {
			column := len(cells)
			if cell.Ref != "" {
				column, ok = columnOf(cell.Ref)
				if !ok || column < len(cells) {
					return nil, ErrUnreadable
				}
			}
			if column >= MaxColumns {
				return nil, ErrUnreadable
			}
			for len(cells) < column {
				cells = append(cells, "")
			}
			value, err := cellText(cell.Type, cell.Value, cell.Inline, cell.Style, shared, dates, date1904)
			if err != nil {
				return nil, err
			}
			cells = append(cells, value)
		}
		rows = append(rows, cells)
	}
}

func cellText(kind, value string, inline text, style int, shared []string, dates []bool, date1904 bool) (string, error) {
	switch kind {
	case "s":
		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || index < 0 || index >= len(shared) {
			return "", ErrUnreadable
		}
		return shared[index], nil
	case "inlineStr":
		return inline.String(), nil
	case "str", "e":
		return value, nil
	case "b":
		return strconv.FormatBool(strings.TrimSpace(value) == "1"), nil
	}
	if style >= 0 && style < len(dates) && dates[style] && value != "" {
		serial, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil {
			return serialDate(serial, date1904), nil
		}
	}
	return value, nil
}

// serialDate turns a spreadsheet's day number into a civil date, and a time of day where it has one.
// The 1900 system counts from 30 December 1899, which absorbs its fictitious 29 February 1900 for
// every date after it.
func serialDate(serial float64, date1904 bool) string {
	base := time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC)
	if date1904 {
		base = time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	if math.IsNaN(serial) || serial < 0 || serial > 2_958_465 { // 9999-12-31
		return strconv.FormatFloat(serial, 'f', -1, 64)
	}
	days := math.Floor(serial)
	seconds := math.Round((serial - days) * 86400)
	at := base.AddDate(0, 0, int(days)).Add(time.Duration(seconds) * time.Second)
	if seconds == 0 {
		return at.Format(time.DateOnly)
	}
	return at.Format("2006-01-02T15:04:05")
}

// columnOf reads the letters of a reference like "AB12" as a zero-based column.
func columnOf(ref string) (int, bool) {
	column, letters := 0, 0
	for _, r := range ref {
		if r >= 'A' && r <= 'Z' {
			column = column*26 + int(r-'A'+1)
			letters++
			if letters > 3 {
				return 0, false
			}
			continue
		}
		break
	}
	if letters == 0 {
		return 0, false
	}
	return column - 1, true
}
