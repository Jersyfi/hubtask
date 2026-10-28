// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var (
	linkNow   = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	linkID    = shared.MustParseID("0192f000-0000-7000-8000-0000000001a1")
	entryOne  = shared.MustParseID("0192f000-0000-7000-8000-0000000001b1")
	entryTwo  = shared.MustParseID("0192f000-0000-7000-8000-0000000001b2")
	ownLinkIn = work.NewLinkTypeInput{
		ID: linkID, TenantID: tenant, Key: "tests", Name: "tests", InverseName: "is tested by",
		Now: linkNow, Text: text.Composing{},
	}
)

func linkCode(err error) string {
	var typed *shared.Error
	if errors.As(err, &typed) {
		return typed.DetailCode
	}
	return ""
}

// UC-PLN-01 check 5: a workspace's own link type has its two names, a direction, who waits, at
// most one per entry and its cycle rule - and the combinations that mean nothing are refused.
func TestNewLinkTypeRefusesWhatIsNotALinkType(t *testing.T) {
	cases := []struct {
		name   string
		change func(*work.NewLinkTypeInput)
		code   string
	}{
		{"a malformed key", func(in *work.NewLinkTypeInput) { in.Key = "Tests!" }, "link_types.key_malformed"},
		{"a shipped key", func(in *work.NewLinkTypeInput) { in.Key = work.LinkTypeBlocks }, "link_types.key_reserved"},
		{"no name", func(in *work.NewLinkTypeInput) { in.Name = " " }, "link_types.name_empty"},
		{"a directed type without its inverse", func(in *work.NewLinkTypeInput) { in.InverseName = "" }, "link_types.inverse_name_empty"},
		{"an unknown direction", func(in *work.NewLinkTypeInput) { in.Direction = "SIDEWAYS" }, "link_types.direction_unknown"},
		{"a symmetric type somebody waits on", func(in *work.NewLinkTypeInput) {
			in.Direction, in.Waits = work.LinkSymmetric, work.LinkWaitsSource
		}, "link_types.symmetric_waits"},
		{"a symmetric type of one per entry", func(in *work.NewLinkTypeInput) {
			in.Direction, in.Cardinality = work.LinkSymmetric, work.LinkOne
		}, "link_types.symmetric_one"},
		{"a dependency that may loop", func(in *work.NewLinkTypeInput) {
			in.Waits, in.Cycles = work.LinkWaitsSource, work.LinkCyclesAllowed
		}, "link_types.waiting_cycles"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := ownLinkIn
			tc.change(&in)
			_, err := work.NewLinkType(in)
			if got := linkCode(err); got != tc.code {
				t.Errorf("refused with %q (%v), want %q", got, err, tc.code)
			}
		})
	}
}

func TestANewLinkTypeDefaultsToADirectedManyLinkThatMayLoop(t *testing.T) {
	linkType, err := work.NewLinkType(ownLinkIn)
	if err != nil {
		t.Fatal(err)
	}
	if linkType.Direction != work.LinkDirected || linkType.Cardinality != work.LinkMany ||
		linkType.Cycles != work.LinkCyclesAllowed || linkType.Waits != work.LinkWaitsNone || linkType.Version != 1 {
		t.Errorf("defaults: %+v", linkType)
	}
}

// UC-PLN-01 check 5: archiving keeps the type readable and records the change; a shipped type
// does not change.
func TestALinkTypeIsArchivedAndAShippedOneIsFixed(t *testing.T) {
	own, err := work.NewLinkType(ownLinkIn)
	if err != nil {
		t.Fatal(err)
	}
	archive := true
	archived, changes, err := own.Changed(work.LinkTypeChange{Archived: &archive}, linkNow.Add(time.Hour), text.Composing{})
	if err != nil || !archived.IsArchived() || len(changes) != 1 || changes[0].Field != "archived" {
		t.Errorf("archived %+v, %v, %v", archived, changes, err)
	}
	blocks, _ := work.NewLinkTypes(nil).Find(work.LinkTypeBlocks)
	name := "stops"
	if _, _, err := blocks.Changed(work.LinkTypeChange{Name: &name}, linkNow, text.Composing{}); linkCode(err) != "link_types.system_fixed" {
		t.Errorf("a shipped type changed: %v", err)
	}
}

// F-3: the six shipped types, and a workspace's row never shadows one of them.
func TestTheRegistryCarriesTheSixShippedTypesFirst(t *testing.T) {
	shadow := work.LinkType{Key: work.LinkTypeBlocks, Name: "mine"}
	own, _ := work.NewLinkType(ownLinkIn)
	registry := work.NewLinkTypes([]work.LinkType{shadow, own})
	want := []string{"blocks", "depends_on", "duplicates", "runs_on", "affects", "implements", "tests"}
	if len(registry) != len(want) {
		t.Fatalf("registry %v", registry)
	}
	for i, key := range want {
		if registry[i].Key != key {
			t.Errorf("position %d is %q, want %q", i, registry[i].Key, key)
		}
	}
	if blocks, _ := registry.Find(work.LinkTypeBlocks); blocks.Name != "" || !blocks.System {
		t.Errorf("the shipped type was shadowed: %+v", blocks)
	}
	sourceWaits, targetWaits := registry.Waiting()
	if len(sourceWaits) != 1 || sourceWaits[0] != "depends_on" || len(targetWaits) != 1 || targetWaits[0] != "blocks" {
		t.Errorf("waiting %v %v", sourceWaits, targetWaits)
	}
}

// UC-PLN-01 check 1: an entry does not link to itself, a symmetric link is one link whichever end
// draws it, and an archived type draws none.
func TestNewEntryLink(t *testing.T) {
	registry := work.NewLinkTypes(nil)
	blocks, _ := registry.Find(work.LinkTypeBlocks)
	in := work.NewEntryLinkInput{ID: linkID, TenantID: tenant, Type: blocks, SourceID: entryTwo, TargetID: entryOne, Now: linkNow}

	link, err := work.NewEntryLink(in)
	if err != nil || link.SourceID != entryTwo || link.TargetID != entryOne {
		t.Errorf("a directed link kept its ends: %+v, %v", link, err)
	}

	self := in
	self.TargetID = entryTwo
	if _, err := work.NewEntryLink(self); linkCode(err) != "links.self" {
		t.Errorf("an entry linked to itself: %v", err)
	}

	symmetric := in
	symmetric.Type = work.LinkType{Key: "relates", Direction: work.LinkSymmetric}
	link, err = work.NewEntryLink(symmetric)
	if err != nil || link.SourceID != entryOne || link.TargetID != entryTwo {
		t.Errorf("a symmetric link was not stored in one order: %+v, %v", link, err)
	}

	at := linkNow
	archived := in
	archived.Type.ArchivedAt = &at
	if _, err := work.NewEntryLink(archived); linkCode(err) != "links.type_archived" {
		t.Errorf("an archived type drew a link: %v", err)
	}
}

// UC-PLN-01 checks 2 and 6: who waits for whom, and the walk that proves a loop - over every
// waiting type for a dependency, so that "depends on" and "blocks" cannot loop between them.
func TestADependencyReadsWhoWaitsAndWhereALoopWouldClose(t *testing.T) {
	registry := work.NewLinkTypes(nil)
	dependsOn, _ := registry.Find(work.LinkTypeDependsOn)
	blocks, _ := registry.Find(work.LinkTypeBlocks)
	affects, _ := registry.Find(work.LinkTypeAffects)
	link := work.EntryLink{Type: dependsOn.Key, SourceID: entryOne, TargetID: entryTwo}

	if waiter, waitedFor, ok := link.Dependency(dependsOn); !ok || waiter != entryOne || waitedFor != entryTwo {
		t.Errorf("depends on: %v waits for %v (%v)", waiter, waitedFor, ok)
	}
	if waiter, waitedFor, ok := link.Dependency(blocks); !ok || waiter != entryTwo || waitedFor != entryOne {
		t.Errorf("blocks: %v waits for %v (%v)", waiter, waitedFor, ok)
	}
	if _, _, ok := link.Dependency(affects); ok {
		t.Error("affects is a dependency")
	}

	from, to, forward, backward := link.ClosesLoopFrom(dependsOn, registry)
	if from != entryTwo || to != entryOne || len(forward) != 1 || forward[0] != "depends_on" ||
		len(backward) != 1 || backward[0] != "blocks" {
		t.Errorf("walk %v -> %v over %v / %v", from, to, forward, backward)
	}
	from, to, forward, backward = link.ClosesLoopFrom(affects, registry)
	if from != entryTwo || to != entryOne || len(forward) != 1 || forward[0] != "affects" || len(backward) != 0 {
		t.Errorf("walk %v -> %v over %v / %v", from, to, forward, backward)
	}
	if other, outward := link.Other(entryTwo); other != entryOne || outward {
		t.Errorf("other end %v, outward %v", other, outward)
	}
}
