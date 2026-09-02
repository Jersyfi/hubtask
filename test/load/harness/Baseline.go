// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// The two tiers of decision 7 (docs/backlog/milestone-0.6.0.md), and this file is the cheap one.
//
// A shared runner varies by 10-30 % between runs, so a percent-level regression is invisible there
// and an absolute target is a coin toss. What a shared runner *can* answer is a narrower question:
// against a figure recorded on the same kind of machine, did this get significantly worse? That is
// all this compares, and the band is what "significantly" means, written down beside the figure
// rather than argued about after a red build.
//
// The other tier - the full capacity ramp, on named hardware, per release - is where an absolute
// number comes from. Nothing here produces one.

// Direction says which way is worse. Latency and memory rise; throughput falls. A figure without
// one would be a comparison that silently reads half of its regressions as improvements.
type Direction string

const (
	LowerIsBetter  Direction = "lower_is_better"
	HigherIsBetter Direction = "higher_is_better"
)

// Figure is one recorded number and the band around it.
type Figure struct {
	Value     float64   `json:"value"`
	Direction Direction `json:"direction"`
	// BandPercent is how far the figure may move in the worse direction before it is a
	// regression. It is per figure because the figures are not equally noisy: a P95 on a shared
	// runner moves far more than a count of refusals does.
	BandPercent float64 `json:"band_percent"`
	// Note says what the figure is, in words, so that a red build is readable without this file
	// beside the test that produced it.
	Note string `json:"note,omitempty"`
}

// Baseline is a set of figures, recorded on a named machine over a named dataset.
type Baseline struct {
	Name       string `json:"name"`
	RecordedAt string `json:"recorded_at"`
	// Hardware is the machine the figures were measured on, and it is load-bearing: comparing a
	// run against figures from different iron measures the iron. A run on hardware this does not
	// name records its measurement and compares nothing (decision 7, point 3).
	Hardware string `json:"hardware"`
	Dataset  struct {
		Tenants int `json:"tenants"`
		Items   int `json:"items"`
	} `json:"dataset"`
	Figures map[string]Figure `json:"figures"`
}

// Regression is one figure that moved too far the wrong way.
type Regression struct {
	Figure   string  `json:"figure"`
	Baseline float64 `json:"baseline"`
	Measured float64 `json:"measured"`
	// Allowed is the worst value that would still have passed - the edge of the band, so that a
	// red build says how far past it the run landed rather than only that it was past.
	Allowed float64 `json:"allowed"`
	Note    string  `json:"note,omitempty"`
}

func (r Regression) String() string {
	return fmt.Sprintf("%s: %.4g against a baseline of %.4g, past the band's %.4g",
		r.Figure, r.Measured, r.Baseline, r.Allowed)
}

// LoadBaseline reads a baseline from the repository.
func LoadBaseline(path string) (Baseline, error) {
	payload, err := os.ReadFile(path) //nolint:gosec // G304: the path is a constant in the test.
	if err != nil {
		return Baseline{}, fmt.Errorf("reading the baseline: %w", err)
	}
	var baseline Baseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return Baseline{}, fmt.Errorf("reading the baseline: %w", err)
	}
	if len(baseline.Figures) == 0 {
		return Baseline{}, fmt.Errorf("the baseline %s carries no figures", path)
	}
	for name, figure := range baseline.Figures {
		switch {
		case figure.Direction != LowerIsBetter && figure.Direction != HigherIsBetter:
			return Baseline{}, fmt.Errorf("the figure %s has no direction", name)
		case figure.BandPercent < 0:
			return Baseline{}, fmt.Errorf("the figure %s has a negative band", name)
		}
	}
	return baseline, nil
}

// Compare answers the one question this tier is allowed to answer: did anything get significantly
// worse?
//
// A figure the run did not measure is a miss rather than a pass - a guard that silently ignores
// what disappeared would go green on the day the measurement broke. A figure the run measured and
// the baseline does not carry is not an error: a new figure is added to the baseline when somebody
// has a number worth recording, not on the first run that produces one.
func (b Baseline) Compare(measured map[string]float64) (regressions []Regression, missing []string) {
	for name, figure := range b.Figures {
		value, taken := measured[name]
		if !taken {
			missing = append(missing, name)
			continue
		}

		allowed := figure.Value * (1 + figure.BandPercent/100)
		worse := value > allowed
		if figure.Direction == HigherIsBetter {
			allowed = figure.Value * (1 - figure.BandPercent/100)
			worse = value < allowed
		}
		if worse {
			regressions = append(regressions, Regression{
				Figure: name, Baseline: figure.Value, Measured: value,
				Allowed: allowed, Note: figure.Note,
			})
		}
	}

	sort.Strings(missing)
	sort.Slice(regressions, func(i, j int) bool { return regressions[i].Figure < regressions[j].Figure })
	return regressions, missing
}
