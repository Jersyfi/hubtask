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
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	rulesFile = "../../deploy/observability/alerts/prometheus-rules.yaml"
	// tenantRulesFile is the multi-tenant operator's additions (H-08): a separate file, so the
	// self-hosting pin below stays on the set where doing nothing loses data.
	tenantRulesFile = "../../deploy/observability/alerts/prometheus-rules-tenant.yaml"
	// providerRulesFile is the rest of the catalogue (H-12): what provider operation adds, with
	// an on-call rota behind it. A third file for the same reason the second is one - the
	// self-hosting pin must keep reading a file that never grows.
	providerRulesFile = "../../deploy/observability/alerts/prometheus-rules-provider.yaml"
	runbookDir        = "../../deploy/observability/runbooks"
	dashboardDir      = "../../deploy/observability/dashboards"
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

func load(t *testing.T, path string) ruleFile {
	t.Helper()
	raw, err := os.ReadFile(path)
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
	for _, path := range []string{rulesFile, tenantRulesFile, providerRulesFile} {
		for _, group := range load(t, path).Groups {
			for _, rule := range group.Rules {
				if rule.Alert == "" {
					// A recording rule (the SLO ratios A-01/A-02 read) - not an alert, and the
					// runbook obligation does not apply to it.
					continue
				}
				all = append(all, rule)
			}
		}
	}
	if len(all) == 0 {
		t.Fatal("no alert was read at all - the parser no longer matches the file")
	}
	return all
}

// selfHostingAlerts is the pinned set's own file, alone.
func selfHostingAlerts(t *testing.T) []string {
	t.Helper()
	var got []string
	for _, group := range load(t, rulesFile).Groups {
		for _, rule := range group.Rules {
			got = append(got, rule.Labels["alert_id"])
		}
	}
	return got
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
	want := []string{"A-03", "A-04", "A-05", "A-07", "A-08", "A-12", "A-19", "A-20"}

	// The self-hosting file alone: the tenant file next door (A-18, H-08) is the provider's
	// set, under its own rule - a capacity signal is not "doing nothing loses data", which is
	// exactly why it does not join this list.
	got := selfHostingAlerts(t)
	sort.Strings(got)

	// A-19 joined with E-10, under the rule this test states: a statutory deadline is not a
	// symptom somebody can catch up on afterwards. It loses no data - it loses a right, and the
	// period runs out whether or not anybody was watching, which is the same reason A-08 is here.
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

// dashboard is the part of the Grafana schema this gate reads. As with ruleFile above, it is
// deliberately not the whole thing: Grafana owns that, and the questions asked here are about what
// the document promises, not about whether Grafana would render every field.
type dashboard struct {
	UID    string `json:"uid"`
	Title  string `json:"title"`
	Panels []struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		FieldConfig struct {
			Defaults struct {
				NoValue string `json:"noValue"`
			} `json:"defaults"`
		} `json:"fieldConfig"`
		Options struct {
			Content string `json:"content"`
		} `json:"options"`
		Targets []struct {
			Expr string `json:"expr"`
		} `json:"targets"`
	} `json:"panels"`
}

func dashboards(t *testing.T) map[string]dashboard {
	t.Helper()
	entries, err := os.ReadDir(dashboardDir)
	if err != nil {
		t.Fatalf("the dashboard directory is not readable: %v", err)
	}
	loaded := map[string]dashboard{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dashboardDir, entry.Name()))
		if err != nil {
			t.Fatalf("%s is not readable: %v", entry.Name(), err)
		}
		var parsed dashboard
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("%s is not valid JSON: %v", entry.Name(), err)
		}
		loaded[entry.Name()] = parsed
	}
	return loaded
}

// The shipped set is pinned for the reason the self-hosting alert set is: §11 lists these files by
// name, and a dashboard that exists in one place and not the other is a promise to somebody
// reading the document rather than the directory.
func TestTheShippedDashboardsAreTheOnesTheDocumentPromises(t *testing.T) {
	want := []string{"overview.json", "pipeline.json", "slo.json", "tenant.json"}

	var got []string
	for name := range dashboards(t) {
		got = append(got, name)
	}
	sort.Strings(got)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the shipped dashboards are %v, want %v (§11).\n"+
			"Adding one is a decision: §11 names them, and the list there has to name it too.",
			got, want)
	}
}

// Grafana keys an imported dashboard by its uid: two files sharing one silently overwrite each
// other, and the operator ends up with three dashboards where four were shipped.
func TestTheDashboardIdentifiersAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for name, board := range dashboards(t) {
		if board.UID == "" {
			t.Errorf("%s has no uid - Grafana invents one per import and updates never land", name)
			continue
		}
		if other, taken := seen[board.UID]; taken {
			t.Errorf("%s and %s share the uid %q - importing both keeps one", name, other, board.UID)
		}
		seen[board.UID] = name
	}
}

// Every objective in §2 has a row. The dashboard is the answer to "are we keeping our promises",
// and an objective missing from it is one nobody is looking at - which is how SLO-6 and SLO-7 sat
// unmeasured until their metrics were built (H-12).
func TestTheSLODashboardCoversEveryObjective(t *testing.T) {
	board, ok := dashboards(t)["slo.json"]
	if !ok {
		t.Fatal("slo.json is not shipped (§11)")
	}

	var text strings.Builder
	for _, panel := range board.Panels {
		text.WriteString(panel.Title)
		text.WriteString(" ")
	}
	rows := text.String()

	for _, slo := range []string{"SLO-1", "SLO-2", "SLO-3", "SLO-4", "SLO-5", "SLO-6", "SLO-7", "SLO-8"} {
		if !strings.Contains(rows, slo) {
			t.Errorf("%s has no row in slo.json - §2 lists eight objectives and the view shows them all", slo)
		}
	}
}

// The tenant label is off by default (§3.2), and a dashboard built on it must say so rather than
// render empty graphs: an operator who sees "No data" concludes the panel is broken, and the one
// who reads the notice knows it is a setting. Grafana's `noValue` is the whole mechanism - it
// replaces an empty result with the text, so the degradation needs no plugin and no scripting.
func TestTheTenantDashboardDegradesToANotice(t *testing.T) {
	board, ok := dashboards(t)["tenant.json"]
	if !ok {
		t.Fatal("tenant.json is not shipped (§11)")
	}

	var explains bool
	for _, panel := range board.Panels {
		if strings.Contains(panel.Options.Content, "HUBTASK_METRICS_TENANT_LABEL") {
			explains = true
		}
		for _, target := range panel.Targets {
			if !strings.Contains(target.Expr, "tenant_id") {
				continue
			}
			if strings.TrimSpace(panel.FieldConfig.Defaults.NoValue) == "" {
				t.Errorf("panel %q reads tenant_id and has no noValue notice: without the label "+
					"it renders as No data, which reads as a broken panel rather than as a "+
					"setting that is off (§3.2)", panel.Title)
			}
		}
	}
	if !explains {
		t.Error("tenant.json nowhere names HUBTASK_METRICS_TENANT_LABEL - the notice it degrades " +
			"to has to say which setting fills it")
	}
}
