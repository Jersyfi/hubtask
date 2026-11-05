// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build load

package load

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
)

// The CMDB views that judge a hub's inventory as one set - its health, its software and its
// service map - read the whole inventory in each request, up to cmdb.MaxCIsPerHub. The bound
// keeps that read finite; this run measures what it costs at the bound (test/load/PLAN.md, C1).

// cmdbServices, cmdbSoftware and the servers between them fill one hub to the ceiling: every
// server runs one service's work, and every software CI is installed on a server.
const (
	cmdbServices = 200
	cmdbSoftware = 4_000
)

var cmdbPaths = []capacityPath{
	{name: "cmdb_health", method: http.MethodGet,
		path: func(as tenant) string { return "/api/v1/hubs/" + as.hub + "/cmdb-health" }},
	{name: "cmdb_software", method: http.MethodGet,
		path: func(as tenant) string { return "/api/v1/hubs/" + as.hub + "/software" }},
	{name: "cmdb_service_map", method: http.MethodGet,
		path: func(as tenant) string { return "/api/v1/hubs/" + as.hub + "/service-map" }},
}

// CmdbFinding is one run of the CMDB views at the ceiling.
type CmdbFinding struct {
	Test           string         `json:"test"`
	RanAt          time.Time      `json:"ran_at"`
	Hardware       string         `json:"hardware"`
	VCPUs          int            `json:"vcpus"`
	CIs            int            `json:"cis"`
	Relations      int            `json:"relations"`
	Paths          []CapacityPath `json:"paths"`
	PeakResident   int64          `json:"peak_resident_bytes"`
	MemoryLimit    int64          `json:"memory_limit_bytes"`
	MemoryObserved bool           `json:"memory_observed"`
}

// TestCmdbViewsAtTheHubCeiling seeds the largest tenant's hub with MaxCIsPerHub CIs and holds each
// whole-inventory view under the capacity run's closed loop. It asserts only that every view
// answered without a server error; the figures are written as evidence, never invented.
func TestCmdbViewsAtTheHubCeiling(t *testing.T) {
	if os.Getenv(envCapacity) == "" {
		t.Skip(envCapacity + " is not set; run `make capacity`")
	}
	stack := runningStack(t)
	as := stack.tenants[0]
	relations := seedInventory(t, stack, as)

	finding := CmdbFinding{
		Test: "cmdb_ceiling", RanAt: time.Now().UTC(), Hardware: hardware(), VCPUs: runtime.NumCPU(),
		CIs: cmdb.MaxCIsPerHub, Relations: relations, MemoryLimit: memoryLimitBytes,
	}
	watching, stopWatching := context.WithCancel(context.Background())
	defer stopWatching()
	peak := watchMemory(watching, stack.serverPID)
	for _, path := range cmdbPaths {
		measured := measurePath(t, stack, as, path)
		if measured.ServerErrors > 0 || measured.Latency.Count == 0 {
			t.Errorf("%s: %d server errors over %d answers", path.name, measured.ServerErrors, measured.Latency.Count)
		}
		t.Logf("%s: %.1f/s, p50 %d ms, p95 %d ms, p99 %d ms over %d answers", path.name, measured.PerSecond,
			measured.Latency.P50, measured.Latency.P95, measured.Latency.P99, measured.Latency.Count)
		finding.Paths = append(finding.Paths, measured)
	}
	finding.PeakResident, finding.MemoryObserved = peak.read()
	t.Logf("peak resident %d bytes (observed %v) against a limit of %d", finding.PeakResident,
		finding.MemoryObserved, finding.MemoryLimit)
	writeEvidence(t, "cmdb-ceiling.json", finding)
}

// seedInventory switches the assets module on for the workspace and writes the hub's inventory
// and its relations straight into the tables, as the dataset's seed writes entries. It answers
// how many relations it wrote.
func seedInventory(t *testing.T, stack *stack, as tenant) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	account := derived(ctx, "account", 0)
	servers := cmdb.MaxCIsPerHub - cmdbServices - cmdbSoftware
	firstServer, firstSoftware := cmdbServices+1, cmdbServices+servers+1
	ci := func(n string) string { return "md5('" + as.hub + "/ci/' || " + n + ")::uuid" }
	statements := []string{
		fmt.Sprintf(`INSERT INTO setting_value (id, tenant_id, level, scope_id, key, value, updated_at)
			VALUES (gen_random_uuid(), '%[1]s', 'WORKSPACE', '%[1]s', 'module.assets', 'true', now())
			ON CONFLICT (tenant_id, level, scope_id, key) DO UPDATE SET value = 'true'`, as.id),
		fmt.Sprintf(`INSERT INTO cmdb_ci (id, tenant_id, hub_id, class_key, name, stage, status, attributes,
				owner_account_id, last_seen_at, created_by, created_at, updated_by, updated_at)
			SELECT %[1]s, '%[2]s', '%[3]s',
			       CASE WHEN n <= %[5]d THEN 'application_service' WHEN n < %[6]d THEN 'server' ELSE 'software' END,
			       CASE WHEN n <= %[5]d THEN 'service ' || n WHEN n < %[6]d THEN 'srv-' || n
			            ELSE 'product ' || (n %% 50) END,
			       'IN_USE', 'OK',
			       CASE WHEN n <= %[5]d THEN '{}'::jsonb
			            WHEN n < %[6]d THEN jsonb_build_object('hostname', 'srv-' || n, 'manufacturer', 'Acme',
			                                                   'serial_number', 'SN-' || n)
			            ELSE jsonb_build_object('publisher', 'Vendor ' || (n %% 50), 'version', '1.' || (n %% 80)) END,
			       '%[4]s', now(), '%[4]s', now(), '%[4]s', now()
			FROM generate_series(1, %[7]d) AS n`,
			ci("n"), as.id, as.hub, account, cmdbServices, firstSoftware, cmdb.MaxCIsPerHub),
		fmt.Sprintf(`INSERT INTO cmdb_relation (id, tenant_id, hub_id, type_key, source_ci_id, target_ci_id,
				data_source, first_seen, last_seen, created_by)
			SELECT gen_random_uuid(), '%[2]s', '%[3]s', 'runs_on', %[1]s, %[5]s, 'load', now(), now(), '%[4]s'
			FROM generate_series(%[6]d, %[7]d) AS s`,
			ci("((s - 1) % "+fmt.Sprint(cmdbServices)+" + 1)"), as.id, as.hub, account, ci("s"),
			firstServer, firstSoftware-1),
		fmt.Sprintf(`INSERT INTO cmdb_relation (id, tenant_id, hub_id, type_key, source_ci_id, target_ci_id,
				data_source, first_seen, last_seen, created_by)
			SELECT gen_random_uuid(), '%[2]s', '%[3]s', 'installed_on', %[1]s, %[5]s, 'load', now(), now(), '%[4]s'
			FROM generate_series(%[6]d, %[7]d) AS s`,
			ci("s"), as.id, as.hub, account, ci("("+fmt.Sprint(firstServer)+" + s % "+fmt.Sprint(servers)+")"),
			firstSoftware, cmdb.MaxCIsPerHub),
		"VACUUM (ANALYZE, PARALLEL 0) cmdb_ci, cmdb_relation, setting_value",
	}
	for _, statement := range statements {
		if err := psql(ctx, stack.database, statement); err != nil {
			t.Fatalf("seeding the inventory: %v", err)
		}
	}
	return servers + cmdbSoftware
}
