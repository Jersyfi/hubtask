// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// The OpenSSF Best Practices badge (ci-cd.md §12). The badge site answers a project's state as
// public JSON; the project's id stands in thresholds.json beside the level the badge must hold.

const (
	badgeMeasure = "best_practices_badge"
	badgeTimeout = 20 * time.Second
	// badgeAnswerLimit bounds what is read of an answer; a project's JSON is a few kilobytes.
	badgeAnswerLimit = 1 << 20
)

// badgeLevels are the levels the badge site names, lowest first.
var badgeLevels = []string{"in_progress", "passing", "silver", "gold"}

// badgeRank is a level's place among the levels, and false for a word that is none.
func badgeRank(level string) (int, bool) {
	for i, l := range badgeLevels {
		if l == level {
			return i, true
		}
	}
	return 0, false
}

// badge reads the level the project holds. Without a project there is nothing to ask for.
func badge(project int, fetch func(project int) ([]byte, error)) value {
	if project == 0 {
		return unmeasured("the project is not registered yet")
	}
	answer, err := fetch(project)
	if err != nil {
		return unmeasured("%v", err)
	}
	return badgeLevel(answer)
}

// badgeLevel reads the level out of the badge site's answer for one project.
func badgeLevel(answer []byte) value {
	var project struct {
		Level string `json:"badge_level"`
	}
	if err := json.Unmarshal(answer, &project); err != nil {
		return unmeasured("the badge site's answer is not JSON")
	}
	rank, ok := badgeRank(project.Level)
	if !ok {
		return unmeasured("the badge site's answer names no known level")
	}
	return value{n: float64(rank), numeric: true, text: project.Level}
}

// badgeSite is the one host this tool asks. Rule 6 guards the server's outbound calls (T-07);
// this tool is not part of the server and asks for a number-named page.
const badgeSite = "https://www.bestpractices.dev"

func fetchBadge(project int) ([]byte, error) {
	return fetchBadgeFrom(badgeSite, project)
}

// fetchBadgeFrom follows no redirect: an answer from another host is no answer.
func fetchBadgeFrom(site string, project int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), badgeTimeout)
	defer cancel()
	url := fmt.Sprintf("%s/projects/%d.json", site, project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout:       badgeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the badge site did not answer")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the badge site answered HTTP %d for project %d", resp.StatusCode, project)
	}
	return io.ReadAll(io.LimitReader(resp.Body, badgeAnswerLimit))
}
