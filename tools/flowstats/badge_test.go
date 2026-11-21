// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func answers(t *testing.T, answer []byte, err error) func(int) ([]byte, error) {
	t.Helper()
	return func(project int) ([]byte, error) {
		if project != 4242 {
			t.Errorf("asked for project %d, want 4242", project)
		}
		return answer, err
	}
}

func TestTheBadgeRowReadsTheProjectsLevel(t *testing.T) {
	fixture, err := os.ReadFile("testdata/badge-passing.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := badge(4242, answers(t, fixture, nil)).String(); got != "passing" {
		t.Errorf("the fixture reads %q, want passing", got)
	}

	// Without a project nothing is asked.
	asked := false
	got := badge(0, func(int) ([]byte, error) { asked = true; return nil, nil })
	if want := "not measured: the project is not registered yet"; got.String() != want || asked {
		t.Errorf("no project: %q (asked: %v), want %q and no request", got, asked, want)
	}

	for name, c := range map[string]struct {
		answer string
		err    error
		want   string
	}{
		"the site does not answer": {"", errors.New("the badge site did not answer"), "not measured: the badge site did not answer"},
		"no JSON":                  {"<html>", nil, "not measured: the badge site's answer is not JSON"},
		"no level":                 {`{"id": 4242}`, nil, "not measured: the badge site's answer names no known level"},
		"a level it does not know": {`{"badge_level": "platinum"}`, nil, "not measured: the badge site's answer names no known level"},
	} {
		if got := badge(4242, answers(t, []byte(c.answer), c.err)); got.String() != c.want || got.numeric {
			t.Errorf("%s: %q, want %q and no number", name, got, c.want)
		}
	}
}

// A level below the one the threshold asks for is a breach like any other; an unregistered
// project breaches nothing.
func TestABadgeBelowItsLevelIsABreach(t *testing.T) {
	limits, err := parseThresholds([]byte(`{"best_practices_badge": {"level": "passing", "project": 4242}}`))
	if err != nil {
		t.Fatal(err)
	}
	if limits[badgeMeasure].Project != 4242 {
		t.Fatalf("project %d, want 4242", limits[badgeMeasure].Project)
	}
	state := unmeasured("the badge's level is read as it is now")
	for level, want := range map[string]bool{"in_progress": true, "passing": false, "silver": false, "gold": false} {
		m := measure{id: badgeMeasure, name: "OpenSSF Best Practices badge", now: badgeLevel([]byte(`{"badge_level": "` + level + `"}`)), before: state}
		found := breaches([]measure{m}, limits, week)
		if got := len(found) == 1; got != want {
			t.Errorf("%s: breach %v, want %v", level, got, want)
		}
		if want {
			if title := "Flow report: OpenSSF Best Practices badge is below its threshold"; found[0].Title != title {
				t.Errorf("title %q, want %q", found[0].Title, title)
			}
			for _, part := range []string{"* Value: in_progress", "* Threshold: at least passing"} {
				if !strings.Contains(found[0].Body, part) {
					t.Errorf("the finding's text lacks %q:\n%s", part, found[0].Body)
				}
			}
		}
		if got := len(within([]measure{m}, limits)) == 1; got == want {
			t.Errorf("%s: within %v beside breach %v", level, got, want)
		}
	}
	unregistered := measure{id: badgeMeasure, now: badge(0, nil), before: state}
	if len(breaches([]measure{unregistered}, limits, week))+len(within([]measure{unregistered}, limits)) != 0 {
		t.Error("an unregistered project breaches or holds a threshold")
	}
}

func TestBadgeThresholdsAreRefused(t *testing.T) {
	for name, data := range map[string]string{
		"an unknown level":           `{"best_practices_badge": {"level": "platinum"}}`,
		"a level and a max":          `{"best_practices_badge": {"level": "passing", "max": 1}}`,
		"a project without a level":  `{"best_practices_badge": {"project": 4242}}`,
		"a project below zero":       `{"best_practices_badge": {"level": "passing", "project": -1}}`,
		"a level on another measure": `{"fixed_waits": {"level": "passing"}}`,
		"a project on another one":   `{"fixed_waits": {"rule": "no_rise", "project": 4242}}`,
	} {
		if _, err := parseThresholds([]byte(data)); err == nil {
			t.Errorf("%s is accepted: %s", name, data)
		}
	}
}

// The fetch itself, against a site that answers, refuses and sends elsewhere.
func TestTheBadgeFetchTakesOnlyThePagesOwnAnswer(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/projects/7.json":
			_, _ = w.Write([]byte(`{"badge_level":"passing"}`))
		case "/projects/8.json":
			http.Redirect(w, r, "/projects/7.json", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()

	answer, err := fetchBadgeFrom(site.URL, 7)
	if err != nil || string(answer) != `{"badge_level":"passing"}` {
		t.Fatalf("the page's answer was not returned: %q, %v", answer, err)
	}
	if _, err := fetchBadgeFrom(site.URL, 8); err == nil {
		t.Error("a redirect was followed")
	}
	if _, err := fetchBadgeFrom(site.URL, 9); err == nil {
		t.Error("a missing project gave an answer")
	}
}
