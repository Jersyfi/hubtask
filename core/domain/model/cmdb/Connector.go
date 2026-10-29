// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ConnectorKind is the program a connector talks to (research itsm-cmdb-ai §4.6, the first wave).
type ConnectorKind string

const (
	ConnectorGLPI         ConnectorKind = "GLPI"
	ConnectorNetBox       ConnectorKind = "NETBOX"
	ConnectorIntune       ConnectorKind = "INTUNE"
	ConnectorEntra        ConnectorKind = "ENTRA"
	ConnectorHetzner      ConnectorKind = "HETZNER"
	ConnectorOpenStack    ConnectorKind = "OPENSTACK"
	ConnectorAlertmanager ConnectorKind = "ALERTMANAGER"
	ConnectorZabbix       ConnectorKind = "ZABBIX"
	ConnectorCheckmk      ConnectorKind = "CHECKMK"
)

// The bounds of a connector.
const (
	MinConnectorEvery   = 15
	MaxConnectorEvery   = 7 * 24 * 60
	MaxConnectorHosts   = 10
	MaxConnectorSetting = 500
	MaxConnectorsPerHub = 50
	// MaxConnectorFailures switches a scheduled connector off after so many failed runs in a row,
	// so that a source gone for good stops costing a call every period.
	MaxConnectorFailures = 10
)

// ConnectorSpec is what a kind needs: whether it pulls or is pushed to and the settings it cannot
// do without. Every address a connector calls arrives as its configuration - the base address and
// an identity provider's authority - never from the source code (PG-6).
type ConnectorSpec struct {
	Kind ConnectorKind
	// Push is an intake a monitoring program posts alerts to; otherwise Hubtask pulls.
	Push bool
	// Settings are the non-secret settings the kind needs, by key.
	Settings []string
	// Authority names the setting holding the https address of the identity provider the kind
	// takes its token from, a host it may reach besides its base address's.
	Authority string
}

// ConnectorSpecs are the kinds Hubtask ships.
func ConnectorSpecs() []ConnectorSpec {
	return []ConnectorSpec{
		{Kind: ConnectorGLPI, Settings: []string{"client_id"}},
		{Kind: ConnectorNetBox},
		{Kind: ConnectorIntune, Settings: []string{"tenant", "client_id", "authority"}, Authority: "authority"},
		{Kind: ConnectorEntra, Settings: []string{"tenant", "client_id", "authority"}, Authority: "authority"},
		{Kind: ConnectorHetzner},
		{Kind: ConnectorOpenStack, Settings: []string{"application_credential_id"}},
		{Kind: ConnectorAlertmanager, Push: true},
		{Kind: ConnectorZabbix, Push: true},
		{Kind: ConnectorCheckmk, Push: true},
	}
}

// SpecOf answers a kind's spec.
func SpecOf(kind ConnectorKind) (ConnectorSpec, bool) {
	for _, s := range ConnectorSpecs() {
		if s.Kind == kind {
			return s, true
		}
	}
	return ConnectorSpec{}, false
}

// ConnectorOutcome is how a connector's last run ended.
type ConnectorOutcome string

const (
	ConnectorNeverRan ConnectorOutcome = ""
	ConnectorOK       ConnectorOutcome = "OK"
	// ConnectorEmpty is a run that fetched nothing, which a source that should report is told of
	// (research itsm-cmdb-ai C8: alert on zero results).
	ConnectorEmpty  ConnectorOutcome = "EMPTY"
	ConnectorFailed ConnectorOutcome = "FAILED"
)

// Connector is a program Hubtask pulls CIs from, or that posts alerts to it, in one hub. It
// writes as one of the workspace's sources, so the source's rank and classes decide what it may
// change (R1, R2). Its credential is sealed and never answered (research itsm-cmdb-ai C5).
type Connector struct {
	ID       shared.ID
	TenantID shared.ID
	HubID    shared.ID
	Key      string
	Kind     ConnectorKind
	// Source is the key of the source it writes as.
	Source  string
	BaseURL string
	// Settings are the kind's non-secret settings (a client's identifier, a directory's tenant).
	Settings map[string]string
	// AllowedHosts are hosts it may reach besides its base address's and the kind's own, such as
	// a cloud's compute endpoint its catalogue names (research itsm-cmdb-ai C6).
	AllowedHosts []string
	// Every is the period in minutes of a pull; zero runs only when asked.
	Every   int
	Enabled bool
	// Collection is where an alert intake opens an entry for a firing alert; zero records the
	// alert only.
	Collection shared.ID
	// RunAs is the person whose rights a scheduled run and an intake act with: the one who last
	// defined it.
	RunAs         shared.ID
	HasCredential bool
	Cursor        string
	NextRunAt     *time.Time
	LastRunAt     *time.Time
	LastRun       shared.ID
	LastOutcome   ConnectorOutcome
	// LastError is the last failure's message code.
	LastError string
	Failures  int
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// ConnectorInput is what a person writes for a connector.
type ConnectorInput struct {
	ID           shared.ID
	TenantID     shared.ID
	HubID        shared.ID
	Key          string
	Kind         ConnectorKind
	Source       Source
	BaseURL      string
	Settings     map[string]string
	AllowedHosts []string
	Every        int
	Enabled      bool
	Collection   shared.ID
	By           shared.ID
	Now          time.Time
}

// NewConnector checks a connector and, for a change, keeps what only runs write.
func NewConnector(in ConnectorInput, existing *Connector) (Connector, error) {
	if !keyPattern.MatchString(in.Key) {
		return Connector{}, invalid("/key", "cmdb.connector_key_malformed")
	}
	spec, ok := SpecOf(in.Kind)
	if !ok {
		return Connector{}, invalid("/kind", "cmdb.connector_kind_unknown")
	}
	if existing != nil && existing.Kind != in.Kind {
		return Connector{}, invalid("/kind", "cmdb.connector_kind_fixed")
	}
	if in.Source.Kind != SourceConnector {
		return Connector{}, invalid("/source", "cmdb.connector_source_not_connector", "key", in.Source.Key)
	}
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if !spec.Push {
		if base == "" {
			return Connector{}, invalid("/base_url", "cmdb.connector_base_url_required")
		}
		if _, err := hostOf(base); err != nil {
			return Connector{}, err
		}
	} else {
		base = ""
	}
	settings := map[string]string{}
	for k, v := range in.Settings {
		v = strings.TrimSpace(v)
		if !keyPattern.MatchString(k) || len(v) > MaxConnectorSetting {
			return Connector{}, invalid("/settings/"+k, "cmdb.connector_setting_invalid")
		}
		if v != "" {
			settings[k] = v
		}
	}
	for _, k := range spec.Settings {
		if settings[k] == "" {
			return Connector{}, invalid("/settings/"+k, "cmdb.connector_setting_required", "key", k)
		}
	}
	if spec.Authority != "" {
		authority := strings.TrimRight(settings[spec.Authority], "/")
		if _, err := hostOf(authority); err != nil {
			return Connector{}, invalid("/settings/"+spec.Authority, "cmdb.connector_base_url_invalid")
		}
		settings[spec.Authority] = authority
	}
	if len(in.AllowedHosts) > MaxConnectorHosts {
		return Connector{}, invalid("/allowed_hosts", "cmdb.connector_hosts_too_many", "maximum", "10")
	}
	hosts := make([]string, 0, len(in.AllowedHosts))
	for _, h := range in.AllowedHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || strings.ContainsAny(h, "/:@ *") {
			return Connector{}, invalid("/allowed_hosts", "cmdb.connector_host_invalid")
		}
		hosts = append(hosts, h)
	}
	slices.Sort(hosts)
	hosts = slices.Compact(hosts)
	if in.Every != 0 && (in.Every < MinConnectorEvery || in.Every > MaxConnectorEvery) {
		return Connector{}, invalid("/every_minutes", "cmdb.connector_every_invalid")
	}
	if spec.Push && in.Every != 0 {
		return Connector{}, invalid("/every_minutes", "cmdb.connector_push_not_scheduled")
	}
	if !spec.Push && !in.Collection.IsZero() {
		return Connector{}, invalid("/collection_id", "cmdb.connector_collection_push_only")
	}
	c := Connector{ID: in.ID, TenantID: in.TenantID, HubID: in.HubID, CreatedAt: in.Now, Version: 1}
	if existing != nil {
		c = *existing
		c.Version++
		if c.HubID != in.HubID {
			return Connector{}, invalid("/hub_id", "cmdb.connector_hub_fixed")
		}
	}
	c.Key, c.Kind, c.Source, c.BaseURL, c.Settings, c.AllowedHosts = in.Key, in.Kind, in.Source.Key, base, settings, hosts
	c.Every, c.Enabled, c.Collection, c.RunAs, c.UpdatedAt = in.Every, in.Enabled, in.Collection, in.By, in.Now
	if in.Enabled && existing != nil && !existing.Enabled {
		c.Failures = 0
	}
	c.NextRunAt = nil
	if c.Enabled && c.Every > 0 {
		next := in.Now
		if existing != nil && existing.LastRunAt != nil {
			next = maxTime(in.Now, existing.LastRunAt.Add(time.Duration(c.Every)*time.Minute))
		}
		c.NextRunAt = &next
	}
	return c, nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func hostOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	// https only: the credential travels with every call.
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" {
		return "", invalid("/base_url", "cmdb.connector_base_url_invalid")
	}
	return strings.ToLower(u.Hostname()), nil
}

// Spec is the connector's kind's spec.
func (c Connector) Spec() ConnectorSpec {
	s, _ := SpecOf(c.Kind)
	return s
}

// Hosts are every host the connector may reach: its base address's, its identity provider's and
// those it allows (research itsm-cmdb-ai C6).
func (c Connector) Hosts() []string {
	var out []string
	if h, err := hostOf(c.BaseURL); err == nil {
		out = append(out, h)
	}
	if a := c.Spec().Authority; a != "" {
		if h, err := hostOf(c.Settings[a]); err == nil {
			out = append(out, h)
		}
	}
	out = append(out, c.AllowedHosts...)
	slices.Sort(out)
	return slices.Compact(out)
}

// Due reports whether a scheduled pull is owed at a moment.
func (c Connector) Due(now time.Time) bool {
	return c.Enabled && c.Every > 0 && c.NextRunAt != nil && !c.NextRunAt.After(now)
}

// Ran records a pull: its run, its outcome, the cursor it reached and when it is owed next. A
// failure keeps the cursor, so the next run fetches the same changes again; after
// MaxConnectorFailures in a row a scheduled connector switches itself off.
func (c Connector) Ran(now time.Time, run shared.ID, outcome ConnectorOutcome, errorCode, cursor string, retryAt *time.Time) Connector {
	next := c
	next.LastRunAt, next.LastOutcome, next.LastError = &now, outcome, errorCode
	if !run.IsZero() {
		next.LastRun = run
	}
	if outcome == ConnectorFailed {
		next.Failures++
	} else {
		next.Failures, next.Cursor = 0, cursor
	}
	next.NextRunAt = nil
	if next.Every > 0 && next.Enabled {
		if next.Failures >= MaxConnectorFailures {
			next.Enabled = false
			return next
		}
		at := now.Add(time.Duration(next.Every) * time.Minute)
		if retryAt != nil && retryAt.After(now) && retryAt.Before(at) {
			at = *retryAt
		}
		next.NextRunAt = &at
	}
	return next
}

// SettingsCopy answers a copy of the connector's settings.
func (c Connector) SettingsCopy() map[string]string { return maps.Clone(c.Settings) }

// AlertStatus is whether a monitored problem is going on.
type AlertStatus string

const (
	AlertFiring   AlertStatus = "FIRING"
	AlertResolved AlertStatus = "RESOLVED"
)

// The bounds of an alert.
const (
	MaxAlertsPerDelivery = 200
	MaxAlertKey          = 200
	MaxAlertTitle        = 300
	MaxAlertSeverity     = 30
	MaxAlertHosts        = 5
)

// AlertSignal is one alert as a monitoring program reports it, in the shape every intake
// translates its program's format into (research itsm-cmdb-ai C10).
type AlertSignal struct {
	// Key deduplicates: the same key is the same problem, firing again or resolved.
	Key      string
	Status   AlertStatus
	Severity string
	// Title is content - the monitoring program's words - and is stored, never logged (rule 10).
	Title string
	// Hosts are what names the affected CI: a host name, an address, a CI's name.
	Hosts []string
	At    *time.Time
}

// CheckSignal refuses a signal that cannot be kept and trims what may be shortened.
func CheckSignal(s AlertSignal) (AlertSignal, error) {
	s.Key = strings.TrimSpace(s.Key)
	if s.Key == "" || len(s.Key) > MaxAlertKey {
		return AlertSignal{}, invalid("/alerts", "cmdb.alert_key_invalid")
	}
	if s.Status != AlertFiring && s.Status != AlertResolved {
		return AlertSignal{}, invalid("/alerts", "cmdb.alert_status_unknown")
	}
	s.Title = truncate(strings.TrimSpace(s.Title), MaxAlertTitle)
	s.Severity = truncate(strings.ToUpper(strings.TrimSpace(s.Severity)), MaxAlertSeverity)
	hosts := make([]string, 0, len(s.Hosts))
	for _, h := range s.Hosts {
		if h = strings.TrimSpace(h); h != "" && len(hosts) < MaxAlertHosts {
			hosts = append(hosts, h)
		}
	}
	s.Hosts = hosts
	return s, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// AlertLookups are the identifier pairs an alert's hosts are looked up by, in order: a host name
// (an instance's port and a domain stripped as a fallback), an address, a CI's name.
func AlertLookups(s AlertSignal) []map[string]string {
	var out []map[string]string
	for _, h := range s.Hosts {
		v := strings.ToLower(h)
		if host, _, ok := strings.Cut(v, ":"); ok && !strings.Contains(host, "[") {
			v = host
		}
		pairs := map[string]string{"hostname": v, "ip_address": v, FieldName: v}
		out = append(out, pairs)
		if short, _, ok := strings.Cut(v, "."); ok && short != "" && !isAddress(v) {
			out = append(out, map[string]string{"hostname": short, FieldName: short})
		}
	}
	return out
}

func isAddress(v string) bool {
	return strings.Trim(v, "0123456789.") == ""
}

// Alert is a monitored problem as Hubtask keeps it: one per connector and key, bound to the CI it
// names, with the entry opened for it.
type Alert struct {
	ID         shared.ID
	TenantID   shared.ID
	HubID      shared.ID
	Connector  shared.ID
	Key        string
	Status     AlertStatus
	Severity   string
	Title      string
	CI         shared.ID
	Entry      shared.ID
	FirstAt    time.Time
	LastAt     time.Time
	ResolvedAt *time.Time
	Count      int
}

// AlertChange is what a signal did to an alert.
type AlertChange string

const (
	AlertOpened    AlertChange = "OPENED"
	AlertRepeated  AlertChange = "REPEATED"
	AlertReopened  AlertChange = "REOPENED"
	AlertClosed    AlertChange = "RESOLVED"
	AlertUnchanged AlertChange = "UNCHANGED"
)

// Receive applies a signal to the alert its key names (zero for a new one) and answers what
// changed. A resolution of a problem never seen records nothing.
func Receive(a Alert, s AlertSignal, now time.Time) (Alert, AlertChange) {
	next := a
	if s.Title != "" {
		next.Title = s.Title
	}
	if s.Severity != "" {
		next.Severity = s.Severity
	}
	next.LastAt = now
	switch {
	case a.ID.IsZero() && s.Status == AlertResolved:
		return a, AlertUnchanged
	case a.ID.IsZero():
		next.Key, next.Status, next.FirstAt, next.Count = s.Key, AlertFiring, now, 1
		return next, AlertOpened
	case s.Status == AlertResolved && a.Status == AlertResolved:
		return a, AlertUnchanged
	case s.Status == AlertResolved:
		next.Status, next.ResolvedAt = AlertResolved, &now
		return next, AlertClosed
	case a.Status == AlertResolved:
		next.Status, next.ResolvedAt, next.Count = AlertFiring, nil, a.Count+1
		return next, AlertReopened
	}
	next.Count = a.Count + 1
	return next, AlertRepeated
}
