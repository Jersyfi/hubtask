// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package realtime

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// coverageCase is one run of packages/editor/scripts/coverage-fixtures.mjs: three writers' log,
// where each update ends per writer as the editors parse it, half the log merged, and the
// snapshots of a writer that applied the whole log and of one that missed an update.
type coverageCase struct {
	GC       bool                `json:"gc"`
	Updates  []string            `json:"updates"`
	Ends     []map[string]uint64 `json:"ends"`
	Merged   string              `json:"merged"`
	Snapshot string              `json:"snapshot"`
	Partial  string              `json:"partial"`
}

func coverageCases(t *testing.T) []coverageCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/coverage.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []coverageCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	return file.Cases
}

func readHex(t *testing.T, s string) Coverage {
	t.Helper()
	data, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ReadCoverage(data)
	if err != nil {
		t.Fatalf("an editor's update is unreadable: %v", err)
	}
	return c
}

func TestEveryEditorsUpdateIsReadWhereItEnds(t *testing.T) {
	for n, c := range coverageCases(t) {
		for i, update := range c.Updates {
			read := readHex(t, update)
			for client, end := range c.Ends[i] {
				id, _ := strconv.ParseUint(client, 10, 64)
				spans := read.structs[id]
				if len(spans) == 0 || spans[len(spans)-1].to != end {
					t.Errorf("case %d update %d: writer %s read to %v, the editors end it at %d", n, i, client, spans, end)
				}
			}
		}
	}
}

func TestASnapshotOfTheWholeLogCoversIt(t *testing.T) {
	for n, c := range coverageCases(t) {
		var log Coverage
		for _, update := range c.Updates {
			log.Add(readHex(t, update))
		}
		if !readHex(t, c.Snapshot).Covers(log) {
			t.Errorf("case %d (gc %v): the whole state does not cover its log", n, c.GC)
		}
		if !readHex(t, c.Merged).Covers(readHex(t, c.Updates[0])) {
			t.Errorf("case %d: a merged update does not cover its first", n)
		}
	}
}

func TestASnapshotThatMissedAnUpdateDoesNotCoverTheLog(t *testing.T) {
	for n, c := range coverageCases(t) {
		var log Coverage
		for _, update := range c.Updates {
			log.Add(readHex(t, update))
		}
		if readHex(t, c.Partial).Covers(log) {
			t.Errorf("case %d (gc %v): a snapshot without one update covers the log", n, c.GC)
		}
	}
}

func TestADeletionTheSnapshotLacksIsNotCovered(t *testing.T) {
	// One writer's three characters (client 5, clocks 0-2), then their deletion as its own update.
	insert := []byte{1, 1, 5, 0, 4, 1, 4, 't', 'e', 'x', 't', 3, 'a', 'b', 'c', 0}
	deletion := []byte{0, 1, 5, 1, 0, 3}
	var log Coverage
	for _, update := range [][]byte{insert, deletion} {
		c, err := ReadCoverage(update)
		if err != nil {
			t.Fatal(err)
		}
		log.Add(c)
	}
	held, _ := ReadCoverage(insert)
	if held.Covers(log) {
		t.Error("a state with the words but not their deletion covers a log that deleted them")
	}
	withDeletion, _ := ReadCoverage(append(insert[:len(insert)-1:len(insert)-1], 1, 5, 1, 0, 3))
	if !withDeletion.Covers(log) {
		t.Error("the state with the deletion does not cover the log")
	}
}

func TestAnUpdateTheEditorsCouldNotReadIsRefused(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":            {},
		"one byte":         {1},
		"cut short":        {1, 1, 5, 0, 4, 1, 4, 't', 'e', 'x', 't', 3, 'a', 'b'},
		"no delete set":    {1, 1, 5, 0, 4, 1, 4, 't', 'e', 'x', 't', 3, 'a', 'b', 'c'},
		"unknown content":  {1, 1, 5, 0, 15, 0},
		"unknown type":     {1, 1, 5, 0, 7, 1, 1, 'm', 9, 0},
		"invalid utf-8":    {1, 1, 5, 0, 4, 1, 4, 't', 'e', 'x', 't', 2, 0xe2, 0x82, 0},
		"invalid json":     {1, 1, 5, 0, 5, 1, 1, 'm', 2, '{', 'x', 0},
		"clock past range": {1, 1, 5, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0f, 0, 1, 0},
		"length past end":  {0, 1, 5, 1, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
	} {
		if _, err := ReadCoverage(data); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s: read as %v, want refused", name, err)
		}
	}
	nested := []byte{1, 1, 5, 0, 8, 1, 1, 'm', 1}
	for range maxAnyDepth + 2 {
		nested = append(nested, 117, 1)
	}
	if _, err := ReadCoverage(append(nested, 126, 0)); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a nesting past the editors' own: %v, want refused", err)
	}
}

func TestAnEmptyDocumentsStateIsReadable(t *testing.T) {
	c, err := ReadCoverage([]byte{0, 0})
	if err != nil || len(c.structs) != 0 {
		t.Fatalf("read %+v %v", c, err)
	}
	if !c.Covers(Coverage{}) {
		t.Error("an empty state does not cover an empty log")
	}
}
