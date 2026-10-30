// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// FileFormat is the kind of project file a plan is brought in from (PLN-09).
type FileFormat string

const (
	// FormatProjectXML is the XML a desktop project tool saves: tasks with their outline level,
	// dates, milestone mark and predecessor links.
	FormatProjectXML FileFormat = "MSPROJECT_XML"
	// FormatCSV is a table with a header row: an identifier, a name, a start, a finish, a
	// predecessors column in the usual notation (`3`, `3SS`, `4FF+2d`, `5FS-1d`, separated by `;`
	// or `,`), and optionally an outline level and a milestone column.
	FormatCSV FileFormat = "CSV"
)

// The bounds of a project file: what one import may bring in.
const (
	MaxFileBytes        = 512 << 10
	MaxImportedEntries  = 500
	MaxImportedLinks    = 2000
	maxImportedTitle    = 500
	minutesPerWorkday   = 8 * 60
	minutesPerDay       = 24 * 60
	tenthsPerMinute     = 10
	maxLagDays          = 999
	firstElapsedFormat  = 4
	lastKnownLagFormats = 20
)

// FileTask is one task of a project file: its key in the file, its name, its depth in the file's
// outline (1 is the top), its dates, whether it is a milestone, and what it waits on.
type FileTask struct {
	Key          string
	Name         string
	Level        int
	Start        *Day
	Finish       *Day
	Milestone    bool
	Predecessors []FilePredecessor
}

// FilePredecessor is one dependency of a task on another of the same file.
type FilePredecessor struct {
	Key    string
	Timing Timing
}

// ParseProjectFile reads a project file into its tasks, in the file's order. A task without a
// name, a predecessor the file does not hold, a date that is no date, or a file beyond the bounds
// is refused with the line or task it is on, so the person fixes the file rather than guessing.
func ParseProjectFile(format FileFormat, content []byte) ([]FileTask, error) {
	if len(content) == 0 {
		return nil, importInvalid("planning.import_empty", "")
	}
	if len(content) > MaxFileBytes {
		return nil, shared.ErrValidation.WithDetail("planning.import_too_large").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxFileBytes)})
	}
	if !utf8.Valid(content) {
		return nil, importInvalid("planning.import_not_utf8", "")
	}
	var (
		tasks []FileTask
		err   error
	)
	switch format {
	case FormatProjectXML:
		tasks, err = parseProjectXML(content)
	case FormatCSV:
		tasks, err = parseProjectCSV(content)
	default:
		return nil, shared.ErrValidation.WithDetail("planning.import_format_unknown").
			WithFields(shared.FieldError{Path: "/format", Code: "planning.import_format_unknown"})
	}
	if err != nil {
		return nil, err
	}
	return tasks, checkFileTasks(tasks)
}

func checkFileTasks(tasks []FileTask) error {
	if len(tasks) == 0 {
		return importInvalid("planning.import_empty", "")
	}
	if len(tasks) > MaxImportedEntries {
		return shared.ErrValidation.WithDetail("planning.import_too_many_entries").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxImportedEntries)})
	}
	keys := map[string]bool{}
	links := 0
	for _, t := range tasks {
		if keys[t.Key] {
			return importInvalid("planning.import_key_duplicate", t.Key)
		}
		keys[t.Key] = true
		links += len(t.Predecessors)
	}
	if links > MaxImportedLinks {
		return shared.ErrValidation.WithDetail("planning.import_too_many_links").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxImportedLinks)})
	}
	for _, t := range tasks {
		for _, p := range t.Predecessors {
			if !keys[p.Key] || p.Key == t.Key {
				return importInvalid("planning.import_predecessor_unknown", t.Key)
			}
		}
	}
	return nil
}

func importInvalid(code, at string) error {
	err := shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: "/content", Code: code})
	if at != "" {
		err = err.WithParams(map[string]string{"at": at})
	}
	return err
}

// projectXML is the part of a project tool's XML a plan takes; everything else is ignored.
type projectXML struct {
	Tasks []struct {
		UID          string `xml:"UID"`
		Name         string `xml:"Name"`
		OutlineLevel int    `xml:"OutlineLevel"`
		Start        string `xml:"Start"`
		Finish       string `xml:"Finish"`
		Milestone    string `xml:"Milestone"`
		Null         string `xml:"IsNull"`
		Links        []struct {
			PredecessorUID string `xml:"PredecessorUID"`
			Type           int    `xml:"Type"`
			LinkLag        int    `xml:"LinkLag"`
			LagFormat      int    `xml:"LagFormat"`
		} `xml:"PredecessorLink"`
	} `xml:"Tasks>Task"`
}

// xmlKinds is the tool's link type: 0 finish to finish, 1 finish to start, 2 start to finish, 3
// start to start.
var xmlKinds = map[int]DependencyKind{0: FinishToFinish, 1: FinishToStart, 2: StartToFinish, 3: StartToStart}

func parseProjectXML(content []byte) ([]FileTask, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	// The decoder resolves no external entity, and strict it refuses one it was not told of: a
	// project file is read, never fetched from.
	decoder.Strict = true
	var doc projectXML
	if err := decoder.Decode(&doc); err != nil {
		return nil, importInvalid("planning.import_not_xml", "")
	}
	tasks := make([]FileTask, 0, len(doc.Tasks))
	for _, t := range doc.Tasks {
		// Outline level 0 is the project's own summary task, and a null task an empty row.
		if t.OutlineLevel < 1 || t.Null == "1" {
			continue
		}
		task := FileTask{Key: strings.TrimSpace(t.UID), Name: strings.TrimSpace(t.Name), Level: t.OutlineLevel, Milestone: t.Milestone == "1"}
		if task.Key == "" || task.Name == "" || utf8.RuneCountInString(task.Name) > maxImportedTitle {
			return nil, importInvalid("planning.import_task_invalid", task.Key)
		}
		var err error
		if task.Start, err = fileDay(t.Start); err != nil {
			return nil, importInvalid("planning.import_date_invalid", task.Key)
		}
		if task.Finish, err = fileDay(t.Finish); err != nil {
			return nil, importInvalid("planning.import_date_invalid", task.Key)
		}
		for _, l := range t.Links {
			kind, ok := xmlKinds[l.Type]
			if !ok {
				return nil, importInvalid("planning.import_link_invalid", task.Key)
			}
			timing, err := NewTiming(kind, xmlLagDays(l.LinkLag, l.LagFormat))
			if err != nil {
				return nil, importInvalid("planning.import_link_invalid", task.Key)
			}
			task.Predecessors = append(task.Predecessors, FilePredecessor{Key: strings.TrimSpace(l.PredecessorUID), Timing: timing})
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// xmlLagDays reads a lag in tenths of a minute as working days: elapsed formats count whole days
// of the clock, the others working days of eight hours, rounded to the nearest.
func xmlLagDays(tenths, format int) int {
	per := float64(minutesPerWorkday * tenthsPerMinute)
	if format >= firstElapsedFormat && format <= lastKnownLagFormats && format%2 == 0 {
		per = float64(minutesPerDay * tenthsPerMinute)
	}
	days := int(math.Round(float64(tenths) / per))
	return max(min(days, maxLagDays), -maxLagDays)
}

// fileDay reads a date, or the date of a date and time as the file wrote it; empty is no date.
func fileDay(text string) (*Day, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if len(text) > len(time.DateOnly) {
		text = text[:len(time.DateOnly)]
	}
	at, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return nil, err
	}
	day := DayFrom(at.Year(), at.Month(), at.Day())
	return &day, nil
}

// csvColumns names the columns a CSV may carry, by the header's words in lower case.
var csvColumns = map[string]string{
	"id": "key", "key": "key", "uid": "key", "unique id": "key",
	"name": "name", "title": "name", "task name": "name",
	"start": "start", "begin": "start",
	"finish": "finish", "end": "finish", "due": "finish",
	"predecessors": "predecessors", "depends on": "predecessors",
	"outline level": "level", "level": "level",
	"milestone": "milestone",
}

func parseProjectCSV(content []byte) ([]FileTask, error) {
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	if first, _, _ := bytes.Cut(content, []byte("\n")); bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		reader.Comma = ';'
	}
	header, err := reader.Read()
	if err != nil {
		return nil, importInvalid("planning.import_not_csv", "")
	}
	at := map[string]int{}
	for i, word := range header {
		if column, ok := csvColumns[strings.ToLower(strings.TrimSpace(word))]; ok {
			if _, seen := at[column]; !seen {
				at[column] = i
			}
		}
	}
	if _, ok := at["name"]; !ok {
		return nil, importInvalid("planning.import_name_column_missing", "")
	}
	cell := func(row []string, column string) string {
		if i, ok := at[column]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var tasks []FileTask
	for line := 2; ; line++ {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		where := strconv.Itoa(line)
		if err != nil {
			return nil, importInvalid("planning.import_not_csv", where)
		}
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		task := FileTask{Key: cell(row, "key"), Name: cell(row, "name"), Level: 1}
		if task.Key == "" {
			task.Key = strconv.Itoa(len(tasks) + 1)
		}
		if task.Name == "" || utf8.RuneCountInString(task.Name) > maxImportedTitle {
			return nil, importInvalid("planning.import_task_invalid", where)
		}
		if level := cell(row, "level"); level != "" {
			if task.Level, err = strconv.Atoi(level); err != nil || task.Level < 1 {
				return nil, importInvalid("planning.import_task_invalid", where)
			}
		}
		switch strings.ToLower(cell(row, "milestone")) {
		case "1", "yes", "true", "x", "ja":
			task.Milestone = true
		}
		if task.Start, err = fileDay(cell(row, "start")); err != nil {
			return nil, importInvalid("planning.import_date_invalid", where)
		}
		if task.Finish, err = fileDay(cell(row, "finish")); err != nil {
			return nil, importInvalid("planning.import_date_invalid", where)
		}
		if task.Predecessors, err = predecessorsOf(cell(row, "predecessors")); err != nil {
			return nil, importInvalid("planning.import_link_invalid", where)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// predecessorsOf reads `3`, `3SS`, `4FF+2d`, `5FS-1 d`, separated by `;` or `,`.
func predecessorsOf(text string) ([]FilePredecessor, error) {
	var out []FilePredecessor
	for _, part := range strings.FieldsFunc(text, func(r rune) bool { return r == ';' || r == ',' }) {
		part = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(part), " ", ""))
		if part == "" {
			continue
		}
		key := part
		kind := FinishToStart
		lag := 0
		if i := strings.IndexAny(part, "FS"); i > 0 && i+2 <= len(part) {
			key, kind = part[:i], DependencyKind(part[i:i+2])
			rest := strings.TrimSuffix(strings.TrimSuffix(part[i+2:], "D"), "T")
			if rest != "" {
				n, err := strconv.Atoi(rest)
				if err != nil {
					return nil, err
				}
				lag = n
			}
		}
		timing, err := NewTiming(kind, lag)
		if err != nil {
			return nil, err
		}
		out = append(out, FilePredecessor{Key: key, Timing: timing})
	}
	return out, nil
}
