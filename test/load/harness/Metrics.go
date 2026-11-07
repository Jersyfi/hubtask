// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// sampleLine is one sample of the Prometheus text format: the name, its labels, the value.
var sampleLine = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{([^}]*)\})?\s+(\S+)`)

// CounterBy sums a counter's samples by one label's value, so a scenario can read what a run
// added to it (one reading before, one after). A sample without the label is filed under "".
func CounterBy(r io.Reader, name, label string) (map[string]float64, error) {
	out := map[string]float64{}
	lines := bufio.NewScanner(r)
	lines.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for lines.Scan() {
		match := sampleLine.FindStringSubmatch(lines.Text())
		if match == nil || match[1] != name {
			continue
		}
		value, err := strconv.ParseFloat(match[3], 64)
		if err != nil {
			continue
		}
		key := ""
		for _, pair := range strings.Split(match[2], ",") {
			if k, v, found := strings.Cut(pair, "="); found && strings.TrimSpace(k) == label {
				key = strings.Trim(strings.TrimSpace(v), `"`)
			}
		}
		out[key] += value
	}
	return out, lines.Err()
}
