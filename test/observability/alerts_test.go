// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package observability checks the artefacts under deploy/observability - the alert rules, their
// runbooks, and the dashboards.
//
// It is a test rather than a shell script for the reason every other gate in this repository is:
// it runs in `make verify` without a downloaded tool, and it fails with a sentence that says what
// to do. promtool checks the same file for PromQL validity in `make gate-observability`; the two
// halves are deliberately separate, because one needs Prometheus and the other needs to run on
// every laptop.
package observability

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	rulesFile    = "../../deploy/observability/alerts/prometheus-rules.yaml"
	runbookDir   = "../../deploy/observability/runbooks"
	dashboardDir = "../../deploy/observability/dashboards"
)

// ruleFile is the part of the Prometheus rule format this gate reads. Deliberately not the whole
// schema: promtool owns that, and a second parser would only disagree with it.
type ruleFile struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []struct {
			Alert       string            `yaml:"alert"`
			Expr        string            `yaml:"expr"`
			Labels      map[string]string `yaml:"labels"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

func load(t *testing.T) ruleFile {
	t.Helper()
	raw, err := os.ReadFile(rulesFile)
	if err != nil {
		t.Fatalf("the shipped rules are not readable: %v", err)
	}
	var parsed ruleFile
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("the shipped rules are not valid YAML: %v", err)
	}
	return parsed
}

func alerts(t *testing.T) []struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
} {
	t.Helper()
	var all []struct {
		Alert       string            `yaml:"alert"`
		Expr        string            `yaml:"expr"`
		Labels      map[string]string `yaml:"labels"`
		Annotations map[string]string `yaml:"annotations"`
	}
	for _, group := range load(t).Groups {
		all = append(all, group.Rules...)
	}
	if len(all) == 0 {
		t.Fatal("no alert was read at all - the parser no longer matches the file")
	}
	return all
}

// The rule observability-reliability.md §11 states in one line: any alert without a runbook does
// not ship. It is checked here rather than trusted, because the alert that goes out without one
// is the alert somebody added at two in the morning.
func TestEveryAlertNamesARunbookThatExists(t *testing.T) {
	for _, alert := range alerts(t) {
		runbook := alert.Annotations["runbook"]
		if runbook == "" {
			t.Errorf("%s ships without a runbook annotation (§11)", alert.Alert)
			continue
		}
		if _, err := os.Stat(filepath.Join(runbookDir, runbook)); err != nil {
			t.Errorf("%s names the runbook %s, which does not exist", alert.Alert, runbook)
		}
	}
}

// The other direction. A runbook nobody links to is a document that rots: it describes a threshold
// that moved, or an alert that was renamed, and the next person reads it as current.
func TestEveryRunbookBelongsToAnAlert(t *testing.T) {
	linked := map[string]bool{}
	for _, alert := range alerts(t) {
		linked[alert.Annotations["runbook"]] = true
	}

	entries, err := os.ReadDir(runbookDir)
	if err != nil {
		t.Fatalf("the runbook directory is not readable: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		if !linked[name] {
			t.Errorf("%s is a runbook no alert points at", name)
		}
	}
}

// An alert needs enough on it to be acted on without opening the rule file: which catalogue entry
// it is, how loudly it should be treated, and what it means in one line.
func TestEveryAlertCarriesWhatAnOperatorNeeds(t *testing.T) {
	catalogueID := regexp.MustCompile(`^A-\d{2}$`)
	severities := map[string]bool{"page": true, "ticket": true, "info": true}

	for _, alert := range alerts(t) {
		if id := alert.Labels["alert_id"]; !catalogueID.MatchString(id) {
			t.Errorf("%s has alert_id %q, want the catalogue form A-xx (§10)", alert.Alert, id)
		}
		if severity := alert.Labels["severity"]; !severities[severity] {
			t.Errorf("%s has severity %q, want page, ticket or info (§10)", alert.Alert, severity)
		}
		if strings.TrimSpace(alert.Annotations["summary"]) == "" {
			t.Errorf("%s has no summary - the line that arrives on a phone", alert.Alert)
		}
		if strings.TrimSpace(alert.Expr) == "" {
			t.Errorf("%s has no expression", alert.Alert)
		}
	}
}

// The self-hosting set is a decision, not an accident: alerts where doing nothing loses data or
// leaves the installation broken (§10). Pinning it here means adding one is a deliberate act -
// somebody has to change this list and say why in the pull request.
//
// A-08 was added by D-03, and the argument is in the rules file: a reminder that does not arrive
// cannot be caught up on afterwards, because the moment it was for has passed - which puts it with
// the losses rather than with the symptoms.
//
// A-20 was added by E-05, and the argument is the one A-12 already makes: a backup nobody has
// restored is a promise about a day that has not happened yet, and the only evidence for it is a
// restore that worked. An installation that discovers on that day that its archives do not open has
// lost the data as surely as one that never made them.
func TestTheSelfHostingSetIsExactlyWhatWasDecided(t *testing.T) {
	want := []string{"A-03", "A-04", "A-05", "A-07", "A-08", "A-12", "A-20"}

	var got []string
	for _, alert := range alerts(t) {
		got = append(got, alert.Labels["alert_id"])
	}
	sort.Strings(got)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the shipped set is %v, want %v (§10, the reduced self-hosting variant).\n"+
			"Adding an alert here is a decision: it has to be one where doing nothing loses data.",
			got, want)
	}
}

// A dashboard that does not parse is a dashboard nobody notices is broken until they import it.
func TestTheDashboardsAreImportable(t *testing.T) {
	entries, err := os.ReadDir(dashboardDir)
	if err != nil {
		t.Fatalf("the dashboard directory is not readable: %v", err)
	}

	var found int
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		found++
		raw, err := os.ReadFile(filepath.Join(dashboardDir, entry.Name()))
		if err != nil {
			t.Errorf("%s is not readable: %v", entry.Name(), err)
			continue
		}
		var dashboard map[string]any
		if err := yaml.Unmarshal(raw, &dashboard); err != nil {
			t.Errorf("%s is not valid JSON: %v", entry.Name(), err)
			continue
		}
		if dashboard["title"] == nil {
			t.Errorf("%s has no title - Grafana lists it as untitled", entry.Name())
		}
		if dashboard["panels"] == nil {
			t.Errorf("%s has no panels", entry.Name())
		}
	}
	if found == 0 {
		t.Error("no dashboard is shipped at all (§11)")
	}
}
