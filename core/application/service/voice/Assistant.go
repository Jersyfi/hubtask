// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package voice turns what a person says to a voice assistant into work (ADR-0093): entries put
// on a list, a list read out, an entry ticked off, the day's due entries told.
//
// Every act goes through the catalogue as the person who linked the assistant - the same
// CreateWorkItem, QueryItems and CompleteWorkItem a click goes through - so what they may do by
// voice is exactly what they may do on the screen, and the audit trail reads the same.
package voice

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/voice"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	listContainersName   = "ListContainers"
	listLabelsName       = "ListLabels"
	createWorkItemName   = "CreateWorkItem"
	queryItemsName       = "QueryItems"
	completeWorkItemName = "CompleteWorkItem"

	// readOut is how many entries a list is read out with: more is not listened to.
	readOut = 8
	// pageSize is one level of the tree read at once.
	pageSize = 200
)

// Catalogue performs a named use case as an actor.
type Catalogue interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Assistant answers spoken requests.
type Assistant struct {
	Catalogue  Catalogue
	Settings   repository.Settings
	UnitOfWork persistence.UnitOfWork
}

// Reply is what the assistant says: a message code of the catalogue and its words. Open says the
// assistant keeps listening for an answer - a question it asked.
type Reply struct {
	Code   string
	Params map[string]string
	Open   bool
}

func reply(code string, params map[string]string) Reply { return Reply{Code: code, Params: params} }

// Interpretation is what a sentence would do, for the settings page's *Try a sentence*.
type Interpretation struct {
	Things    []domain.Thing
	Place     domain.Place
	Found     bool
	Ambiguous []domain.Place
	Default   bool
}

// Enabled reads whether the workspace takes spoken entries, and its own skill.
func (a Assistant) Enabled(ctx context.Context, actor appshared.ActorContext) (domain.Settings, error) {
	var settings domain.Settings
	err := a.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := a.Settings.Find(ctx)
		settings = found
		return err
	})
	return settings, err
}

// Interpret reads a sentence as an addition without doing anything.
func (a Assistant) Interpret(
	ctx context.Context, actor appshared.ActorContext, sentence string, language domain.Language,
) (Interpretation, error) {
	settings, known, err := a.places(ctx, actor)
	if err != nil {
		return Interpretation{}, err
	}
	return interpret(sentence, language, settings, known), nil
}

func interpret(sentence string, language domain.Language, settings domain.Settings, known []domain.Known) Interpretation {
	return interpretParts(sentence, "", language, settings, known)
}

// interpretParts reads things and a place said apart - an assistant's two slots - or, with no
// place given, one sentence that may name its place itself.
func interpretParts(things, target string, language domain.Language, settings domain.Settings, known []domain.Known) Interpretation {
	sentence := things
	addition := domain.ParseAddition(things, language)
	if strings.TrimSpace(target) != "" {
		addition = domain.Addition{Things: domain.WithoutTarget(things, language).Things, Target: target}
		sentence = things
	}
	if addition.Target != "" {
		resolution := domain.Resolve(addition.Target, language, known)
		switch {
		case resolution.Found:
			return Interpretation{Things: addition.Things, Place: resolution.Place, Found: true}
		case len(resolution.Ambiguous) > 0:
			return Interpretation{Things: addition.Things, Ambiguous: resolution.Ambiguous}
		}
		if strings.TrimSpace(target) != "" {
			// A place said in its own slot that names nothing is said back, not guessed.
			return Interpretation{Things: addition.Things}
		}
		// Words after "auf"/"to" that name no place are part of the thing: "Brot für Oma".
		addition = domain.WithoutTarget(sentence, language)
	}
	if place, ok := defaultPlace(settings, known); ok {
		return Interpretation{Things: addition.Things, Place: place, Found: true, Default: true}
	}
	return Interpretation{Things: addition.Things}
}

// Add puts what was said on the list it names: target is the place said in its own slot, or
// empty for a sentence that names its place itself ("Butter auf die Einkaufsliste").
func (a Assistant) Add(
	ctx context.Context, actor appshared.ActorContext, things, target string, language domain.Language,
) (Reply, error) {
	settings, known, err := a.places(ctx, actor)
	if err != nil {
		return Reply{}, err
	}
	meant := interpretParts(things, target, language, settings, known)
	if strings.TrimSpace(target) != "" && !meant.Found && len(meant.Ambiguous) == 0 {
		return reply("voice.no_such_list", map[string]string{"list": target}), nil
	}
	switch {
	case len(meant.Things) == 0:
		return reply("voice.nothing_heard", nil), nil
	case len(meant.Ambiguous) > 0:
		return Reply{Code: "voice.which_list", Params: map[string]string{"lists": names(meant.Ambiguous)}, Open: true}, nil
	case !meant.Found:
		return reply("voice.no_list", nil), nil
	}
	added := make([]string, 0, len(meant.Things))
	for _, thing := range meant.Things {
		input := usecase.Input{"type": "TASK", "title": thing.Title, "collection_id": meant.Place.CollectionID.String()}
		if thing.Amount != "" {
			input["amount"] = thing.Amount
		}
		if !meant.Place.LabelID.IsZero() {
			input["label_ids"] = []any{meant.Place.LabelID.String()}
		}
		if _, err := a.Catalogue.Invoke(ctx, createWorkItemName, actor, input); err != nil {
			if errors.Is(err, shared.ErrForbidden) {
				return reply("voice.not_allowed", map[string]string{"list": meant.Place.Name}), nil
			}
			// A collection that does not offer an amount refuses it; the thing goes on without.
			if thing.Amount != "" && errors.Is(err, shared.ErrValidation) {
				delete(input, "amount")
				title := thing.Amount + " " + thing.Title
				input["title"] = title
				if _, err := a.Catalogue.Invoke(ctx, createWorkItemName, actor, input); err != nil {
					return Reply{}, err
				}
				added = append(added, title)
				continue
			}
			return Reply{}, err
		}
		added = append(added, spoken(thing))
	}
	return reply("voice.added", map[string]string{
		"things": joinSpoken(added, language), "list": meant.Place.Name, "count": strconv.Itoa(len(added)),
	}), nil
}

// Read says what is open on a list: the one named, or the default list.
func (a Assistant) Read(
	ctx context.Context, actor appshared.ActorContext, target string, language domain.Language,
) (Reply, error) {
	settings, known, err := a.places(ctx, actor)
	if err != nil {
		return Reply{}, err
	}
	place, answer, ok := pick(target, target, language, settings, known)
	if !ok {
		return answer, nil
	}
	open, err := a.open(ctx, actor, place, readOut+1)
	if err != nil {
		return Reply{}, err
	}
	if len(open) == 0 {
		return reply("voice.list_empty", map[string]string{"list": place.Name}), nil
	}
	titles := make([]string, 0, readOut)
	for _, item := range open[:min(readOut, len(open))] {
		titles = append(titles, item.title)
	}
	code := "voice.list_reads"
	if len(open) > readOut {
		code = "voice.list_reads_more"
	}
	return reply(code, map[string]string{"list": place.Name, "things": joinSpoken(titles, language), "count": strconv.Itoa(len(titles))}), nil
}

// Complete ticks off the open entry named: thing and place said apart, or one sentence
// ("Butter von der Einkaufsliste") with target empty.
func (a Assistant) Complete(
	ctx context.Context, actor appshared.ActorContext, thing, target string, language domain.Language,
) (Reply, error) {
	settings, known, err := a.places(ctx, actor)
	if err != nil {
		return Reply{}, err
	}
	asked := domain.ParsePick(thing, language)
	if strings.TrimSpace(target) != "" {
		asked = domain.Pick{Title: strings.TrimSpace(thing), Target: target}
	}
	place, answer, ok := pick(asked.Target, asked.Target, language, settings, known)
	if !ok {
		return answer, nil
	}
	open, err := a.open(ctx, actor, place, pageSize)
	if err != nil {
		return Reply{}, err
	}
	wanted := domain.Fold(asked.Title)
	var match *openItem
	for index := range open {
		if domain.Fold(open[index].title) == wanted {
			match = &open[index]
			break
		}
	}
	if match == nil {
		for index := range open {
			if wanted != "" && strings.Contains(domain.Fold(open[index].title), wanted) {
				match = &open[index]
				break
			}
		}
	}
	if match == nil {
		return reply("voice.not_on_list", map[string]string{"thing": asked.Title, "list": place.Name}), nil
	}
	if _, err := a.Catalogue.Invoke(ctx, completeWorkItemName, actor, usecase.Input{"item_id": match.id}); err != nil {
		return Reply{}, err
	}
	return reply("voice.completed", map[string]string{"thing": match.title, "list": place.Name}), nil
}

// Today says what is due today or overdue on the person.
func (a Assistant) Today(ctx context.Context, actor appshared.ActorContext, language domain.Language) (Reply, error) {
	out, err := a.Catalogue.Invoke(ctx, queryItemsName, actor, usecase.Input{
		"filter": map[string]any{"op": "AND", "nodes": []any{
			map[string]any{"field": "is_completed", "op": "EQ", "value": false},
			map[string]any{"field": "assignee_id", "op": "EQ", "value": "@me"},
			map[string]any{"field": "due_at", "op": "LT", "value": "@today+P1D"},
		}},
		"sort": []any{map[string]any{"field": "due_at", "dir": "ASC"}},
		"size": readOut + 1,
	})
	if err != nil {
		return Reply{}, err
	}
	items := itemsOf(out)
	if len(items) == 0 {
		return reply("voice.today_none", nil), nil
	}
	titles := make([]string, 0, readOut)
	for _, item := range items[:min(readOut, len(items))] {
		titles = append(titles, item.title)
	}
	return reply("voice.today", map[string]string{"things": joinSpoken(titles, language), "count": strconv.Itoa(len(items))}), nil
}

// pick resolves the place of a read or a tick-off: the one named, else the default list.
func pick(target, said string, language domain.Language, settings domain.Settings, known []domain.Known) (domain.Place, Reply, bool) {
	if strings.TrimSpace(target) != "" {
		resolution := domain.Resolve(said, language, known)
		switch {
		case resolution.Found:
			return resolution.Place, Reply{}, true
		case len(resolution.Ambiguous) > 0:
			return domain.Place{}, Reply{Code: "voice.which_list", Params: map[string]string{"lists": names(resolution.Ambiguous)}, Open: true}, false
		default:
			return domain.Place{}, reply("voice.no_such_list", map[string]string{"list": said}), false
		}
	}
	if place, ok := defaultPlace(settings, known); ok {
		return place, Reply{}, true
	}
	return domain.Place{}, reply("voice.no_list", nil), false
}

func defaultPlace(settings domain.Settings, known []domain.Known) (domain.Place, bool) {
	if settings.DefaultCollectionID.IsZero() {
		return domain.Place{}, false
	}
	for _, each := range known {
		if each.Place.CollectionID == settings.DefaultCollectionID && each.Place.LabelID.IsZero() {
			return each.Place, true
		}
	}
	return domain.Place{}, false
}

type openItem struct{ id, title string }

// open reads a place's open entries, newest first by the list's own order.
func (a Assistant) open(ctx context.Context, actor appshared.ActorContext, place domain.Place, size int) ([]openItem, error) {
	nodes := []any{map[string]any{"field": "is_completed", "op": "EQ", "value": false}}
	if !place.LabelID.IsZero() {
		nodes = append(nodes, map[string]any{"field": "labels", "op": "CONTAINS", "value": place.LabelID.String()})
	}
	out, err := a.Catalogue.Invoke(ctx, queryItemsName, actor, usecase.Input{
		"scope_container_id": place.CollectionID.String(),
		"filter":             map[string]any{"op": "AND", "nodes": nodes},
		"size":               size,
	})
	if err != nil {
		return nil, err
	}
	return itemsOf(out), nil
}

func itemsOf(out usecase.Output) []openItem {
	rows, _ := out["data"].([]usecase.Output)
	items := make([]openItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, openItem{id: row.String("id"), title: row.String("title")})
	}
	return items
}

// places reads what the person can put things on: every collection they see, its categories
// (its labels), and the workspace's aliases.
func (a Assistant) places(ctx context.Context, actor appshared.ActorContext) (domain.Settings, []domain.Known, error) {
	settings, err := a.Enabled(ctx, actor)
	if err != nil {
		return domain.Settings{}, nil, err
	}
	hubs, err := a.Catalogue.Invoke(ctx, listContainersName, actor, usecase.Input{"type": "HUB", "size": pageSize})
	if err != nil {
		return domain.Settings{}, nil, err
	}
	var known []domain.Known
	collections := map[shared.ID]string{}
	hubRows, _ := hubs["data"].([]usecase.Output)
	for _, hub := range hubRows {
		level, err := a.Catalogue.Invoke(ctx, listContainersName, actor, usecase.Input{
			"parent_id": hub.String("id"), "type": "COLLECTION", "size": pageSize,
		})
		if err != nil {
			return domain.Settings{}, nil, err
		}
		rows, _ := level["data"].([]usecase.Output)
		for _, row := range rows {
			if archived, _ := row["effective_archived"].(bool); archived {
				continue
			}
			id := shared.ID(row.String("id"))
			name := row.String("name")
			collections[id] = name
			known = append(known, domain.Known{Place: domain.Place{CollectionID: id, Name: name}, Name: name})
			labels, err := a.Catalogue.Invoke(ctx, listLabelsName, actor, usecase.Input{"collection_id": id.String()})
			if err != nil {
				continue
			}
			labelRows, _ := labels["data"].([]usecase.Output)
			for _, label := range labelRows {
				labelName := label.String("name")
				known = append(known, domain.Known{
					Place:    domain.Place{CollectionID: id, LabelID: shared.ID(label.String("id")), Name: name + ", " + labelName},
					Name:     labelName,
					Category: true,
				})
			}
		}
	}
	for _, alias := range settings.Aliases {
		name, seen := collections[alias.CollectionID]
		if !seen {
			continue
		}
		place := domain.Place{CollectionID: alias.CollectionID, Name: name}
		if !alias.LabelID.IsZero() {
			for _, each := range known {
				if each.Place.CollectionID == alias.CollectionID && each.Place.LabelID == alias.LabelID {
					place = each.Place
				}
			}
		}
		known = append(known, domain.Known{Place: place, Name: alias.Phrase, IsAlias: true})
	}
	sort.SliceStable(known, func(i, j int) bool { return known[i].Name < known[j].Name })
	return settings, known, nil
}

func names(places []domain.Place) string {
	words := make([]string, 0, len(places))
	for _, place := range places {
		words = append(words, place.Name)
	}
	return strings.Join(words, ", ")
}

func spoken(thing domain.Thing) string {
	if thing.Amount == "" {
		return thing.Title
	}
	return thing.Amount + " " + thing.Title
}

// joinSpoken lists things the way they are said: "Milch, Eier und Brot".
func joinSpoken(words []string, language domain.Language) string {
	and := " and "
	if language == domain.German {
		and = " und "
	}
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	default:
		return strings.Join(words[:len(words)-1], ", ") + and + words[len(words)-1]
	}
}
