// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"testing"
	"time"
)

func connectorSource() Source { return Source{Key: "glpi", Kind: SourceConnector, Rank: 50} }

func TestAConnectorIsCheckedAgainstItsKind(t *testing.T) {
	base := ConnectorInput{ID: id("01"), HubID: id("02"), Key: "glpi", Kind: ConnectorGLPI, Source: connectorSource(),
		BaseURL: "https://glpi.example.org/", Settings: map[string]string{"client_id": "c"}, Enabled: true, Now: now}
	cases := []struct {
		name string
		edit func(*ConnectorInput)
		code string
	}{
		{"fine", func(*ConnectorInput) {}, ""},
		{"an unknown kind", func(in *ConnectorInput) { in.Kind = "SNMP" }, "cmdb.connector_kind_unknown"},
		{"a malformed key", func(in *ConnectorInput) { in.Key = "GLPI" }, "cmdb.connector_key_malformed"},
		{"a source that is a file", func(in *ConnectorInput) { in.Source.Kind = SourceFile }, "cmdb.connector_source_not_connector"},
		{"no address to pull from", func(in *ConnectorInput) { in.BaseURL = "" }, "cmdb.connector_base_url_required"},
		{"a plain-text address", func(in *ConnectorInput) { in.BaseURL = "http://glpi.example.org" }, "cmdb.connector_base_url_invalid"},
		{"an address with a credential", func(in *ConnectorInput) { in.BaseURL = "https://u:p@glpi.example.org" }, "cmdb.connector_base_url_invalid"},
		{"a missing setting", func(in *ConnectorInput) { in.Settings = nil }, "cmdb.connector_setting_required"},
		{"a period too short", func(in *ConnectorInput) { in.Every = 5 }, "cmdb.connector_every_invalid"},
		{"a host with a path", func(in *ConnectorInput) { in.AllowedHosts = []string{"a.example/x"} }, "cmdb.connector_host_invalid"},
		{"a pull with an incident collection", func(in *ConnectorInput) { in.Collection = id("09") }, "cmdb.connector_collection_push_only"},
	}
	for _, c := range cases {
		in := base
		in.Settings = map[string]string{"client_id": "c"}
		c.edit(&in)
		_, err := NewConnector(in, nil)
		if got := code(err); got != c.code {
			t.Errorf("%s: got %q, want %q", c.name, got, c.code)
		}
	}
}

func TestAConnectorReachesOnlyItsOwnHosts(t *testing.T) {
	c, err := NewConnector(ConnectorInput{Key: "intune", Kind: ConnectorIntune, Source: connectorSource(),
		Settings: map[string]string{"tenant": "t", "client_id": "c"}, AllowedHosts: []string{"Extra.Example.org"}, Now: now}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://graph.microsoft.com" {
		t.Fatalf("the kind's default address, got %q", c.BaseURL)
	}
	want := []string{"extra.example.org", "graph.microsoft.com", "login.microsoftonline.com"}
	if got := c.Hosts(); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("hosts %v, want %v", got, want)
	}
}

func TestAnAlertIntakeIsNeverScheduledAndNeedsNoAddress(t *testing.T) {
	in := ConnectorInput{Key: "am", Kind: ConnectorAlertmanager, Source: connectorSource(), BaseURL: "https://x.example", Now: now}
	c, err := NewConnector(in, nil)
	if err != nil || c.BaseURL != "" || c.NextRunAt != nil {
		t.Fatalf("an intake keeps no address and no schedule: %+v %v", c, err)
	}
	in.Every = 60
	if _, err := NewConnector(in, nil); code(err) != "cmdb.connector_push_not_scheduled" {
		t.Fatalf("got %v", err)
	}
}

func TestAScheduledConnectorRunsEveryPeriodAndStopsAfterFailures(t *testing.T) {
	c, err := NewConnector(ConnectorInput{Key: "hz", Kind: ConnectorHetzner, Source: connectorSource(), Every: 60, Enabled: true, Now: now}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Due(now) || c.Due(now.Add(-time.Minute)) {
		t.Fatal("a new scheduled connector is owed at once")
	}
	c = c.Ran(now, id("05"), ConnectorOK, "", "cursor-1", nil)
	if c.Cursor != "cursor-1" || !c.NextRunAt.Equal(now.Add(time.Hour)) || c.Due(now.Add(59*time.Minute)) {
		t.Fatalf("after a run: %+v", c)
	}
	retry := now.Add(10 * time.Minute)
	failed := c.Ran(now, "", ConnectorFailed, "connector.rate_limited", "ignored", &retry)
	if failed.Cursor != "cursor-1" || !failed.NextRunAt.Equal(retry) || failed.LastRun != id("05") {
		t.Fatalf("a failure keeps the cursor and honours the retry: %+v", failed)
	}
	for range MaxConnectorFailures - 1 {
		failed = failed.Ran(now, "", ConnectorFailed, "connector.unreachable", "", nil)
	}
	if failed.Enabled || failed.NextRunAt != nil {
		t.Fatalf("switched off after %d failures: %+v", MaxConnectorFailures, failed)
	}
}

func TestAnAlertOpensRepeatsResolvesAndReopens(t *testing.T) {
	fire := AlertSignal{Key: "k", Status: AlertFiring, Title: "disk full", Severity: "critical"}
	a, change := Receive(Alert{}, fire, now)
	if change != AlertOpened || a.Count != 1 || a.Status != AlertFiring {
		t.Fatalf("opened: %v %+v", change, a)
	}
	a.ID = id("07")
	if a, change = Receive(a, fire, now.Add(time.Minute)); change != AlertRepeated || a.Count != 2 {
		t.Fatalf("repeated: %v %+v", change, a)
	}
	if a, change = Receive(a, AlertSignal{Key: "k", Status: AlertResolved}, now.Add(2*time.Minute)); change != AlertClosed || a.ResolvedAt == nil {
		t.Fatalf("resolved: %v %+v", change, a)
	}
	if a, change = Receive(a, fire, now.Add(3*time.Minute)); change != AlertReopened || a.ResolvedAt != nil || a.Count != 3 {
		t.Fatalf("reopened: %v %+v", change, a)
	}
	if _, change = Receive(Alert{}, AlertSignal{Key: "x", Status: AlertResolved}, now); change != AlertUnchanged {
		t.Fatal("a resolution of a problem never seen records nothing")
	}
}

func TestAnAlertsHostsAreLookedUpByNameAddressAndShortName(t *testing.T) {
	s, err := CheckSignal(AlertSignal{Key: "k", Status: AlertFiring, Hosts: []string{"Web01.example.org:9100", " "}})
	if err != nil {
		t.Fatal(err)
	}
	l := AlertLookups(s)
	if len(l) != 2 || l[0]["hostname"] != "web01.example.org" || l[1]["hostname"] != "web01" {
		t.Fatalf("lookups %v", l)
	}
	if _, err := CheckSignal(AlertSignal{Status: AlertFiring}); code(err) != "cmdb.alert_key_invalid" {
		t.Fatalf("a signal without a key: %v", err)
	}
}
