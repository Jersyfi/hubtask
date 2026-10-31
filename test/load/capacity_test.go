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
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/test/load/harness"
)

// envCapacity switches the capacity run on. It is not part of the nightly: at 10⁶ entries the seed
// alone takes longer than the nightly's budget, and the run answers a different question - how the
// read paths hold as a workspace grows, not whether anything got worse (observability-reliability.md
// §13.2, F-10). `make capacity` sets it.
const envCapacity = "HUBTASK_LOAD_CAPACITY"

// What each path is held at: a closed loop of capacityWorkers callers for capacityHold, after a
// warm-up that is not recorded.
const (
	capacityWorkers = 8
	capacityHold    = 30 * time.Second
	capacityWarmUp  = 5 * time.Second
	capacityRate    = 400
	// The mixed run's writers: a few people adding entries while eight read the hub's level, so
	// every read meets a collection whose number of changes moved since its figure was kept.
	mixedWriters   = 2
	mixedWriteRate = 40
)

// capacityPath is one read path the capacity run measures, as a request a tenant makes.
type capacityPath struct {
	name   string
	method string
	path   func(as tenant) string
	body   func(as tenant) string
}

// capacityPaths are the reads that grow with a workspace: a collection's page, a filtered and
// sorted query, a board grouped by column, a count per column over the whole hub, the hub's level.
var capacityPaths = []capacityPath{
	{name: "list_collection", method: http.MethodGet,
		path: func(as tenant) string { return "/api/v1/items?collection_id=" + as.collection + "&page_size=50" }},
	{name: "query_open_by_due", method: http.MethodPost,
		path: func(tenant) string { return "/api/v1/items:query" },
		body: func(as tenant) string {
			return `{"scope":{"container_id":"` + as.collection + `"},` +
				`"filter":{"field":"is_completed","op":"EQ","value":false},` +
				`"sort":[{"field":"due_at","direction":"ASC"}],"page":{"size":50}}`
		}},
	{name: "board_by_completion", method: http.MethodPost,
		path: func(tenant) string { return "/api/v1/items:query" },
		body: func(as tenant) string {
			return `{"scope":{"container_id":"` + as.collection + `"},` +
				`"group_by":{"field":"is_completed","limit_per_group":50}}`
		}},
	{name: "count_hub_by_completion", method: http.MethodPost,
		path: func(tenant) string { return "/api/v1/items:aggregate" },
		body: func(as tenant) string {
			return `{"scope":{"container_id":"` + as.hub + `"},` +
				`"aggregate":{"function":"COUNT","group_by":"is_completed"}}`
		}},
	{name: "hub_level", method: http.MethodGet,
		path: func(as tenant) string { return "/api/v1/containers?parent_id=" + as.hub }},
}

// CapacityFinding is one capacity run: per tenant (largest first, so the decay with entries per
// workspace is read down the list) and per path, what it held.
type CapacityFinding struct {
	Test     string    `json:"test"`
	RanAt    time.Time `json:"ran_at"`
	Hardware string    `json:"hardware"`
	VCPUs    int       `json:"vcpus"`
	Dataset  struct {
		Tenants int `json:"tenants"`
		Items   int `json:"items"`
	} `json:"dataset"`
	Tenants []CapacityTenant `json:"tenants"`
	// Mixed is the hub's level read while entries are written, per measured tenant.
	Mixed []CapacityMixed `json:"mixed"`
	// PeakResident is the server's largest resident set over the measured paths, against the
	// GOMEMLIMIT the run starts it with; MemoryObserved is false where the platform does not say.
	PeakResident   int64 `json:"peak_resident_bytes"`
	MemoryLimit    int64 `json:"memory_limit_bytes"`
	MemoryObserved bool  `json:"memory_observed"`
}

// CapacityTenant is one workspace's paths.
type CapacityTenant struct {
	Rank  int            `json:"rank"`
	Items int            `json:"items"`
	Paths []CapacityPath `json:"paths"`
}

// CapacityMixed is one workspace's hub level read under writes: the read pays the recount a write
// leaves, which the read-only paths never see (UC-WRK-29, migration 0292).
type CapacityMixed struct {
	Rank   int          `json:"rank"`
	Items  int          `json:"items"`
	Reads  CapacityPath `json:"reads"`
	Writes CapacityPath `json:"writes"`
}

// CapacityPath is one path's figures: requests per second per vCPU, and the latencies held.
type CapacityPath struct {
	Name         string          `json:"name"`
	PerSecond    float64         `json:"per_second"`
	PerVCPU      float64         `json:"per_second_per_vcpu"`
	Latency      harness.Latency `json:"latency"`
	ServerErrors int             `json:"server_errors"`
	Refused      int             `json:"refused"`
}

// TestCapacityAtScale measures the read paths at the dataset's size, for the largest, a middle and
// the smallest credentialled tenant. It records and asserts only that every path answered: the
// figures are the release tier's to judge, and a number no run produced is never written
// (observability-reliability.md §13.2).
func TestCapacityAtScale(t *testing.T) {
	if os.Getenv(envCapacity) == "" {
		t.Skip(envCapacity + " is not set; run `make capacity`")
	}
	stack := runningStack(t)
	tenants, items := datasetSize(t)

	finding := CapacityFinding{Test: "capacity", RanAt: time.Now().UTC(), Hardware: hardware(), VCPUs: runtime.NumCPU()}
	finding.Dataset.Tenants, finding.Dataset.Items = tenants, items

	watching, stopWatching := context.WithCancel(context.Background())
	defer stopWatching()
	peak := watchMemory(watching, stack.serverPID)

	ranks := []int{0, len(stack.tenants) / 2, len(stack.tenants) - 1}
	seen := map[int]bool{}
	for _, rank := range ranks {
		if seen[rank] {
			continue
		}
		seen[rank] = true
		as := stack.tenants[rank]
		row := CapacityTenant{Rank: rank, Items: itemsOfRank(rank, tenants, items)}
		for _, path := range capacityPaths {
			measured := measurePath(t, stack, as, path)
			if measured.ServerErrors > 0 || measured.Latency.Count == 0 {
				t.Errorf("rank %d, %s: %d server errors over %d answers", rank, path.name,
					measured.ServerErrors, measured.Latency.Count)
			}
			row.Paths = append(row.Paths, measured)
		}
		finding.Tenants = append(finding.Tenants, row)
		mixed := measureMixed(t, stack, as)
		mixed.Rank, mixed.Items = rank, row.Items
		for _, measured := range []CapacityPath{mixed.Reads, mixed.Writes} {
			if measured.ServerErrors > 0 || measured.Latency.Count == 0 {
				t.Errorf("rank %d, mixed %s: %d server errors over %d answers", rank, measured.Name,
					measured.ServerErrors, measured.Latency.Count)
			}
		}
		finding.Mixed = append(finding.Mixed, mixed)
	}
	finding.PeakResident, finding.MemoryObserved = peak.read()
	finding.MemoryLimit = memoryLimitBytes
	writeEvidence(t, fmt.Sprintf("capacity-%d.json", items), finding)
}

// measurePath holds one path for one tenant and reads what the window recorded.
func measurePath(t *testing.T, stack *stack, as tenant, path capacityPath) CapacityPath {
	t.Helper()
	plan := harness.FlatPlan(capacityRate, capacityWarmUp+capacityHold)
	ctx, cancel := context.WithTimeout(context.Background(), plan.Duration()+10*time.Second)
	defer cancel()

	started := time.Now()
	recorder := harness.NewRecorder(started, plan)
	pacer := harness.NewPacer(ctx, plan, started)
	client := &http.Client{Timeout: 10 * time.Second}

	var wg sync.WaitGroup
	for range capacityWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				body := ""
				if path.body != nil {
					body = path.body(as)
				}
				call(ctx, client, recorder, pacer, stack.baseURL, as, harness.ClassInteractive,
					path.method, path.path(as), body)
			}
		}()
	}
	wg.Wait()

	summary := recorder.Summarise(time.Now())
	perSecond := recorder.Throughput(capacityWarmUp, capacityWarmUp+capacityHold)
	return CapacityPath{
		Name:         path.name,
		PerSecond:    perSecond,
		PerVCPU:      perSecond / float64(runtime.NumCPU()),
		Latency:      recorder.Window(harness.ClassInteractive, capacityWarmUp, capacityWarmUp+capacityHold),
		ServerErrors: summary.ServerErrors(),
		Refused:      recorder.Shed(harness.ClassInteractive),
	}
}

// measureMixed holds the hub's level for one tenant while writers add entries to its collection,
// each side paced and recorded on its own so the read's latency is read apart from the write's.
func measureMixed(t *testing.T, stack *stack, as tenant) CapacityMixed {
	t.Helper()
	readPlan := harness.FlatPlan(capacityRate, capacityWarmUp+capacityHold)
	writePlan := harness.FlatPlan(mixedWriteRate, capacityWarmUp+capacityHold)
	ctx, cancel := context.WithTimeout(context.Background(), readPlan.Duration()+10*time.Second)
	defer cancel()

	started := time.Now()
	reads, writes := harness.NewRecorder(started, readPlan), harness.NewRecorder(started, writePlan)
	readPacer, writePacer := harness.NewPacer(ctx, readPlan, started), harness.NewPacer(ctx, writePlan, started)
	client := &http.Client{Timeout: 10 * time.Second}

	var wg sync.WaitGroup
	for range capacityWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				call(ctx, client, reads, readPacer, stack.baseURL, as, harness.ClassInteractive,
					http.MethodGet, "/api/v1/containers?parent_id="+as.hub, "")
			}
		}()
	}
	for writer := range mixedWriters {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for n := 0; ctx.Err() == nil; n++ {
				call(ctx, client, writes, writePacer, stack.baseURL, as, harness.ClassInteractive,
					http.MethodPost, "/api/v1/items",
					fmt.Sprintf(`{"collection_id":%q,"type":"TASK","title":"mixed w%d n%d"}`, as.collection, writer, n))
			}
		}(writer)
	}
	wg.Wait()

	figures := func(name string, recorder *harness.Recorder) CapacityPath {
		perSecond := recorder.Throughput(capacityWarmUp, capacityWarmUp+capacityHold)
		return CapacityPath{
			Name: name, PerSecond: perSecond, PerVCPU: perSecond / float64(runtime.NumCPU()),
			Latency:      recorder.Window(harness.ClassInteractive, capacityWarmUp, capacityWarmUp+capacityHold),
			ServerErrors: recorder.Summarise(time.Now()).ServerErrors(),
			Refused:      recorder.Shed(harness.ClassInteractive),
		}
	}
	return CapacityMixed{Reads: figures("hub_level_under_writes", reads), Writes: figures("create_entry", writes)}
}

// itemsOfRank is the long tail's share of one tenant, as test/load/seed draws it: rank r of n
// holds a share proportional to 1/(r+1).
func itemsOfRank(rank, tenants, items int) int {
	var total float64
	for r := range tenants {
		total += 1 / float64(r+1)
	}
	return int(float64(items) / total / float64(rank+1))
}
