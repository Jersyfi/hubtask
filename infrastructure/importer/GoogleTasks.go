// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"time"

	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// GoogleTasks is Takeout's `Tasks.json` (P-10): every task list of the account, each with its
// items. One collection per list, one entry per item, a child under its parent - Google Tasks
// nests one level, which is a task under a task here. A due date is a date and never a time, so
// it lands as an all-day date in the importing person's zone; `completed` is the moment, and an
// item Google hid after completion is still an item. A deleted one is not.
//
// Takeout carries no account identifier a converter may use, so identities derive from the
// lists' and items' own identifiers, which Google keeps stable across exports.
type GoogleTasks struct{}

var _ service.Converter = GoogleTasks{}

func (GoogleTasks) Kind() domain.Kind { return domain.KindGoogleTasks }

type googleTakeout struct {
	Kind  string           `json:"kind"`
	Items []googleTaskList `json:"items"`
}

type googleTaskList struct {
	Kind  string       `json:"kind"`
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Items []googleTask `json:"items"`
}

type googleTask struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Notes     string `json:"notes"`
	Status    string `json:"status"`
	Due       string `json:"due"`
	Completed string `json:"completed"`
	Updated   string `json:"updated"`
	Parent    string `json:"parent"`
	Position  string `json:"position"`
	Deleted   bool   `json:"deleted"`
	Links     []struct {
		Link        string `json:"link"`
		Description string `json:"description"`
	} `json:"links"`
}

func (GoogleTasks) Convert(_ context.Context, source service.Source) (service.Result, error) {
	raw, err := io.ReadAll(source.Content)
	if err != nil {
		return service.Result{}, err
	}
	var takeout googleTakeout
	if err := json.Unmarshal(raw, &takeout); err != nil || !isGoogleTakeout(takeout) {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}

	b := newBuilder(source, "google-tasks")
	zone := zoneOf(source.Zone)
	var refused []refusal
	row := 0
	for _, list := range takeout.Items {
		name := strings.TrimSpace(list.Title)
		if name == "" {
			name = "Tasks"
		}
		collection := b.collection("list:"+list.ID, name, "")

		// Parents before children, each level in its position order; a child whose parent is
		// missing or refused lands at the top rather than nowhere.
		items := append([]googleTask{}, list.Items...)
		sort.SliceStable(items, func(i, j int) bool {
			if (items[i].Parent == "") != (items[j].Parent == "") {
				return items[i].Parent == ""
			}
			return items[i].Position < items[j].Position
		})
		for _, task := range items {
			row++
			if task.Deleted {
				continue
			}
			if strings.TrimSpace(task.Title) == "" {
				refused = append(refused, refusal{row: row, code: domain.CodeRowTitleMissing})
				continue
			}
			item := Item{Key: "task:" + task.ID, Collection: collection, Title: task.Title, Notes: googleNotes(task)}
			if task.Parent != "" {
				if _, known := b.entries["task:"+task.Parent]; known {
					// A subtask is the work under its task: Google nests one level, and a `TASK`
					// with a parent is what the model refuses (I-W1) - the e2e's first import found
					// the row rejected for exactly that.
					item.ParentKey, item.Type = "task:"+task.Parent, "WORK_PACKAGE"
				}
			}
			if task.Due != "" {
				at, err := time.Parse(time.RFC3339, task.Due)
				if err != nil {
					refused = append(refused, refusal{row: row, code: domain.CodeRowDateInvalid})
					continue
				}
				// The date Google wrote at midnight UTC, as that day in the person's zone.
				day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, zone).UTC()
				item.DueAt, item.DueDateOnly, item.DueZone = &day, true, zone.String()
			}
			if task.Status == "completed" {
				done := source.Now
				if at, err := time.Parse(time.RFC3339, task.Completed); err == nil {
					done = at
				}
				item.CompletedAt = &done
			}
			if at, err := time.Parse(time.RFC3339, task.Updated); err == nil {
				item.CreatedAt = &at
			}
			if _, ok := b.item(item); !ok {
				refused = append(refused, refusal{row: row, code: domain.CodeRowUnreadable})
			}
		}
	}
	return b.result(refused), nil
}

// isGoogleTakeout recognises the file by its own name for itself: `tasks#taskLists` at the top,
// or lists that each say `tasks#taskList`. A file of another shape is refused rather than landing
// as nothing.
func isGoogleTakeout(takeout googleTakeout) bool {
	if takeout.Kind == "tasks#taskLists" {
		return true
	}
	if len(takeout.Items) == 0 {
		return false
	}
	for _, list := range takeout.Items {
		if list.Kind != "tasks#taskList" {
			return false
		}
	}
	return true
}

// googleNotes is the notes, and the links Google attached as lines beneath them.
func googleNotes(task googleTask) string {
	notes := strings.TrimSpace(task.Notes)
	var lines []string
	for _, link := range task.Links {
		if link.Link == "" {
			continue
		}
		if link.Description != "" && link.Description != link.Link {
			lines = append(lines, link.Description+": "+link.Link)
		} else {
			lines = append(lines, link.Link)
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
