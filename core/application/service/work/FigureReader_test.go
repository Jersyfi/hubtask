// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// figureStore answers fixed counts, or fails, and remembers what it was asked.
type figureStore struct {
	counts map[shared.ID]domain.FigureCounts
	err    error
	asked  []domain.FigureRequest
	window domain.FigureWindow
}

func (f *figureStore) Count(
	_ context.Context, requests []domain.FigureRequest, window domain.FigureWindow,
) (map[shared.ID]domain.FigureCounts, error) {
	f.asked, f.window = append(f.asked, requests...), window
	if f.err != nil {
		return nil, f.err
	}
	return f.counts, nil
}

func structureCounting(figure domain.Figure, levels ...domain.ItemType) domain.EffectiveStructure {
	values := domain.StructureValues{Counts: &domain.Counts{Figure: figure}, Fields: map[domain.ItemType]domain.LevelFields{}}
	for _, itemType := range levels {
		values.Levels = append(values.Levels, domain.Level{Type: itemType})
		values.Fields[itemType] = domain.LevelFields{Capabilities: []domain.Capability{domain.CapabilityCompletion, domain.CapabilityDueDate}}
	}
	return domain.EffectiveStructure{Values: values}
}

func countedCollection(id shared.ID) domain.Container {
	return domain.Container{ID: id, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID, Name: "C", Version: 1}
}

// Which containers are counted, in the reader's day: only collections that are not trashed and
// whose structure counts something counted here.
func TestOnlyCollectionsThatCountSomethingAreCounted(t *testing.T) {
	trashedAt := now
	trashed := countedCollection("0192f000-0000-7000-8000-0000000000d1")
	trashed.DeletedAt = &trashedAt
	none := countedCollection("0192f000-0000-7000-8000-0000000000d2")
	flow := countedCollection("0192f000-0000-7000-8000-0000000000d3")
	due := countedCollection("0192f000-0000-7000-8000-0000000000d4")
	hub := domain.Container{ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "H", Version: 1}

	store := &figureStore{counts: map[shared.ID]domain.FigureCounts{due.ID: {Due: 3}}}
	reader := FigureReader{
		Figures: store, Clock: clock.Fixed(time.Date(2026, 10, 18, 23, 30, 0, 0, time.UTC)),
		Profiles: &profiles{rows: seededProfiles()},
	}
	caller := actor()
	caller.TimeZone, caller.WeekStart = "Europe/Berlin", "MONDAY"

	figures := reader.Count(context.Background(), caller, &unitOfWork{},
		[]domain.Container{hub, trashed, none, flow, due},
		map[shared.ID]domain.EffectiveStructure{
			hub.ID:     structureCounting(domain.FigureDue, domain.ItemTask),
			trashed.ID: structureCounting(domain.FigureDue, domain.ItemTask),
			none.ID:    structureCounting(domain.FigureNone, domain.ItemTask),
			flow.ID:    structureCounting(domain.FigureFlow, domain.ItemTask),
			due.ID:     structureCounting(domain.FigureDue, domain.ItemTask),
		})

	if len(store.asked) != 1 || store.asked[0].CollectionID != due.ID {
		t.Fatalf("asked %+v", store.asked)
	}
	if store.window.Today != "2026-10-19" {
		t.Errorf("counted for %s, want the reader's Monday", store.window.Today)
	}
	if len(figures) != 1 || figures[due.ID].Counts.Due != 3 || figures[due.ID].Today != "2026-10-19" {
		t.Errorf("figures %+v", figures)
	}
}

// A failing count answers the mark, never an error; an unwired reader answers nothing.
func TestAFigureThatCannotBeCountedIsMarkedAndTheReadGoesOn(t *testing.T) {
	collection := countedCollection(collectionID)
	structures := map[shared.ID]domain.EffectiveStructure{collection.ID: structureCounting(domain.FigureOpen, domain.ItemTask)}

	failing := FigureReader{
		Figures: &figureStore{err: errors.New("statement timeout")}, Clock: clock.Fixed(now),
		Profiles: &profiles{rows: seededProfiles()},
	}
	figures := failing.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	out := figureOutput(figures[collection.ID])
	if out["unavailable"] != true || out["kind"] != "OPEN" || out["open"] != nil {
		t.Errorf("a failed count answered %v", out)
	}

	if got := (FigureReader{}).Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures); len(got) != 0 {
		t.Errorf("an unwired reader counted %v", got)
	}
}

// Each kind answers the numbers it needs and nothing else; a sum is exact.
func TestAFigureAnswersTheNumbersOfItsKind(t *testing.T) {
	sum := "38.10"
	cases := []struct {
		kind domain.Figure
		want []string
	}{
		{domain.FigureOpen, []string{"open"}},
		{domain.FigureFieldSum, []string{"open", "sum", "summed"}},
		{domain.FigureDue, []string{"due"}},
		{domain.FigureRhythm, []string{"days", "week_start"}},
		{domain.FigureProgress, []string{"done", "total"}},
	}
	for _, tc := range cases {
		out := figureOutput(domain.CollectionFigure{Kind: tc.kind, Today: "2026-10-22", WeekStart: "2026-10-19",
			Counts: domain.FigureCounts{Open: 5, Sum: &sum, Summed: 4, Due: 3, Done: 2, Total: 8}})
		if len(out) != len(tc.want)+2 {
			t.Errorf("%s answered %v", tc.kind, out)
		}
		for _, key := range tc.want {
			if _, held := out[key]; !held {
				t.Errorf("%s lacks %s", tc.kind, key)
			}
		}
	}
	encoded, _ := json.Marshal(figureOutput(domain.CollectionFigure{Kind: domain.FigureFieldSum, Counts: domain.FigureCounts{Sum: &sum}}))
	if want := `"sum":38.10`; !strings.Contains(string(encoded), want) {
		t.Errorf("encoded %s, want %s", encoded, want)
	}
}

// Through the registry: a collection's read answers its figure beside its structure.
func TestAContainerReadAnswersTheFigure(t *testing.T) {
	h := newStructureHarness(t)
	store := &figureStore{counts: map[shared.ID]domain.FigureCounts{collectionID: {Open: 4}}}
	get := GetContainer{
		Containers: h.containers, Authorizer: h.authorizer, UnitOfWork: h.uow,
		Structure: h.set.Structure, Figures: FigureReader{Figures: store, Clock: clock.Fixed(now), Profiles: &profiles{rows: seededProfiles()}},
	}
	registry, err := usecase.NewRegistry(nil, get.Descriptor())
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	out, err := registry.Invoke(context.Background(), GetContainerName, actor(), usecase.Input{"container_id": collectionID.String()})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	figure, _ := out["figure"].(map[string]any)
	if figure["kind"] != "OPEN" || figure["open"] != 4 {
		t.Errorf("figure %v", out["figure"])
	}
	if out["structure"] == nil {
		t.Error("the structure went missing")
	}
}
