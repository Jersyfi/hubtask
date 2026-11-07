// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SlowPlan is the slowest plan auto_explain logged for one query during a run, and how often that
// query was over the threshold. The plan is the run's own: the dataset is torn down with it, and a
// plan read on a new seed may be another one.
type SlowPlan struct {
	Query     string  `json:"query"`
	Count     int     `json:"count"`
	SlowestMS float64 `json:"slowest_ms"`
	Plan      string  `json:"plan"`
}

var (
	planHead  = regexp.MustCompile(`\bLOG:\s+duration: ([0-9.]+) ms\s+plan:`)
	planQuery = regexp.MustCompile(`^\tQuery Text: (?:-- name: (\w+))?(.*)$`)
)

// SlowPlans reads PostgreSQL's log for auto_explain's plans and answers the slowest per sqlc query,
// the queries most often slow first.
func SlowPlans(r io.Reader) ([]SlowPlan, error) {
	kept := map[string]*SlowPlan{}
	var (
		query    string
		duration float64
		plan     []string
		inPlan   bool
		open     bool
	)
	closePlan := func() {
		if !open {
			return
		}
		open = false
		if query == "" {
			query = "unnamed"
		}
		entry := kept[query]
		if entry == nil {
			entry = &SlowPlan{Query: query}
			kept[query] = entry
		}
		entry.Count++
		if duration > entry.SlowestMS {
			entry.SlowestMS = duration
			entry.Plan = strings.Join(plan, "\n")
		}
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if head := planHead.FindStringSubmatch(line); head != nil {
			closePlan()
			open, inPlan, query, plan = true, false, "", nil
			duration, _ = strconv.ParseFloat(head[1], 64)
			continue
		}
		if !open {
			continue
		}
		if !strings.HasPrefix(line, "\t") {
			closePlan()
			continue
		}
		if named := planQuery.FindStringSubmatch(line); named != nil {
			query = named[1]
			if query == "" {
				// The query DSL's statements carry no sqlc name and share long openings: their
				// opening, values masked, and a short hash of the whole tell them apart.
				masked := varying.ReplaceAllString(strings.TrimSpace(named[2]), "…")
				sum := sha256.Sum256([]byte(masked))
				query = masked[:min(len(masked), 80)] + " #" + hex.EncodeToString(sum[:4])
			}
			continue
		}
		// The plan starts at its first node; the query text before it is not kept (rule 10: a
		// literal in it could be content).
		if !inPlan && strings.Contains(line, "(cost=") {
			inPlan = true
		}
		if inPlan {
			plan = append(plan, strings.TrimPrefix(line, "\t"))
		}
	}
	closePlan()
	out := make([]SlowPlan, 0, len(kept))
	for _, entry := range kept {
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Query < out[j].Query
	})
	return out, scanner.Err()
}
