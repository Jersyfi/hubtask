// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"strings"
	"testing"
)

// S5 reads the awareness relay's outcomes from the metrics before and after its window
// (test/load/PLAN.md, S5): one sum per outcome, other counters and comments passed over.
func TestACounterIsSummedByOneLabel(t *testing.T) {
	text := strings.Join([]string{
		`# HELP hubtask_realtime_awareness_relayed_total Live document awareness handed to the other replicas.`,
		`# TYPE hubtask_realtime_awareness_relayed_total counter`,
		`hubtask_realtime_awareness_relayed_total{otel_scope_name="hubtask",outcome="local"} 120`,
		`hubtask_realtime_awareness_relayed_total{outcome="sent",otel_scope_name="other"} 3`,
		`hubtask_realtime_awareness_relayed_total{outcome="sent"} 4`,
		`hubtask_realtime_awareness_relayed_total_other{outcome="failed"} 9`,
		`hubtask_realtime_awareness_relayed_total 2`,
	}, "\n")
	got, err := CounterBy(strings.NewReader(text), "hubtask_realtime_awareness_relayed_total", "outcome")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["local"] != 120 || got["sent"] != 7 || got[""] != 2 || got["failed"] != 0 {
		t.Errorf("got %v", got)
	}
}
