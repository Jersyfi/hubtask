// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	connectorport "github.com/Jersyfi/hubtask/core/port/connector"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var incidents = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d006")

type connectorMemory struct {
	repository.Connectors
	conns   map[shared.ID]domain.Connector
	creds   map[shared.ID]repository.Credential
	alerts  map[string]domain.Alert
	digests map[string]bool
}

func newConnectorMemory() *connectorMemory {
	return &connectorMemory{conns: map[shared.ID]domain.Connector{}, creds: map[shared.ID]repository.Credential{},
		alerts: map[string]domain.Alert{}, digests: map[string]bool{}}
}

func (m *connectorMemory) ListConnectors(_ context.Context, h shared.ID) ([]domain.Connector, error) {
	var out []domain.Connector
	for _, c := range m.conns {
		if h.IsZero() || c.HubID == h {
			out = append(out, c)
		}
	}
	return out, nil
}
func (m *connectorMemory) FindConnector(_ context.Context, id shared.ID, _ bool) (domain.Connector, error) {
	c, ok := m.conns[id]
	if !ok {
		return c, shared.ErrNotFound.WithDetail("cmdb.connector_not_found")
	}
	_, c.HasCredential = m.creds[id]
	return c, nil
}
func (m *connectorMemory) CountConnectors(context.Context, shared.ID) (int, error) {
	return len(m.conns), nil
}
func (m *connectorMemory) PutConnector(_ context.Context, c domain.Connector, expected int) error {
	if m.conns[c.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	c.Version = expected + 1
	m.conns[c.ID] = c
	return nil
}
func (m *connectorMemory) SetCredential(_ context.Context, id shared.ID, ct []byte, key string, _ time.Time) error {
	m.creds[id] = repository.Credential{ConnectorID: id, Ciphertext: ct, KeyID: key}
	return nil
}
func (m *connectorMemory) Credential(_ context.Context, id shared.ID) (repository.Credential, error) {
	c, ok := m.creds[id]
	if !ok {
		return c, shared.ErrNotFound
	}
	return c, nil
}
func (m *connectorMemory) RecordRun(_ context.Context, c domain.Connector) error {
	m.conns[c.ID] = c
	return nil
}
func (m *connectorMemory) FindAlert(_ context.Context, conn shared.ID, key string) (domain.Alert, error) {
	a, ok := m.alerts[conn.String()+key]
	if !ok {
		return a, shared.ErrNotFound
	}
	return a, nil
}
func (m *connectorMemory) PutAlert(_ context.Context, a domain.Alert) error {
	m.alerts[a.Connector.String()+a.Key] = a
	return nil
}
func (m *connectorMemory) SeeDigest(_ context.Context, _ shared.ID, d []byte, _, _ time.Time) (bool, error) {
	if m.digests[string(d)] {
		return false, nil
	}
	m.digests[string(d)] = true
	return true, nil
}

// sealer seals by prefixing the purpose, so that a credential moved to another connector does
// not open.
type sealer struct{ crypto.Encryptor }

func (sealer) Seal(_ context.Context, v secret.Secret, p crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: "k1", Ciphertext: []byte(string(p) + "|" + v.Reveal())}, nil
}
func (sealer) Open(_ context.Context, s crypto.Sealed, p crypto.Purpose) (secret.Secret, error) {
	purpose, value, _ := strings.Cut(string(s.Ciphertext), "|")
	if purpose != string(p) {
		return secret.Secret{}, shared.ErrUnavailable.WithDetail("crypto.not_authentic")
	}
	return secret.New(value), nil
}

type drawn struct{}

func (drawn) Bytes(n int) ([]byte, error) { return make([]byte, n), nil }

type jobs struct{ asked []queue.Request }

func (j *jobs) Enqueue(_ context.Context, r queue.Request) (shared.ID, error) {
	j.asked = append(j.asked, r)
	return "", nil
}

// source answers scripted pages and remembers the cursors it was asked from.
type source struct {
	pages   []connectorport.Page
	fail    error
	cursors []string
	secrets []string
}

func (s *source) Pull(_ context.Context, p connectorport.Pull) (connectorport.Page, error) {
	s.cursors, s.secrets = append(s.cursors, p.Cursor), append(s.secrets, p.Credential.Reveal())
	if s.fail != nil {
		return connectorport.Page{}, s.fail
	}
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}

type entries struct {
	calls []string
	ins   []usecase.Input
}

func (e *entries) Invoke(_ context.Context, name string, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	e.calls, e.ins = append(e.calls, name), append(e.ins, in)
	return usecase.Output{"id": "01936f2a-7c1e-7000-8000-00000000e001"}, nil
}

type hubsAndCollections struct{ hubs }

func (h hubsAndCollections) Find(ctx context.Context, id shared.ID) (work.Container, error) {
	if id == incidents {
		return work.Container{ID: id, Type: work.ContainerCollection, ParentID: hub}, nil
	}
	return h.hubs.Find(ctx, id)
}

type connectorFixture struct {
	fixture
	c       Connectors
	store   *connectorMemory
	src     *source
	jobs    *jobs
	entries *entries
}

func newConnectorFixture() connectorFixture {
	f := newFixture()
	f.m.sources = append(f.m.sources, domain.Source{Key: "alerts", Kind: domain.SourceConnector, Rank: 300})
	f.inv.Containers = hubsAndCollections{}
	cf := connectorFixture{fixture: f, store: newConnectorMemory(), src: &source{}, jobs: &jobs{}, entries: &entries{}}
	cf.c = Connectors{Inventory: f.inv, Store: cf.store, Pullers: cf.src, Encryptor: sealer{}, Entropy: drawn{},
		Jobs: cf.jobs, Entries: cf.entries}
	return cf
}

func (f connectorFixture) define(t *testing.T, in usecase.Input) usecase.Output {
	t.Helper()
	out, err := call(t, DefineCiConnector{f.c}.Descriptor(), as(anna), in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func idOf(t *testing.T, out usecase.Output) shared.ID {
	t.Helper()
	id, err := shared.ParseID(out.String("id"))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAConnectorIsScheduledAndItsCredentialIsSealedAndNeverAnswered(t *testing.T) {
	f := newConnectorFixture()
	out := f.define(t, usecase.Input{"hub_id": hub.String(), "key": "nb", "kind": "NETBOX", "source": "netbox",
		"base_url": "https://netbox.example.org", "every_minutes": 60})
	if len(f.jobs.asked) != 1 || f.jobs.asked[0].Kind != queue.KindCmdbConnectorPull || !f.jobs.asked[0].RunAt.Equal(start) {
		t.Fatalf("a scheduled connector seeds its pull: %+v", f.jobs.asked)
	}
	id := idOf(t, out)
	got, err := call(t, SetCiConnectorCredential{f.c}.Descriptor(), as(anna), usecase.Input{"connector_id": id.String(), "credential": "tok"})
	if err != nil || got["has_credential"] != true || got["secret"] != nil {
		t.Fatalf("a pull's credential is never answered: %v %v", got, err)
	}
	if string(f.store.creds[id].Ciphertext) != string(CredentialPurpose(id))+"|tok" {
		t.Fatal("the credential is sealed under the connector's purpose")
	}
	if _, err := call(t, DefineCiConnector{f.c}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "key": "x",
		"kind": "NETBOX", "source": "scan", "base_url": "https://n.example"}); code(err) != "cmdb.connector_source_not_connector" {
		t.Fatalf("a connector writes as a connector source: %v", err)
	}
}

func TestAPullRunsEveryPageThroughTheReconciliationAndKeepsTheCursor(t *testing.T) {
	f := newConnectorFixture()
	id := idOf(t, f.define(t, usecase.Input{"hub_id": hub.String(), "key": "nb", "kind": "NETBOX", "source": "netbox",
		"base_url": "https://netbox.example.org"}))
	if _, err := call(t, RunCiConnector{f.c}.Descriptor(), as(anna), usecase.Input{"connector_id": id.String()}); code(err) != "cmdb.connector_credential_missing" {
		t.Fatalf("a run without a credential: %v", err)
	}
	if _, err := call(t, SetCiConnectorCredential{f.c}.Descriptor(), as(anna), usecase.Input{"connector_id": id.String(), "credential": "tok"}); err != nil {
		t.Fatal(err)
	}
	item := func(n string) domain.Item {
		return domain.Item{Class: "server", Name: "srv-" + n, NativeKey: n, Attributes: map[string]any{"hostname": "srv-" + n}}
	}
	f.src.pages = []connectorport.Page{
		{Payload: domain.Payload{Items: []domain.Item{item("1")}}, Cursor: "c1", More: true},
		{Payload: domain.Payload{Items: []domain.Item{item("2")}}, Cursor: "c2"},
	}
	out, err := call(t, RunCiConnector{f.c}.Descriptor(), as(anna), usecase.Input{"connector_id": id.String()})
	if err != nil {
		t.Fatal(err)
	}
	if out["pages"] != 2 || len(f.m.cis) != 2 || f.src.secrets[0] != "tok" || f.src.cursors[1] != "c1" {
		t.Fatalf("both pages written: %v, %d CIs, cursors %v", out, len(f.m.cis), f.src.cursors)
	}
	if c := f.store.conns[id]; c.Cursor != "c2" || c.LastOutcome != domain.ConnectorOK || len(f.m.runs) != 2 || c.LastRun != f.m.runs[1].ID {
		t.Fatalf("the connector keeps where it ended: %+v", c)
	}
	f.src.fail = shared.ErrUnavailable.WithDetail(connectorport.CodeRateLimited).WithParams(map[string]string{"retry_after": "30"})
	out, err = call(t, RunCiConnector{f.c}.Descriptor(), as(anna), usecase.Input{"connector_id": id.String()})
	if err != nil || out["outcome"] != "FAILED" || out["error"] != connectorport.CodeRateLimited {
		t.Fatalf("a failure is answered by its code: %v %v", out, err)
	}
	if c := f.store.conns[id]; c.Cursor != "c2" || c.Failures != 1 || f.src.cursors[2] != "c2" {
		t.Fatalf("a failure keeps the cursor: %+v", c)
	}
	f.src.fail = nil
	f.src.pages = []connectorport.Page{{Payload: domain.Payload{Items: []domain.Item{item("3")}}}}
	out, err = call(t, RunCiConnector{f.c}.Descriptor(), as(anna), usecase.Input{"connector_id": id.String(), "dry_run": true, "full": true})
	if err != nil || len(f.m.cis) != 2 || f.src.cursors[3] != "" || out["dry_run"] != true {
		t.Fatalf("a dry run writes nothing and a full walk starts over: %v %v", out, err)
	}
	if decisions, _ := out["decisions"].([]usecase.Output); len(decisions) != 1 || decisions[0]["outcome"] != "CREATE" {
		t.Fatalf("a dry run explains: %v", out["decisions"])
	}
}

func TestAScheduledPullRunsAsItsDefinerWhenDue(t *testing.T) {
	f := newConnectorFixture()
	id := idOf(t, f.define(t, usecase.Input{"hub_id": hub.String(), "key": "hz", "kind": "HETZNER", "source": "netbox",
		"base_url": "https://api.hetzner.example", "every_minutes": 30}))
	_, _ = call(t, SetCiConnectorCredential{f.c}.Descriptor(), as(anna), usecase.Input{"connector_id": id.String(), "credential": "tok"})
	f.src.pages = []connectorport.Page{{Payload: domain.Payload{Items: []domain.Item{{Class: "virtual_machine", Name: "vm", NativeKey: "1"}}}}}
	next, err := f.c.RunScheduled(context.Background(), tenant, id)
	if err != nil || !next.Equal(start.Add(30*time.Minute)) || len(f.m.cis) != 1 {
		t.Fatalf("ran and owed in a period: %v %v", next, err)
	}
	for _, ci := range f.m.cis {
		if ci.CreatedBy != anna {
			t.Fatal("a scheduled run writes as the person who defined the connector")
		}
	}
	if again, _ := f.c.RunScheduled(context.Background(), tenant, id); !again.Equal(next) || len(f.src.cursors) != 1 {
		t.Fatal("a connector not yet due is not pulled")
	}
}

func intake(t *testing.T, f connectorFixture) (shared.ID, string) {
	t.Helper()
	id := idOf(t, f.define(t, usecase.Input{"hub_id": hub.String(), "key": "am", "kind": "ALERTMANAGER", "source": "alerts",
		"collection_id": incidents.String()}))
	out, err := call(t, SetCiConnectorCredential{f.c}.Descriptor(), as(anna), usecase.Input{"connector_id": id.String()})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := out["secret"].(string)
	if key == "" {
		t.Fatal("an intake's secret is drawn and answered once")
	}
	return id, key
}

func signals(list ...domain.AlertSignal) AlertParser {
	return func(domain.ConnectorKind, []byte) ([]domain.AlertSignal, error) { return list, nil }
}

func TestAnAlertIntakeAcceptsOnlyItsSecretOrAFreshSignature(t *testing.T) {
	f := newConnectorFixture()
	id, key := intake(t, f)
	fire := signals(domain.AlertSignal{Key: "k", Status: domain.AlertFiring, Title: "down"})
	ctx := context.Background()
	if _, err := f.c.DeliverAlerts(ctx, AlertDelivery{TenantID: tenant, ConnectorID: id, Bearer: "wrong"}, fire); code(err) != "cmdb.intake_not_found" {
		t.Fatalf("a wrong secret: %v", err)
	}
	if _, err := f.c.DeliverAlerts(ctx, AlertDelivery{TenantID: tenant, ConnectorID: id, Bearer: key}, fire); err != nil {
		t.Fatalf("the bearer secret: %v", err)
	}
	body := []byte(`{"alerts":[]}`)
	ts := strconv.FormatInt(start.Unix(), 10)
	signed := AlertDelivery{TenantID: tenant, ConnectorID: id, Signature: Sign(key, ts, body), Timestamp: ts, Body: body}
	if _, err := f.c.DeliverAlerts(ctx, signed, fire); err != nil {
		t.Fatalf("a signed delivery: %v", err)
	}
	if _, err := f.c.DeliverAlerts(ctx, signed, fire); code(err) != "cmdb.intake_not_found" {
		t.Fatalf("a replayed delivery: %v", err)
	}
	stale := strconv.FormatInt(start.Add(-10*time.Minute).Unix(), 10)
	if _, err := f.c.DeliverAlerts(ctx, AlertDelivery{TenantID: tenant, ConnectorID: id, Signature: Sign(key, stale, body), Timestamp: stale, Body: body}, fire); code(err) != "cmdb.intake_not_found" {
		t.Fatalf("a stale signature: %v", err)
	}
}

func TestAnAlertOpensAnEntryBoundToItsCIAndItsResolutionCompletesIt(t *testing.T) {
	f := newConnectorFixture()
	if _, err := call(t, CreateCi{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "class": "server",
		"name": "web01", "attributes": map[string]any{"hostname": "web01"}}); err != nil {
		t.Fatal(err)
	}
	var ci shared.ID
	for id := range f.m.cis {
		ci = id
	}
	id, key := intake(t, f)
	ctx := context.Background()
	fire := signals(domain.AlertSignal{Key: "fp1", Status: domain.AlertFiring, Title: "disk full", Hosts: []string{"web01.example.org:9100"}})
	r, err := f.c.DeliverAlerts(ctx, AlertDelivery{TenantID: tenant, ConnectorID: id, Bearer: key}, fire)
	if err != nil || r.Opened != 1 {
		t.Fatalf("opened: %+v %v", r, err)
	}
	a := f.store.alerts[id.String()+"fp1"]
	if a.CI != ci || a.Entry.IsZero() || f.entries.calls[0] != "CreateWorkItem" ||
		f.entries.ins[0]["collection_id"] != incidents.String() || f.entries.ins[0]["notes"] != "![](hubtask:ci/"+ci.String()+")" {
		t.Fatalf("bound to the CI and its entry: %+v %v", a, f.entries.ins)
	}
	if r, _ = f.c.DeliverAlerts(ctx, AlertDelivery{TenantID: tenant, ConnectorID: id, Bearer: key}, fire); len(f.entries.calls) != 1 || r.Opened != 0 {
		t.Fatal("firing again opens no second entry")
	}
	resolve := signals(domain.AlertSignal{Key: "fp1", Status: domain.AlertResolved})
	if r, err = f.c.DeliverAlerts(ctx, AlertDelivery{TenantID: tenant, ConnectorID: id, Bearer: key}, resolve); err != nil || r.Resolved != 1 ||
		f.entries.calls[1] != "CompleteWorkItem" || f.store.alerts[id.String()+"fp1"].Status != domain.AlertResolved {
		t.Fatalf("resolved: %+v %v %v", r, err, f.entries.calls)
	}
	list, err := call(t, ListCiAlerts{f.c}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String()})
	if rows, _ := list["data"].([]usecase.Output); err != nil || len(rows) != 1 || rows[0]["count"] != 2 {
		t.Fatalf("listed: %v %v", list, err)
	}
	if _, err := f.c.DeliverAlerts(ctx, AlertDelivery{TenantID: tenant, ConnectorID: id, Bearer: key},
		func(domain.ConnectorKind, []byte) ([]domain.AlertSignal, error) { return nil, errors.New("x") }); err == nil {
		t.Fatal("an unreadable delivery is refused")
	}
}

func (m *connectorMemory) ListAlerts(_ context.Context, h shared.ID, status domain.AlertStatus, limit int) ([]domain.Alert, error) {
	var out []domain.Alert
	for _, a := range m.alerts {
		if a.HubID == h && (status == "" || a.Status == status) && len(out) < limit {
			out = append(out, a)
		}
	}
	return out, nil
}

func code(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	if err != nil {
		return err.Error()
	}
	return ""
}
