// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"os"
	"path/filepath"
	"testing"
)

// The gate-selftest discipline applied to the guard itself: a guard nobody has proved can fail is
// a green light with no bulb in it. These are the two halves of the acceptance criterion - it goes
// red on a seeded slowdown past the band, and it stays green on noise inside it.
func aBaseline() Baseline {
	return Baseline{
		Name:     "rt6",
		Hardware: "a-named-machine",
		Figures: map[string]Figure{
			"interactive_p95_overload_ms": {Value: 400, Direction: LowerIsBetter, BandPercent: 30},
			"requests_per_second":         {Value: 500, Direction: HigherIsBetter, BandPercent: 20},
		},
	}
}

func TestTheGuardIsRedOnASeededSlowdownPastTheBand(t *testing.T) {
	// 30 % of 400 ms is 520 ms; 600 is past it and nothing about the machine explains that.
	regressions, missing := aBaseline().Compare(map[string]float64{
		"interactive_p95_overload_ms": 600,
		"requests_per_second":         510,
	})

	if len(missing) != 0 {
		t.Errorf("missing = %v", missing)
	}
	if len(regressions) != 1 || regressions[0].Figure != "interactive_p95_overload_ms" {
		t.Fatalf("regressions = %v", regressions)
	}
	if regressions[0].Allowed != 520 {
		t.Errorf("the band's edge was reported as %v, want 520", regressions[0].Allowed)
	}
}

func TestTheGuardStaysGreenOnNoiseInsideTheBand(t *testing.T) {
	// Everything a shared runner does between two runs of the same code: a fifth slower here, a
	// tenth fewer requests there. Neither is a regression and a guard that called them one would
	// be turned off within a week.
	regressions, missing := aBaseline().Compare(map[string]float64{
		"interactive_p95_overload_ms": 480,
		"requests_per_second":         450,
	})

	if len(regressions) != 0 {
		t.Errorf("noise was reported as a regression: %v", regressions)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v", missing)
	}
}

// A figure that fell off the measurement is not a pass. A guard that quietly ignored what
// disappeared would go green on the day the measurement broke, which is the worst of the three
// possible answers.
func TestAFigureTheRunDidNotMeasureIsAMiss(t *testing.T) {
	_, missing := aBaseline().Compare(map[string]float64{"requests_per_second": 600})

	if len(missing) != 1 || missing[0] != "interactive_p95_overload_ms" {
		t.Errorf("missing = %v", missing)
	}
}

// Throughput falls when it gets worse. A guard without a direction per figure reads half of its
// regressions as improvements, and the half it reads wrongly is the half that matters to whoever
// is paying for the vCPU.
func TestAFallInThroughputIsARegression(t *testing.T) {
	regressions, _ := aBaseline().Compare(map[string]float64{
		"interactive_p95_overload_ms": 300,
		"requests_per_second":         300,
	})

	if len(regressions) != 1 || regressions[0].Figure != "requests_per_second" {
		t.Fatalf("regressions = %v", regressions)
	}
	if regressions[0].Allowed != 400 {
		t.Errorf("the band's edge was reported as %v, want 400", regressions[0].Allowed)
	}
}

// The band is per figure, and a band of zero means "exactly this or better". It is a real setting
// for a figure that is not noisy - a count of refusals, say - and it must not be read as "no band
// configured, let everything through".
func TestABandOfZeroAdmitsNothingWorse(t *testing.T) {
	baseline := Baseline{Figures: map[string]Figure{
		"shed_requests": {Value: 100, Direction: HigherIsBetter, BandPercent: 0},
	}}

	if regressions, _ := baseline.Compare(map[string]float64{"shed_requests": 100}); len(regressions) != 0 {
		t.Errorf("the recorded figure itself was called a regression: %v", regressions)
	}
	if regressions, _ := baseline.Compare(map[string]float64{"shed_requests": 99}); len(regressions) != 1 {
		t.Errorf("a figure below a zero band passed: %v", regressions)
	}
}

// The baselines in the repository have to be readable and complete, because the first thing a
// broken one does is make the guard pass. A direction missing from a figure is exactly the typo
// that would.
func TestTheStoredBaselinesAreWellFormed(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "baselines", "*.json"))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no baseline is stored, so the guard has nothing to compare against")
	}
	for _, path := range paths {
		baseline, err := LoadBaseline(path)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
			continue
		}
		if baseline.Hardware == "" {
			t.Errorf("%s: no hardware named, so nothing can tell whether a run is comparable",
				filepath.Base(path))
		}
	}
}

func TestAnUnreadableBaselineIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","figures":{"a":{"value":1}}}`), 0o600); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := LoadBaseline(path); err == nil {
		t.Error("a figure without a direction was accepted")
	}
}
