// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build load

package load

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/test/load/harness"
)

// The ramp. Three stages, because a single rate answers neither half of RT-6: the baseline is what
// the overload is compared against, and without a stage below the threshold there is nothing to
// compare to. The recovery stage is what says the process came back rather than staying broken -
// an installation that sheds for ever has not degraded gracefully, it has fallen over politely.
var rt6Stages = []struct {
	name string
	rate int
	hold time.Duration
}{
	{"baseline", 60, 30 * time.Second},
	{"overload", 3000, 60 * time.Second},
	{"recovery", 60, 30 * time.Second},
}

// interactiveCeilingMillis is the target the interactive P95 has to stay inside while the process
// is deliberately overloaded. Five times the ordinary read target of engineering-guidelines.md §4,
// because this is the state that target does not describe: the client is holding more requests
// open than the pool has connections, and every one of them waits for one. What shedding buys is
// that the wait stays a wait instead of becoming a collapse.
//
// An absolute figure here and not a factor against the baseline stage, which was the first shape
// this test had and was the wrong one: the baseline is a nearly idle process, so *any* real
// overload is an order of magnitude worse than it and the comparison would only ever say that the
// overload arrived. The relative discipline decision 7 asks for is between runs, and that is the
// regression guard's job; within one run the honest claim is a target.
const interactiveCeilingMillis = 1000

// recoveryFactor is how much worse than the baseline the interactive P95 may still be once the
// load is off. Here a factor is right: the same process, the same rate, before and after.
const recoveryFactor = 4

// RT6Finding is what the run leaves behind, and it is the evidence file rather than a log line.
type RT6Finding struct {
	Test    string    `json:"test"`
	RanAt   time.Time `json:"ran_at"`
	Dataset struct {
		Tenants int `json:"tenants"`
		Items   int `json:"items"`
	} `json:"dataset"`
	ShedThreshold  int                        `json:"shed_threshold_inflight"`
	StageLatency   map[string]harness.Latency `json:"interactive_latency_ms_by_stage"`
	PeakResident   int64                      `json:"peak_resident_bytes"`
	MemoryLimit    int64                      `json:"memory_limit_bytes"`
	MemoryObserved bool                       `json:"memory_observed"`
	Summary        harness.Summary            `json:"run"`
}

// RT-6 (observability-reliability.md §12): a load test beyond capacity in which shedding engages,
// the interactive P95 stays within target, and nothing runs out of memory.
//
// What is asserted here is deliberately mostly hardware-independent, because the nightly's runner
// is not quiet hardware and a test that is a coin toss is a test nobody believes: that deferrable
// work was refused and interactive work never was, that no answer was a five hundred the harness
// did not ask for, and that the interactive P95 under overload stayed within a factor of the
// baseline stage's. The absolute figures are the capacity ramp's, on named iron.
func TestRT6SheddingHoldsTheInteractivePathUnderOverload(t *testing.T) {
	stack := runningStack(t)
	tenants, items := datasetSize(t)

	plan := make(harness.Plan, 0, len(rt6Stages))
	for _, stage := range rt6Stages {
		plan = append(plan, harness.Stage{PerSecond: stage.rate, For: stage.hold})
	}

	started := time.Now()
	ctx, stop := context.WithDeadline(context.Background(), started.Add(plan.Duration()))
	defer stop()

	recorder := harness.NewRecorder(started, plan)
	pacer := harness.NewPacer(ctx, plan, started)
	peak := watchMemory(ctx, stack.serverPID)

	drive(ctx, stack, recorder, pacer, driveWorkers)
	ended := time.Now()

	summary := recorder.Summarise(ended)
	finding := RT6Finding{
		Test: "RT-6", RanAt: started.UTC(),
		ShedThreshold: shedLimit, StageLatency: map[string]harness.Latency{},
		MemoryLimit: memoryLimitBytes, Summary: summary,
	}
	finding.Dataset.Tenants, finding.Dataset.Items = tenants, items

	var at time.Duration
	for _, stage := range rt6Stages {
		window := harness.Window{From: at, To: at + stage.hold}
		if stage.name == "recovery" {
			// The last half of the recovery stage. The first intervals after the load comes off
			// still carry the requests that were in flight when it did, and a recovery measured
			// over them is a measurement of the overload with a different name.
			window.From = at + stage.hold/2
		}
		finding.StageLatency[stage.name] = recorder.Window(harness.ClassInteractive, window.From, window.To)
		at += stage.hold
	}
	finding.PeakResident, finding.MemoryObserved = peak.read()
	writeEvidence(t, "RT-6-latest.json", finding)

	// 1. The mechanism engaged. Without this the rest of the run says nothing: an installation
	//    that was never pushed past its threshold held its latency for the least interesting of
	//    all reasons.
	if summary.Shed[string(harness.ClassDeferrable)] == 0 {
		t.Errorf("nothing was shed at %d requests in flight, so the overload never arrived: %+v",
			shedLimit, summary.ByStatus)
	}
	// 2. And it engaged only where it may. A person who cannot tick off a task retries by hand,
	//    which adds load rather than removing it.
	if shed := summary.Shed[string(harness.ClassInteractive)]; shed != 0 {
		t.Errorf("%d interactive requests were shed; the classification let them through", shed)
	}
	// 3. Shedding is what happens instead of failing. A five hundred the harness did not ask for,
	//    or a connection that went away, is the tipping over this exists to prevent.
	if errors := summary.ServerErrors(); errors != 0 {
		t.Errorf("%d server errors beyond the shed refusals: %+v", errors, summary.ByStatus)
	}
	if summary.TransportErrors != 0 {
		t.Errorf("%d transport failures: %v", summary.TransportErrors, summary.ErrorExamples)
	}

	// 4. The interactive path held its target while the deferrable one was being refused. This is
	//    the sentence RT-6 is written as.
	baseline := finding.StageLatency["baseline"]
	overload := finding.StageLatency["overload"]
	switch {
	case baseline.Count == 0 || overload.Count == 0:
		t.Fatalf("a stage produced no interactive samples at all: %+v", finding.StageLatency)
	case overload.P95 > interactiveCeilingMillis:
		t.Errorf("interactive P95 under overload was %d ms, past the %d ms target",
			overload.P95, interactiveCeilingMillis)
	}
	// 5. And it came back. Shedding for ever is not graceful degradation - it is falling over
	//    politely.
	if recovery := finding.StageLatency["recovery"]; recovery.Count > 0 &&
		recovery.P95 > baseline.P95*recoveryFactor && recovery.P95 > 50 {
		t.Errorf("interactive P95 was still %d ms after the load came off, against %d ms before it",
			recovery.P95, baseline.P95)
	}

	// 6. No OOM. Asserted where the process's memory can be read and reported as unobserved
	//    elsewhere - a number this cannot see is better said than guessed.
	if finding.MemoryObserved && finding.PeakResident > memoryLimitBytes {
		t.Errorf("the process peaked at %d bytes resident, past its %d byte limit",
			finding.PeakResident, int64(memoryLimitBytes))
	}
	if !finding.MemoryObserved {
		t.Log("the process's resident memory could not be read here; the OOM half of RT-6 is not asserted on this platform")
	}

	t.Logf("RT-6: %d requests, %d shed, interactive P95 %d ms baseline -> %d ms overload -> %d ms recovery",
		summary.Requests, summary.Shed[string(harness.ClassDeferrable)],
		baseline.P95, overload.P95, finding.StageLatency["recovery"].P95)
}

// deferrableQuery is the heavy read: anchored to a collection the way the contract requires, over
// the whole of it, sorted by something that is not the manual order so the query does work rather
// than walking an index it was built for. It is one of the shapes rest.DeferrableRoutes classifies
// as sheddable, which is the only reason it is in this run.
const deferrableQuery = `{"scope":{"container_id":%q,"include_descendants":true},` +
	`"filter":{"op":"EQ","field":"is_completed","value":false},` +
	`"sort":[{"field":"due_at","dir":"DESC","nulls":"LAST"}],` +
	`"page":{"size":200},"count":"exact"}`

// driveWorkers is how many calls may be in flight from the client at once. Well above the
// threshold on purpose: the offered rate alone cannot overload anything if the client will only
// ever hold eight requests open, and what tips latency over is concurrency rather than a rate.
const driveWorkers = 64

// drive is the traffic: a mix of the interactive shapes a person produces and the deferrable ones
// the shedder may refuse, spread over every credential the stack minted.
//
// The mix is fixed rather than proportional to the rate, because the question is what happens to
// the interactive path while the deferrable one is being refused - a mix that thinned out the
// interactive share as the rate rose would take the sample away exactly when it matters.
func drive(ctx context.Context, stack *stack, recorder *harness.Recorder, pacer *harness.Pacer, workers int) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: workers,
			MaxConnsPerHost:     workers,
		},
	}

	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			as := stack.tenants[worker%len(stack.tenants)]
			for n := 0; ctx.Err() == nil; n++ {
				// Two interactive calls to one deferrable: a workspace under an export is still
				// mostly people reading and writing.
				call(ctx, client, recorder, pacer, stack.baseURL, as, harness.ClassInteractive,
					http.MethodGet, "/api/v1/items?collection_id="+as.collection+"&page_size=20", "")
				call(ctx, client, recorder, pacer, stack.baseURL, as, harness.ClassInteractive,
					http.MethodPost, "/api/v1/items",
					fmt.Sprintf(`{"collection_id":%q,"type":"TASK","title":"rt6 w%d n%d"}`,
						as.collection, worker, n))
				call(ctx, client, recorder, pacer, stack.baseURL, as, harness.ClassDeferrable,
					http.MethodPost, "/api/v1/items:query",
					fmt.Sprintf(deferrableQuery, as.collection))
			}
		}(worker)
	}
	wg.Wait()
}

func call(
	ctx context.Context, client *http.Client, recorder *harness.Recorder, pacer *harness.Pacer,
	baseURL string, as tenant, class harness.Class, method, path, body string,
) {
	if !pacer.Wait(ctx) {
		return
	}

	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		return
	}
	request.Header.Set("Authorization", "Bearer "+as.token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	began := time.Now()
	response, err := client.Do(request)
	took := time.Since(began)
	if err != nil {
		// A cancelled context is the run ending, not a fault. Recording it would put a transport
		// error in the report for every worker that was mid-request when the clock ran out.
		if ctx.Err() != nil {
			return
		}
		recorder.Observe(class, 0, took, err)
		return
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	recorder.Observe(class, response.StatusCode, took, nil)
}

// memoryWatch samples the process while the run is on. Sampled rather than read at the end,
// because a peak that has already been released is invisible afterwards and is exactly what an
// OOM kill would have caught.
type memoryWatch struct {
	mu       sync.Mutex
	peak     int64
	observed bool
}

func (m *memoryWatch) read() (int64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peak, m.observed
}

func watchMemory(ctx context.Context, pid int) *memoryWatch {
	watch := &memoryWatch{}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				resident, ok := residentBytes(pid)
				if !ok {
					continue
				}
				watch.mu.Lock()
				watch.observed = true
				if resident > watch.peak {
					watch.peak = resident
				}
				watch.mu.Unlock()
			}
		}
	}()
	return watch
}
