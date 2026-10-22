// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
	"time"
)

func TestSpread(t *testing.T) {
	cases := []struct {
		name string
		in   []float64
		want string
	}{
		{"empty", nil, "—"},
		{"one", []float64{2}, "2.0 / 2.0"},
		{"even count averages the middle", []float64{4, 1, 3, 2}, "2.5 / 4.0"},
		{"odd count", []float64{5, 1, 3}, "3.0 / 5.0"},
	}
	for _, c := range cases {
		if got := spread(c.in); got != c.want {
			t.Errorf("%s: spread(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestAddTranscript(t *testing.T) {
	w := window{from: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	data := strings.Join([]string{
		// Before the window: not counted.
		`{"type":"assistant","timestamp":"2026-10-12T23:59:00Z","message":{"id":"m0","usage":{"cache_read_input_tokens":9}}}`,
		`{"type":"assistant","timestamp":"2026-10-13T10:00:00Z","message":{"id":"m1","usage":{"cache_read_input_tokens":100,"cache_creation_input_tokens":1000,"output_tokens":10},"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"until grep -q exit= out.log; do sleep 5; done"}}]}}`,
		// The same message again, as Claude Code writes one line per content block: counted once.
		`{"type":"assistant","timestamp":"2026-10-13T10:00:01Z","message":{"id":"m1","usage":{"cache_read_input_tokens":100,"cache_creation_input_tokens":1000,"output_tokens":10}}}`,
		`{"type":"user","timestamp":"2026-10-13T10:10:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1"}]}}`,
		// Ten minutes later the whole context is written again.
		`{"type":"assistant","timestamp":"2026-10-13T10:10:05Z","message":{"id":"m2","usage":{"cache_creation_input_tokens":450000,"output_tokens":5},"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"make verify-pr"}}]}}`,
		`not json`,
	}, "\n")

	var s sessions
	if !addTranscript(&s, []byte(data), w, map[string]bool{}) {
		t.Fatal("the window holds lines, but the file was not counted")
	}
	if s.calls != 2 {
		t.Errorf("calls = %d, want 2", s.calls)
	}
	if s.cacheRead != 100 || s.cacheWrite != 451000 || s.output != 15 {
		t.Errorf("tokens = %d/%d/%d, want 100/451000/15", s.cacheRead, s.cacheWrite, s.output)
	}
	if s.rebuilds != 1 || s.rebuildTokens != 450000 {
		t.Errorf("rebuilds = %d (%d tokens), want 1 (450000)", s.rebuilds, s.rebuildTokens)
	}
	if s.inputAbove400k != 450000 {
		t.Errorf("input above 400k = %d, want 450000", s.inputAbove400k)
	}
	if s.waiting != 10*time.Minute || s.waitCalls != 1 {
		t.Errorf("waiting = %s in %d calls, want 10m0s in 1", s.waiting, s.waitCalls)
	}
	if s.checkRuns != 1 {
		t.Errorf("check runs = %d, want 1", s.checkRuns)
	}
}
