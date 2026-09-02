// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build load

package load

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/test/load/harness"
)

// The guard's own run: a flat rate, held, with nothing else going on. Deliberately not RT-6's
// ramp - a regression guard wants the same shape every time, and a ramp's numbers depend on where
// in the ramp the machine happened to fall over.
const (
	guardRate     = 200
	guardHold     = 60 * time.Second
	guardWorkers  = 16
	guardWarmUp   = 10 * time.Second
	guardBaseline = "steady-state.json"
)

// envHardware names the machine, and the guard compares only against a baseline recorded on the
// same one. Comparing a run against figures from different iron measures the iron - which is the
// whole reason decision 7 has two tiers rather than one.
const envHardware = "HUBTASK_LOAD_HARDWARE"

// GuardFinding is the measurement, whether or not there was anything to compare it against.
type GuardFinding struct {
	Test     string    `json:"test"`
	RanAt    time.Time `json:"ran_at"`
	Hardware string    `json:"hardware"`
	VCPUs    int       `json:"vcpus"`
	Dataset  struct {
		Tenants int `json:"tenants"`
		Items   int `json:"items"`
	} `json:"dataset"`
	Measured    map[string]float64   `json:"measured"`
	ComparedTo  string               `json:"compared_to"`
	Regressions []harness.Regression `json:"regressions"`
	Run         harness.Summary      `json:"run"`
}

// The cheap tier of decision 7: against a figure recorded on the same kind of machine, did this
// get significantly worse?
//
// It answers nothing else. It produces no absolute capacity number, it publishes nothing, and on
// hardware it has no baseline for it records its measurement and says so rather than comparing
// against figures from a different machine. That the comparison itself works is proved where it
// can be proved cheaply and deterministically - in the harness's own tests, which run in
// gate-unit, gate-selftest style.
func TestTheRegressionGuardComparesThisRunAgainstTheStoredBaseline(t *testing.T) {
	stack := runningStack(t)
	tenants, items := datasetSize(t)

	// The warm-up is outside the measurement: the first requests against a cold pool and an
	// unfilled plan cache are a different system from the one the baseline describes.
	plan := harness.Plan{
		{PerSecond: guardRate, For: guardWarmUp},
		{PerSecond: guardRate, For: guardHold},
	}
	started := time.Now()
	ctx, stop := context.WithDeadline(context.Background(), started.Add(plan.Duration()))
	defer stop()

	recorder := harness.NewRecorder(started, plan)
	pacer := harness.NewPacer(ctx, plan, started)
	guardTraffic(ctx, stack, recorder, pacer)
	ended := time.Now()

	summary := recorder.Summarise(ended)
	held := recorder.Window(harness.ClassInteractive, guardWarmUp, plan.Duration())
	achieved := float64(held.Count) / guardHold.Seconds()

	finding := GuardFinding{
		Test: "H-11 regression guard", RanAt: started.UTC(),
		Hardware: hardware(), VCPUs: runtime.NumCPU(),
		Measured: map[string]float64{
			"interactive_p50_ms":  float64(held.P50),
			"interactive_p95_ms":  float64(held.P95),
			"interactive_p99_ms":  float64(held.P99),
			"requests_per_second": achieved,
			// Recorded and not compared: at an offered rate the installation keeps up with, this
			// is the offered rate divided by the core count, and guarding it would be guarding
			// the flag the run was started with. The capacity figure comes from RT-6's overload
			// stage, where the offered rate is past what the process can serve.
			"requests_per_second_per_vcpu": achieved / float64(runtime.NumCPU()),
		},
		Run: summary,
	}
	finding.Dataset.Tenants, finding.Dataset.Items = tenants, items

	if summary.ServerErrors() != 0 || summary.TransportErrors != 0 {
		t.Errorf("the guard's own run was not clean: %+v, %d transport failures",
			summary.ByStatus, summary.TransportErrors)
	}
	if held.Count == 0 {
		t.Fatal("the held stage produced no samples")
	}

	// Recorded before anything is compared, and written again at the end with the verdict in it.
	// A run whose comparison could not happen at all is still a measurement, and losing it because
	// the baseline was unreadable would be losing the more useful half.
	writeEvidence(t, "H-11-guard-latest.json", finding)
	t.Logf("guard: %.0f req/s over %d vCPU (%.1f per vCPU), interactive P50/P95/P99 %d/%d/%d ms",
		achieved, runtime.NumCPU(), finding.Measured["requests_per_second_per_vcpu"],
		held.P50, held.P95, held.P99)

	baseline, err := harness.LoadBaseline(filepath.Join("baselines", guardBaseline))
	if err != nil {
		t.Fatalf("%v", err)
	}

	switch {
	case baseline.Hardware != hardware():
		// Recorded, not compared. Saying so is the point: a guard that quietly compared across
		// machines would report the machine, and a guard that quietly passed would report
		// nothing at all.
		finding.ComparedTo = ""
		t.Logf("the stored baseline was measured on %q and this is %q, so this run is recorded and not compared",
			baseline.Hardware, hardware())
	case baseline.Dataset.Items != items || baseline.Dataset.Tenants != tenants:
		finding.ComparedTo = ""
		t.Logf("the stored baseline was measured over %d items in %d tenants and this run used %d in %d, so it is recorded and not compared",
			baseline.Dataset.Items, baseline.Dataset.Tenants, items, tenants)
	default:
		finding.ComparedTo = baseline.Name + " (" + baseline.RecordedAt + ")"
		regressions, missing := baseline.Compare(finding.Measured)
		finding.Regressions = regressions
		for _, name := range missing {
			t.Errorf("the baseline carries %s and this run did not measure it", name)
		}
		for _, regression := range regressions {
			t.Errorf("%s", regression)
		}
	}

	writeEvidence(t, "H-11-guard-latest.json", finding)
}

func hardware() string {
	if named := os.Getenv(envHardware); named != "" {
		return named
	}
	return "unnamed-" + runtime.GOOS + "-" + runtime.GOARCH
}

// guardTraffic is the ordinary path and only the ordinary path: a read, a write, a read of one
// entry. No deferrable call, because a refusal is fast and a run whose mix of refusals moved
// between two measurements would move the figure without anything having got faster or slower.
func guardTraffic(ctx context.Context, stack *stack, recorder *harness.Recorder, pacer *harness.Pacer) {
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{MaxIdleConnsPerHost: guardWorkers, MaxConnsPerHost: guardWorkers},
	}

	var wg sync.WaitGroup
	for worker := range guardWorkers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			as := stack.tenants[worker%len(stack.tenants)]
			for n := 0; ctx.Err() == nil; n++ {
				call(ctx, client, recorder, pacer, stack.baseURL, as, harness.ClassInteractive,
					http.MethodGet, "/api/v1/items?collection_id="+as.collection+"&page_size=20", "")
				call(ctx, client, recorder, pacer, stack.baseURL, as, harness.ClassInteractive,
					http.MethodPost, "/api/v1/items",
					fmt.Sprintf(`{"collection_id":%q,"type":"TASK","title":"guard w%d n%d"}`,
						as.collection, worker, n))
				call(ctx, client, recorder, pacer, stack.baseURL, as, harness.ClassInteractive,
					http.MethodGet, "/api/v1/containers", "")
			}
		}(worker)
	}
	wg.Wait()
}
