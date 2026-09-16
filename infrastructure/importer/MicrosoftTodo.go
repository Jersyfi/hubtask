// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MicrosoftTodo is the Graph API's JSON (P-10, decision 8): the `todoTaskList` collection as
// `GET /me/todo/lists` answers it, each list carrying the `todoTask` items its own `/tasks`
// request answered, attached under `tasks` by the person or the script that fetched both. One
// collection per list, an entry per task with its due date read in the zone Graph names, a
// reminder where one was set, a child per checklist item, a linked resource as a line in the
// notes. `importance` is recorded as nothing: the product has no priority (`ai-first.md` §2).
//
// Graph's zone names are Windows names, not IANA ones; `windowsZones` carries the mapping, and a
// name the table does not know refuses the row rather than guessing a zone.
type MicrosoftTodo struct{}

var _ service.Converter = MicrosoftTodo{}

func (MicrosoftTodo) Kind() domain.Kind { return domain.KindMicrosoftTodo }

type graphLists struct {
	Value []graphList `json:"value"`
	Lists []graphList `json:"lists"`
}

type graphList struct {
	ID          string          `json:"id"`
	DisplayName string          `json:"displayName"`
	WellKnown   string          `json:"wellknownListName"`
	Tasks       json.RawMessage `json:"tasks"`
}

type graphTask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Body   struct {
		Content     string `json:"content"`
		ContentType string `json:"contentType"`
	} `json:"body"`
	IsReminderOn      bool           `json:"isReminderOn"`
	ReminderDateTime  *graphDateTime `json:"reminderDateTime"`
	DueDateTime       *graphDateTime `json:"dueDateTime"`
	CompletedDateTime *graphDateTime `json:"completedDateTime"`
	CreatedDateTime   string         `json:"createdDateTime"`
	ChecklistItems    []struct {
		ID              string `json:"id"`
		DisplayName     string `json:"displayName"`
		IsChecked       bool   `json:"isChecked"`
		CheckedDateTime string `json:"checkedDateTime"`
	} `json:"checklistItems"`
	LinkedResources []struct {
		WebURL          string `json:"webUrl"`
		ApplicationName string `json:"applicationName"`
		DisplayName     string `json:"displayName"`
	} `json:"linkedResources"`
}

// graphDateTime is Graph's `dateTimeTimeZone`: a wall-clock time and the zone it is read in.
type graphDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// instant reads the wall-clock time in its zone. The second answer says whether the time was
// midnight - which is how Graph writes a due date that is a date.
func (d graphDateTime) instant() (at time.Time, midnight bool, err error) {
	zone, known := zoneNamed(d.TimeZone)
	if !known {
		return time.Time{}, false, errZoneUnknown
	}
	text := strings.TrimSuffix(d.DateTime, "Z")
	var parsed time.Time
	for _, layout := range []string{"2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if parsed, err = time.ParseInLocation(layout, text, zone); err == nil {
			break
		}
	}
	if err != nil {
		return time.Time{}, false, err
	}
	h, m, s := parsed.Clock()
	return parsed.UTC(), h == 0 && m == 0 && s == 0, nil
}

var errZoneUnknown = errors.New("zone unknown")

func (MicrosoftTodo) Convert(_ context.Context, source service.Source) (service.Result, error) {
	raw, err := io.ReadAll(source.Content)
	if err != nil {
		return service.Result{}, err
	}
	var document graphLists
	if err := json.Unmarshal(raw, &document); err != nil {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}
	lists := document.Value
	if len(lists) == 0 {
		lists = document.Lists
	}
	if len(lists) == 0 {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}
	for _, list := range lists {
		if list.ID == "" || list.DisplayName == "" {
			return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
		}
	}

	b := newBuilder(source, "microsoft-todo")
	var refused []refusal
	row := 0
	for _, list := range lists {
		collection := b.collection("list:"+list.ID, list.DisplayName, "")
		tasks, err := graphTasksOf(list.Tasks)
		if err != nil {
			return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
		}
		for _, task := range tasks {
			row++
			if strings.TrimSpace(task.Title) == "" {
				refused = append(refused, refusal{row: row, code: domain.CodeRowTitleMissing})
				continue
			}
			item := Item{Key: "task:" + task.ID, Collection: collection, Title: task.Title, Notes: graphNotes(task)}
			if task.DueDateTime != nil && task.DueDateTime.DateTime != "" {
				at, midnight, err := task.DueDateTime.instant()
				if err != nil {
					refused = append(refused, refusal{row: row, code: dateCode(err)})
					continue
				}
				item.DueAt, item.DueDateOnly = &at, midnight
				if midnight {
					if zone, _ := zoneNamed(task.DueDateTime.TimeZone); zone != nil {
						item.DueZone = zone.String()
					}
				}
			}
			if task.Status == "completed" {
				done := source.Now
				if task.CompletedDateTime != nil {
					if at, _, err := task.CompletedDateTime.instant(); err == nil {
						done = at
					}
				}
				item.CompletedAt = &done
			}
			if at, err := time.Parse(time.RFC3339, task.CreatedDateTime); err == nil {
				item.CreatedAt = &at
			}
			id, ok := b.item(item)
			if !ok {
				refused = append(refused, refusal{row: row, code: domain.CodeRowUnreadable})
				continue
			}
			if task.IsReminderOn && task.ReminderDateTime != nil && task.ReminderDateTime.DateTime != "" {
				if at, _, err := task.ReminderDateTime.instant(); err == nil {
					b.reminder("reminder:"+task.ID, id, at)
				} else {
					refused = append(refused, refusal{row: row, code: dateCode(err)})
				}
			}
			for _, check := range task.ChecklistItems {
				step := Item{Key: "check:" + check.ID, ParentKey: "task:" + task.ID, Collection: collection, Type: "ACTIVITY", Title: check.DisplayName}
				if check.IsChecked {
					done := source.Now
					if at, err := time.Parse(time.RFC3339, check.CheckedDateTime); err == nil {
						done = at
					}
					step.CompletedAt = &done
				}
				b.item(step)
			}
		}
	}
	return b.result(refused), nil
}

// graphTasksOf reads a list's tasks as the array the person attached, or the `{"value": [...]}`
// document the `/tasks` request answered, pasted in whole.
func graphTasksOf(raw json.RawMessage) ([]graphTask, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var tasks []graphTask
	if err := json.Unmarshal(raw, &tasks); err == nil {
		return tasks, nil
	}
	var page struct {
		Value []graphTask `json:"value"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	return page.Value, nil
}

func dateCode(err error) string {
	if errors.Is(err, errZoneUnknown) {
		return domain.CodeRowZoneUnknown
	}
	return domain.CodeRowDateInvalid
}

// graphNotes is the body, and the linked resources as lines beneath it.
func graphNotes(task graphTask) string {
	notes := strings.TrimSpace(task.Body.Content)
	var lines []string
	for _, linked := range task.LinkedResources {
		if linked.WebURL == "" {
			continue
		}
		name := linked.DisplayName
		if name == "" {
			name = linked.ApplicationName
		}
		if name != "" && name != linked.WebURL {
			lines = append(lines, name+": "+linked.WebURL)
		} else {
			lines = append(lines, linked.WebURL)
		}
	}
	if len(lines) == 0 {
		return notes
	}
	if notes != "" {
		notes += "\n\n"
	}
	return notes + strings.Join(lines, "\n")
}
