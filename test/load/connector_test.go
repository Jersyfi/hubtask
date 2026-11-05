// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build load

package load

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/test/load/harness"
)

// envConnector turns S6 on (test/load/PLAN.md); HUBTASK_LOAD_CONNECTOR_DEVICES sizes its source.
const envConnector = "HUBTASK_LOAD_CONNECTOR"

const (
	// connectorDevices is more than one run's 50 pages of 100 take, so the second run continues
	// from the kept cursor.
	connectorDevices = 6_000
	// connectorChanged is the share the third run finds changed: 1 %.
	connectorChangedPercent = 1
	// connectorSecret is the source's API token; the fake refuses any other.
	connectorSecret = "load-netbox-token"
	// pageSLO is SLO-3's write bound, held by each page's interval.
	pageSLO = 300 * time.Millisecond
	// connectorMaxWalkRuns bounds the walk before the change, so a run that keeps nothing cannot
	// spin the scenario.
	connectorMaxWalkRuns = 10
)

// ConnectorFinding is S6's record.
type ConnectorFinding struct {
	Test    string         `json:"test"`
	RanAt   time.Time      `json:"ran_at"`
	Devices int            `json:"devices"`
	Changed int            `json:"changed"`
	Runs    []ConnectorRun `json:"runs"`
	// PageInterval is the time between one page's run record and the next one's, in milliseconds:
	// the page's fetch from the source, its reconciliation and its commit. An upper bound of the
	// page's transaction, which the server does not time on its own.
	PageInterval harness.Latency `json:"page_interval_ms"`
	// ThirdRunCreated names the source records the run after the change made new CIs of, which
	// the bar wants none of.
	ThirdRunCreated []string `json:"third_run_created"`
}

// ConnectorRun is one call of POST /cmdb-connectors/{id}:run.
type ConnectorRun struct {
	Status   int            `json:"status"`
	Outcome  string         `json:"outcome"`
	Error    string         `json:"error,omitempty"`
	Pages    int            `json:"pages"`
	Counts   map[string]int `json:"counts"`
	TotalMS  int64          `json:"total_ms"`
	RunIDs   []string       `json:"run_ids"`
	Requests int            `json:"source_requests"`
}

// S6: a NetBox source on loopback, behind https under a root the server trusts by
// HUBTASK_HTTP_CA_FILE, holding more devices than one run takes. Two runs walk it - the second from
// the kept cursor - and a third, after 1 % changed, touches only those.
func TestS6ConnectorReconciliation(t *testing.T) {
	if os.Getenv(envConnector) == "" {
		t.Skip("S6 runs on request: " + envConnector + "=1")
	}
	devices := envInt(t, "HUBTASK_LOAD_CONNECTOR_DEVICES", connectorDevices)
	stack := runningStack(t)
	defer scenario("S6")()
	// The last credentialled workspace: C1 fills the first one's hub to its ceiling.
	as := stack.tenants[len(stack.tenants)-1]

	source := newNetBoxSource(devices)
	server := httptest.NewUnstartedServer(source)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{sourceCertificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := psql(ctx, stack.database, fmt.Sprintf(`INSERT INTO setting_value (id, tenant_id, level, scope_id, key, value, updated_at)
		VALUES (gen_random_uuid(), '%[1]s', 'WORKSPACE', '%[1]s', 'module.assets', 'true', now())
		ON CONFLICT (tenant_id, level, scope_id, key) DO UPDATE SET value = 'true'`, as.id)); err != nil {
		t.Fatalf("switching assets on: %v", err)
	}
	connectorID := defineConnector(ctx, t, stack, as, server.URL)

	finding := ConnectorFinding{Test: "S6 connector reconciliation", RanAt: time.Now().UTC(), Devices: devices}
	run := func() ConnectorRun {
		before := source.served()
		started := time.Now()
		var answer struct {
			Pages  int            `json:"pages"`
			RunIDs []string       `json:"run_ids"`
			Counts map[string]int `json:"counts"`
			Out    string         `json:"outcome"`
			Error  string         `json:"error"`
		}
		status := loadCall(ctx, t, stack, as, http.MethodPost, "/cmdb-connectors/"+connectorID+":run", map[string]any{}, &answer)
		result := ConnectorRun{Status: status, Outcome: answer.Out, Error: answer.Error, Pages: answer.Pages,
			Counts: answer.Counts, TotalMS: time.Since(started).Milliseconds(), RunIDs: answer.RunIDs,
			Requests: source.served() - before}
		t.Logf("run %d: %d, %s %s, %d pages in %d ms, %v", len(finding.Runs)+1, status, result.Outcome, result.Error,
			result.Pages, result.TotalMS, result.Counts)
		finding.Runs = append(finding.Runs, result)
		return result
	}

	// Two runs at least, the second from the cursor the first kept; more while the walk is not
	// done, since a run stops short of its 50 pages where the request's deadline comes first.
	created := 0
	for len(finding.Runs) < 2 || (created < devices && len(finding.Runs) < connectorMaxWalkRuns) {
		walked := run()
		if walked.Status != http.StatusOK {
			break
		}
		created += walked.Counts["created"]
	}
	walkRuns := len(finding.Runs)
	finding.Changed = source.change(connectorChangedPercent)
	third := run()

	var intervals []int64
	for _, each := range finding.Runs {
		intervals = append(intervals, pageIntervals(ctx, t, stack, as, each.RunIDs)...)
	}
	finding.PageInterval = harness.Percentiles(intervals)
	t.Logf("a page's interval over %d pages: %+v ms", len(intervals), finding.PageInterval)
	finding.ThirdRunCreated = createdRefs(ctx, t, stack, as, third.RunIDs)
	writeEvidence(t, fmt.Sprintf("connector-s6-%d.json", stack.items), finding)

	for i, each := range finding.Runs {
		if each.Status >= 500 || each.Status != http.StatusOK || each.Outcome != "OK" {
			t.Errorf("run %d answered %d, %s %s", i+1, each.Status, each.Outcome, each.Error)
		}
	}
	// Each run continues where the last ended: every device is created once, and none is read
	// again as unchanged before the source changed.
	unchanged := 0
	for _, each := range finding.Runs[:walkRuns] {
		unchanged += each.Counts["unchanged"]
	}
	if created != devices || unchanged != 0 {
		t.Errorf("the walk's %d runs created %d CIs and found %d unchanged, want %d and 0", walkRuns, created, unchanged, devices)
	}
	if third.Counts["created"] != 0 || third.Counts["updated"] != finding.Changed {
		t.Errorf("the third run created %d and updated %d, want 0 and the %d changed",
			third.Counts["created"], third.Counts["updated"], finding.Changed)
	}
	if finding.PageInterval.P95 > pageSLO.Milliseconds() {
		t.Errorf("a page's interval is %d ms at P95, over SLO-3's %d", finding.PageInterval.P95, pageSLO.Milliseconds())
	}
}

// defineConnector writes the source and the connector the run pulls through, and its credential.
func defineConnector(ctx context.Context, t *testing.T, stack *stack, as tenant, baseURL string) string {
	t.Helper()
	if status := loadCall(ctx, t, stack, as, http.MethodPut, "/cmdb/sources/load_netbox",
		map[string]any{"kind": "CONNECTOR", "rank": 10, "may_create": true}, nil); status != http.StatusOK {
		t.Fatalf("defining the source answered %d", status)
	}
	var connector struct {
		ID string `json:"id"`
	}
	if status := loadCall(ctx, t, stack, as, http.MethodPut, "/hubs/"+as.hub+"/cmdb-connectors/load_netbox",
		map[string]any{"kind": "NETBOX", "source": "load_netbox", "base_url": baseURL, "enabled": true}, &connector); status != http.StatusOK {
		t.Fatalf("defining the connector answered %d", status)
	}
	if status := loadCall(ctx, t, stack, as, http.MethodPut, "/cmdb-connectors/"+connector.ID+"/credential",
		map[string]any{"credential": connectorSecret}, nil); status != http.StatusOK {
		t.Fatalf("setting the credential answered %d", status)
	}
	return connector.ID
}

// pageIntervals reads each page's run record and answers the gaps between their starts.
func pageIntervals(ctx context.Context, t *testing.T, stack *stack, as tenant, runIDs []string) []int64 {
	t.Helper()
	starts := make([]time.Time, 0, len(runIDs))
	for _, id := range runIDs {
		var run struct {
			StartedAt time.Time `json:"started_at"`
		}
		if status := loadCall(ctx, t, stack, as, http.MethodGet, "/ci-runs/"+id, nil, &run); status != http.StatusOK {
			t.Fatalf("reading run %s answered %d", id, status)
		}
		starts = append(starts, run.StartedAt)
	}
	slices.SortFunc(starts, func(a, b time.Time) int { return a.Compare(b) })
	gaps := make([]int64, 0, len(starts))
	for i := 1; i < len(starts); i++ {
		gaps = append(gaps, starts[i].Sub(starts[i-1]).Milliseconds())
	}
	return gaps
}

// createdRefs answers the references the runs' decisions created a CI for.
func createdRefs(ctx context.Context, t *testing.T, stack *stack, as tenant, runIDs []string) []string {
	t.Helper()
	var refs []string
	for _, id := range runIDs {
		var run struct {
			Decisions []struct {
				Ref     string `json:"ref"`
				Outcome string `json:"outcome"`
				Rule    string `json:"rule"`
			} `json:"decisions"`
		}
		if status := loadCall(ctx, t, stack, as, http.MethodGet, "/ci-runs/"+id, nil, &run); status != http.StatusOK {
			t.Fatalf("reading run %s answered %d", id, status)
		}
		for _, decision := range run.Decisions {
			if decision.Outcome == "CREATE" {
				refs = append(refs, decision.Ref)
			}
		}
	}
	return refs
}

// loadCall sends one request with the workspace's session-scoped token and decodes the answer.
func loadCall(ctx context.Context, t *testing.T, stack *stack, as tenant, method, path string, body, into any) int {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request, err := http.NewRequestWithContext(ctx, method, stack.baseURL+"/api/v1"+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+as.walkToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 400 {
		problem, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		t.Logf("%s %s answered %d: %s", method, path, response.StatusCode, problem)
		return response.StatusCode
	}
	if into != nil {
		if err := json.NewDecoder(response.Body).Decode(into); err != nil {
			t.Fatalf("decoding %s %s: %v", method, path, err)
		}
	}
	return response.StatusCode
}

// netBoxSource answers NetBox's two endpoints as the connector reads them: devices by
// last_updated, limit and offset, and since a moment; no virtual machines.
type netBoxSource struct {
	mu       sync.Mutex
	devices  []netBoxDevice
	requests int
}

type netBoxDevice struct {
	id      int
	updated time.Time
	model   string
}

var connectorEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newNetBoxSource(n int) *netBoxSource {
	source := &netBoxSource{devices: make([]netBoxDevice, n)}
	for i := range source.devices {
		source.devices[i] = netBoxDevice{id: i + 1, updated: connectorEpoch.Add(time.Duration(i) * time.Second), model: "M1"}
	}
	return source
}

// change gives every hundredth device a new model and a later moment, and answers how many.
func (s *netBoxSource) change(percent int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := 0
	later := connectorEpoch.Add(time.Duration(len(s.devices)) * time.Second)
	for i := range s.devices {
		if i%(100/percent) == 0 {
			s.devices[i].model, s.devices[i].updated = "M2", later.Add(time.Duration(i)*time.Second)
			changed++
		}
	}
	return changed
}

func (s *netBoxSource) served() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

func (s *netBoxSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	if r.Header.Get("Authorization") != "Token "+connectorSecret {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	results := []map[string]any{}
	if r.URL.Path == "/api/dcim/devices/" {
		query := r.URL.Query()
		limit, _ := strconv.Atoi(query.Get("limit"))
		offset, _ := strconv.Atoi(query.Get("offset"))
		var since time.Time
		if raw := query.Get("last_updated__gte"); raw != "" {
			since, _ = time.Parse(time.RFC3339Nano, raw)
		}
		ordered := slices.Clone(s.devices)
		slices.SortFunc(ordered, func(a, b netBoxDevice) int {
			if c := a.updated.Compare(b.updated); c != 0 {
				return c
			}
			return a.id - b.id
		})
		var matching []netBoxDevice
		for _, device := range ordered {
			if !device.updated.Before(since) {
				matching = append(matching, device)
			}
		}
		for i := offset; i < len(matching) && i < offset+limit; i++ {
			device := matching[i]
			results = append(results, map[string]any{
				"id": device.id, "name": "load-sw-" + strconv.Itoa(device.id),
				"last_updated": device.updated.Format(time.RFC3339Nano),
				"device_type":  map[string]any{"manufacturer": "Acme", "model": device.model},
			})
		}
	} else if r.URL.Path != "/api/virtualization/virtual-machines/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}
