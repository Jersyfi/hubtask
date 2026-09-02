// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build load

package load

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/test/load/harness"
)

// The storm's three windows. The quiet tenant's traffic runs through all of them at one steady
// rate; only the storm comes and goes. That is what makes the comparison a comparison - the same
// client, the same calls, the same rate, with and without a neighbour writing as hard as it can.
const (
	stormBaseline = 30 * time.Second
	stormWindow   = 60 * time.Second
	stormSettle   = 30 * time.Second
)

// bulkSize is how many operations one storm request carries. Two hundred rather than the
// contract's five hundred, so that a refusal is a refusal about capacity rather than about the
// bound (C-11).
const bulkSize = 200

// fairnessFactor is how much worse the quiet tenant's interactive P95 may get while its neighbour
// storms. Two, and a factor rather than a millisecond figure for the reason RT-6 gives: the
// nightly's runner varies between runs and an absolute target there would be a coin toss.
//
// Tighter than RT-6's, deliberately. RT-6 is about one tenant overloading the process it is on,
// where the interactive path is expected to get slower; this is about a tenant that is doing
// nothing unusual and must barely notice (H-08, multi-tenancy.md §4).
const fairnessFactor = 2

// StormFinding is what the run leaves behind.
type StormFinding struct {
	Test            string                     `json:"test"`
	RanAt           time.Time                  `json:"ran_at"`
	QuietLatency    map[string]harness.Latency `json:"quiet_tenant_interactive_latency_ms_by_window"`
	BulkAccepted    int64                      `json:"bulk_requests_accepted"`
	BulkShed        int64                      `json:"bulk_requests_shed"`
	RuleRuns        int                        `json:"rule_runs_after_the_storm"`
	WebhookAttempts int64                      `json:"webhook_deliveries_attempted"`
	QuietRun        harness.Summary            `json:"quiet_tenant_run"`
}

// The automation storm of H-11, and the fairness of H-08 asserted rather than eyeballed: one
// tenant writes in bulk, its writes fan into a rule, a webhook subscription and the outbox at
// once, and the tenant next door goes on working.
//
// The claim is about the neighbour, not about the storm. A storm that is refused, throttled or
// slowed is a correct outcome; a neighbour whose reads got slower because of it is not.
func TestTheAutomationStormDoesNotStarveTheTenantNextDoor(t *testing.T) {
	stack := runningStack(t)
	if len(stack.tenants) < 2 {
		t.Skip("the storm needs a tenant to storm and a tenant to be quiet")
	}
	storming, quiet := stack.tenants[0], stack.tenants[1]

	sink, delivered := webhookSink(t)
	armRule(t, stack, storming)
	armWebhook(t, stack, storming, sink.URL)

	total := stormBaseline + stormWindow + stormSettle
	plan := harness.FlatPlan(60, total)
	started := time.Now()
	ctx, stop := context.WithDeadline(context.Background(), started.Add(total))
	defer stop()

	recorder := harness.NewRecorder(started, plan)
	pacer := harness.NewPacer(ctx, plan, started)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		quietTraffic(ctx, stack, quiet, recorder, pacer)
	}()

	accepted, shed := storm(ctx, stack, storming, started)
	wg.Wait()
	ended := time.Now()

	finding := StormFinding{
		Test: "H-11 automation storm", RanAt: started.UTC(),
		QuietLatency: map[string]harness.Latency{
			"before": recorder.Window(harness.ClassInteractive, 0, stormBaseline),
			"storm":  recorder.Window(harness.ClassInteractive, stormBaseline, stormBaseline+stormWindow),
			"after":  recorder.Window(harness.ClassInteractive, stormBaseline+stormWindow, total),
		},
		BulkAccepted: accepted.Load(), BulkShed: shed.Load(),
		RuleRuns: ruleRuns(t, stack, storming), WebhookAttempts: delivered.Load(),
		QuietRun: recorder.Summarise(ended),
	}
	writeEvidence(t, "H-11-storm-latest.json", finding)

	// 1. There was a storm. A run in which the bulk was refused before it did anything would
	//    prove the shedder and nothing about fairness.
	if finding.BulkAccepted == 0 {
		t.Fatalf("no bulk write was accepted, so nothing stormed (%d were shed)", finding.BulkShed)
	}
	// 2. And it fanned out. The three destinations H-11 names are the rule, the subscription and
	//    the outbox behind both; a run where nothing was dispatched is a run against an idle
	//    process wearing a storm's name.
	if finding.RuleRuns == 0 {
		t.Errorf("the bulk writes started no rule run; the storm did not reach the automation")
	}
	if finding.WebhookAttempts == 0 {
		t.Errorf("the subscription received nothing; the storm did not reach the outbox dispatch")
	}

	// 3. The neighbour barely noticed. This is the assertion the whole run exists for.
	before, during := finding.QuietLatency["before"], finding.QuietLatency["storm"]
	switch {
	case before.Count == 0 || during.Count == 0:
		t.Fatalf("the quiet tenant produced no samples in a window: %+v", finding.QuietLatency)
	case during.P95 > before.P95*fairnessFactor && during.P95 > 50:
		t.Errorf("the quiet tenant's P95 went from %d ms to %d ms while its neighbour stormed, past the factor of %d",
			before.P95, during.P95, fairnessFactor)
	}
	// 4. And it was never refused. Shedding is the process's answer to its own load; a tenant that
	//    is behaving must not be the one that pays for it.
	if refused := finding.QuietRun.Shed[string(harness.ClassInteractive)]; refused != 0 {
		t.Errorf("%d of the quiet tenant's interactive calls were shed", refused)
	}
	if errors := finding.QuietRun.ServerErrors(); errors != 0 {
		t.Errorf("the quiet tenant saw %d server errors: %+v", errors, finding.QuietRun.ByStatus)
	}

	t.Logf("storm: %d bulk accepted, %d shed, %d rule runs, %d webhook deliveries; quiet P95 %d -> %d -> %d ms",
		finding.BulkAccepted, finding.BulkShed, finding.RuleRuns, finding.WebhookAttempts,
		before.P95, during.P95, finding.QuietLatency["after"].P95)
}

// quietTraffic is the neighbour: ordinary reads and writes at a steady rate, through the whole
// run. Interactive only - a tenant that is not storming is not exporting either, and mixing a
// deferrable call in would put the shedder's refusals into the sample the fairness claim is made
// from.
func quietTraffic(ctx context.Context, stack *stack, as tenant, recorder *harness.Recorder, pacer *harness.Pacer) {
	client := &http.Client{Timeout: 30 * time.Second}
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := 0; ctx.Err() == nil; n++ {
				call(ctx, client, recorder, pacer, stack.baseURL, as, harness.ClassInteractive,
					http.MethodGet, "/api/v1/items?collection_id="+as.collection+"&page_size=20", "")
				call(ctx, client, recorder, pacer, stack.baseURL, as, harness.ClassInteractive,
					http.MethodPost, "/api/v1/items",
					fmt.Sprintf(`{"collection_id":%q,"type":"TASK","title":"quiet w%d n%d"}`,
						as.collection, worker, n))
			}
		}(worker)
	}
	wg.Wait()
}

// storm is the neighbour nobody wants: bulk writes as fast as they are answered, for the storm
// window only.
//
// Unpaced, deliberately. The rest of the harness paces because it is measuring; this one is the
// load being measured *against*, and a storm that waited politely between requests would not be
// one. It counts its own refusals rather than recording them, because a shed bulk is the
// shedder's correct answer and belongs in no latency sample.
func storm(ctx context.Context, stack *stack, as tenant, started time.Time) (accepted, shed *atomic.Int64) {
	accepted, shed = &atomic.Int64{}, &atomic.Int64{}

	select {
	case <-ctx.Done():
		return accepted, shed
	case <-time.After(time.Until(started.Add(stormBaseline))):
	}

	stormCtx, stop := context.WithDeadline(ctx, started.Add(stormBaseline+stormWindow))
	defer stop()

	client := &http.Client{Timeout: 60 * time.Second}
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := 0; stormCtx.Err() == nil; n++ {
				status := send(stormCtx, client, stack.baseURL, as,
					http.MethodPost, "/api/v1/items:bulk", bulkBody(as.collection, worker, n), nil)
				switch {
				case status == http.StatusServiceUnavailable:
					shed.Add(1)
				case status >= 200 && status < 300:
					accepted.Add(1)
				}
			}
		}(worker)
	}
	wg.Wait()
	return accepted, shed
}

func bulkBody(collection string, worker, round int) string {
	var b strings.Builder
	b.WriteString(`{"operations":[`)
	for i := range bulkSize {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"op":"CREATE_ITEM","payload":{"collection_id":%q,"type":"TASK","title":"storm %d-%d-%d"}}`,
			collection, worker, round, i)
	}
	b.WriteString(`]}`)
	return b.String()
}

// armRule puts one rule in the storming tenant's way: every item created starts a run. STOP is the
// action because the fan-out is the point and the work is not - a run row, its events and the
// outbox rows behind them are what a bulk of two hundred creates has to produce two hundred of.
func armRule(t *testing.T, stack *stack, as tenant) {
	t.Helper()
	body := fmt.Sprintf(`{
	  "name": "the storm's rule",
	  "scope": {"type": "COLLECTION", "id": %q},
	  "run_as": %q,
	  "trigger": {"kind": "EVENT", "event_type": "de.hubtask.work.item.created.v1"},
	  "actions": [{"kind": "STOP"}]
	}`, as.collection, accountOf(t, as))

	var created struct {
		ID string `json:"id"`
	}
	if status := send(context.Background(), http.DefaultClient, stack.baseURL, as,
		http.MethodPost, "/api/v1/automation/rules", body, &created); status < 200 || status >= 300 {
		t.Fatalf("writing the rule answered %d", status)
	}
	if created.ID == "" {
		t.Fatal("the rule was written and answered no identifier")
	}

	// A rule is written disabled and armed separately, on purpose: writing what a rule would do
	// and turning it on are two decisions (core/domain/model/automation/Rule.go). The first run of
	// this test skipped the second one and reported a storm that reached no automation at all.
	if status := send(context.Background(), http.DefaultClient, stack.baseURL, as,
		http.MethodPost, "/api/v1/automation/rules/"+created.ID+":enable", "", nil); status < 200 || status >= 300 {
		t.Fatalf("arming the rule answered %d", status)
	}
}

// armWebhook subscribes the sink to the events the storm produces.
func armWebhook(t *testing.T, stack *stack, as tenant, target string) {
	t.Helper()
	body := fmt.Sprintf(`{"target_url":%q,"event_types":["de.hubtask.work.item.created.v1"]}`, target)

	var refusal json.RawMessage
	if status := send(context.Background(), http.DefaultClient, stack.baseURL, as,
		http.MethodPost, "/api/v1/integrations/webhooks", body, &refusal); status < 200 || status >= 300 {
		t.Fatalf("arming the subscription answered %d: %s", status, refusal)
	}
}

// webhookSink is the target: it answers 200 and counts. It answers immediately, because a slow
// recipient is RT-5's subject and would turn this run into that one.
func webhookSink(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var received atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server, &received
}

// ruleRuns asks how many runs the rule produced. Polled, because the dispatch is asynchronous by
// design and asking the instant the load comes off would be asking too early.
func ruleRuns(t *testing.T, stack *stack, as tenant) int {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var answer struct {
			Data []json.RawMessage `json:"data"`
		}
		send(context.Background(), http.DefaultClient, stack.baseURL, as,
			http.MethodGet, "/api/v1/automation/runs?page_size=100", "", &answer)
		if len(answer.Data) > 0 || time.Now().After(deadline) {
			return len(answer.Data)
		}
		time.Sleep(2 * time.Second)
	}
}

// accountOf is the account a rule runs as: the tenant's own, which the dataset gave it.
func accountOf(t *testing.T, as tenant) string {
	t.Helper()
	for rank := range 8 {
		if derived("tenant", rank) == as.id {
			return derived("account", rank)
		}
	}
	t.Fatalf("no account for tenant %s", as.id)
	return ""
}

// send is the unmeasured call: setup, teardown and the storm itself, none of which belongs in a
// latency sample.
func send(ctx context.Context, client *http.Client, baseURL string, as tenant, method, path, body string, into any) int {
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		return 0
	}
	request.Header.Set("Authorization", "Bearer "+as.token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.Do(request)
	if err != nil {
		return 0
	}
	defer func() { _ = response.Body.Close() }()

	payload, _ := io.ReadAll(response.Body)
	if into != nil {
		_ = json.Unmarshal(payload, into)
	}
	return response.StatusCode
}
