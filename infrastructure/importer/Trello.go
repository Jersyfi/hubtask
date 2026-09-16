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

// Trello is a board's JSON export (P-09): one document with the board, its lists, its cards,
// its labels, its checklists and its actions.
//
// The mapping is the one the two products' shapes suggest and nothing cleverer: the board is a
// collection under the hub, its lists are buckets, its cards are entries with their description
// as notes, their due date and whether it was met, their labels mapped onto the ten tokens, and
// a card's checklists are the work under it - one work package per checklist where there are
// several, and an activity per check item. A closed card lands archived rather than dropped,
// because a person who archived it in Trello did not delete it. Comments become comments by the
// importing person with the commenter's name in the text, because a Trello member is not an
// account here; members and their assignments are not mapped, and the report counts them.
// Attachments are addresses Trello serves behind its own sign-in, and the product makes no call
// on anybody's behalf, so each becomes a line in the notes rather than bytes.
//
// Identities derive from the board's own identifier, so that a fresh export of the same board
// into the same hub lands on the rows the first one made and creates nothing twice.
type Trello struct{}

var _ service.Converter = Trello{}

func (Trello) Kind() domain.Kind { return domain.KindTrello }

type trelloBoard struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Desc       string            `json:"desc"`
	Lists      []trelloList      `json:"lists"`
	Cards      []trelloCard      `json:"cards"`
	Labels     []trelloLabel     `json:"labels"`
	Checklists []trelloChecklist `json:"checklists"`
	Actions    []trelloAction    `json:"actions"`
	Members    []trelloMember    `json:"members"`
}

type trelloList struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Closed bool    `json:"closed"`
	Pos    float64 `json:"pos"`
}

type trelloCard struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Desc         string             `json:"desc"`
	IDList       string             `json:"idList"`
	Closed       bool               `json:"closed"`
	Due          *string            `json:"due"`
	DueComplete  bool               `json:"dueComplete"`
	IDLabels     []string           `json:"idLabels"`
	Labels       []trelloLabel      `json:"labels"`
	IDChecklists []string           `json:"idChecklists"`
	IDMembers    []string           `json:"idMembers"`
	Pos          float64            `json:"pos"`
	LastActivity *string            `json:"dateLastActivity"`
	Attachments  []trelloAttachment `json:"attachments"`
}

type trelloLabel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type trelloChecklist struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	IDCard     string            `json:"idCard"`
	Pos        float64           `json:"pos"`
	CheckItems []trelloCheckItem `json:"checkItems"`
}

type trelloCheckItem struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	State string  `json:"state"`
	Pos   float64 `json:"pos"`
}

type trelloAction struct {
	Type          string `json:"type"`
	Date          string `json:"date"`
	MemberCreator struct {
		FullName string `json:"fullName"`
		Username string `json:"username"`
	} `json:"memberCreator"`
	Data struct {
		Text string `json:"text"`
		Card struct {
			ID string `json:"id"`
		} `json:"card"`
	} `json:"data"`
}

type trelloMember struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
}

type trelloAttachment struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

func (Trello) Convert(_ context.Context, source service.Source) (service.Result, error) {
	raw, err := io.ReadAll(source.Content)
	if err != nil {
		return service.Result{}, err
	}
	var board trelloBoard
	if err := json.Unmarshal(raw, &board); err != nil || board.ID == "" || (len(board.Lists) == 0 && len(board.Cards) == 0) {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}

	b := newBuilder(source, "trello:"+board.ID)
	collection := b.collection("board:"+board.ID, board.Name, board.Desc)

	// Lists in their order, closed ones too: a card in a closed list still names it.
	lists := append([]trelloList{}, board.Lists...)
	sort.SliceStable(lists, func(i, j int) bool { return lists[i].Pos < lists[j].Pos })
	buckets := map[string]shared.ID{}
	for _, list := range lists {
		buckets[list.ID] = b.bucket("list:"+list.ID, collection, list.Name, isDoneWord(list.Name))
	}

	labels := map[string]shared.ID{}
	labelFor := func(label trelloLabel) shared.ID {
		if id, ok := labels[label.ID]; ok {
			return id
		}
		name := label.Name
		if strings.TrimSpace(name) == "" {
			name = strings.TrimSpace(label.Color)
		}
		if name == "" {
			name = "Label"
		}
		id := b.label("label:"+label.ID, collection, name, tokenNamed(label.Color, name))
		labels[label.ID] = id
		return id
	}
	for _, label := range board.Labels {
		labelFor(label)
	}
	boardLabels := map[string]trelloLabel{}
	for _, label := range board.Labels {
		boardLabels[label.ID] = label
	}

	checklists := map[string][]trelloChecklist{}
	for _, list := range board.Checklists {
		checklists[list.IDCard] = append(checklists[list.IDCard], list)
	}
	comments := map[string][]trelloAction{}
	for _, action := range board.Actions {
		if action.Type == "commentCard" && action.Data.Card.ID != "" {
			comments[action.Data.Card.ID] = append(comments[action.Data.Card.ID], action)
		}
	}
	members := map[string]string{}
	for _, member := range board.Members {
		members[member.ID] = member.FullName
	}

	cards := append([]trelloCard{}, board.Cards...)
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].Pos < cards[j].Pos })
	var refused []refusal
	unmapped := 0
	for index, card := range cards {
		if strings.TrimSpace(card.Name) == "" {
			refused = append(refused, refusal{row: index + 1, code: domain.CodeRowTitleMissing})
			continue
		}
		item := Item{Key: "card:" + card.ID, Collection: collection, Title: card.Name, Notes: trelloNotes(card)}
		if bucket, ok := buckets[card.IDList]; ok {
			item.Bucket = bucket
		}
		if card.Due != nil && *card.Due != "" {
			at, err := time.Parse(time.RFC3339, *card.Due)
			if err != nil {
				refused = append(refused, refusal{row: index + 1, code: domain.CodeRowDateInvalid})
				continue
			}
			item.DueAt = &at
			if card.DueComplete {
				done := at
				if card.LastActivity != nil {
					if last, err := time.Parse(time.RFC3339, *card.LastActivity); err == nil {
						done = last
					}
				}
				item.CompletedAt = &done
			}
		}
		if card.Closed {
			archived := source.Now
			item.ArchivedAt = &archived
		}
		id, ok := b.item(item)
		if !ok {
			refused = append(refused, refusal{row: index + 1, code: domain.CodeRowUnreadable})
			continue
		}
		unmapped += len(card.IDMembers)

		// The labels: the card's own list first, the board's by identifier where the card names
		// only identifiers.
		seen := map[string]bool{}
		for _, label := range card.Labels {
			if !seen[label.ID] {
				seen[label.ID] = true
				b.link(id, labelFor(label))
			}
		}
		for _, labelID := range card.IDLabels {
			if label, ok := boardLabels[labelID]; ok && !seen[labelID] {
				seen[labelID] = true
				b.link(id, labelFor(label))
			}
		}

		// The checklists: one work package per checklist where there are several, the items
		// straight under the card where there is one - a single checklist is the card's steps.
		lists := checklists[card.ID]
		sort.SliceStable(lists, func(i, j int) bool { return lists[i].Pos < lists[j].Pos })
		for _, list := range lists {
			parentKey := "card:" + card.ID
			if len(lists) > 1 {
				if _, ok := b.item(Item{Key: "checklist:" + list.ID, ParentKey: parentKey, Collection: collection, Type: "WORK_PACKAGE", Title: list.Name}); ok {
					parentKey = "checklist:" + list.ID
				}
			}
			items := append([]trelloCheckItem{}, list.CheckItems...)
			sort.SliceStable(items, func(i, j int) bool { return items[i].Pos < items[j].Pos })
			for _, check := range items {
				step := Item{Key: "check:" + check.ID, ParentKey: parentKey, Collection: collection, Type: "ACTIVITY", Title: check.Name}
				if check.State == "complete" {
					done := source.Now
					step.CompletedAt = &done
				}
				b.item(step)
			}
		}

		// The comments, oldest first, by the importing person with the commenter named.
		actions := comments[card.ID]
		sort.SliceStable(actions, func(i, j int) bool { return actions[i].Date < actions[j].Date })
		for _, action := range actions {
			author := action.MemberCreator.FullName
			if author == "" {
				author = action.MemberCreator.Username
			}
			var at *time.Time
			if parsed, err := time.Parse(time.RFC3339, action.Date); err == nil {
				at = &parsed
			}
			b.comment("comment:"+card.ID+":"+action.Date+":"+action.MemberCreator.Username, id, author, action.Data.Text, at)
		}
	}
	result := b.result(refused)
	if unmapped > 0 {
		result.Unmapped = map[string]int{"members": unmapped}
	}
	return result, nil
}

// trelloNotes is the description, and the attachments as lines beneath it.
func trelloNotes(card trelloCard) string {
	notes := strings.TrimSpace(card.Desc)
	if len(card.Attachments) == 0 {
		return notes
	}
	var lines []string
	for _, attachment := range card.Attachments {
		if attachment.URL == "" {
			continue
		}
		if attachment.Name != "" && attachment.Name != attachment.URL {
			lines = append(lines, attachment.Name+": "+attachment.URL)
		} else {
			lines = append(lines, attachment.URL)
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
