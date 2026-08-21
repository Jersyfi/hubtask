// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command rt8 puts load on a running installation and reports what it saw, so that RT-8 - a
// rolling update under load, with no 5xx and no data loss (observability-reliability.md §12) - is
// answered with numbers rather than with a look at the pods.
//
// It exists because the two halves of that question cannot be asked afterwards. Whether requests
// were refused is only visible while they are being made, and whether anything was lost is only
// answerable by somebody who knows what was written - so this writes, remembers what it wrote,
// and goes looking for all of it again at the end.
//
// Three things it does deliberately:
//
//   - It counts transport errors separately and treats them as failures. A connection reset while
//     a pod goes away carries no status code at all, and a run that only counted 5xx would call
//     that a success. It is the exact failure a rolling update produces when the readiness gate or
//     the grace period is wrong.
//   - It keeps connections alive. A client that opens a fresh connection per request never meets
//     the problem, because it never holds one to a pod that is about to stop. Reuse is both what
//     real clients do and the harsher test.
//   - It records a timeline. "No 5xx over five minutes" and "no 5xx during the ninety seconds the
//     rollout took" are different claims, and only the second one is evidence.
//
// The endpoints it uses are the ones both versions of a rolling update serve. That is a
// constraint, not a simplification: load that only the new version can answer would report the old
// version's correct refusals as failures.
//
// It is paced, and it spreads itself over several credentials, because the installation's rate
// limits are part of what is deployed and are not turned off to make a test comfortable. Traffic
// that runs into the limiter measures the limiter: a 429 is a correct answer, it never reaches
// the path a rollout could break, and a run full of them would report a quiet rollout it never
// actually tested. So --rate is chosen below the tenant's budget and the tokens are several,
// which is also how real traffic arrives (deployment.md §6.1).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// envToken carries the credentials, comma separated. Read from the environment rather than taken
// as a flag: a flag is visible in `ps` (test/e2e/mint makes the same choice for the same reason).
//
// Several of them because each one carries its own per-token budget. One credential driving the
// whole run would spend that budget in seconds and spend the rest of the run being refused.
const envToken = "HUBTASK_TOKEN" //nolint:gosec // G101: the name of an environment variable.

// requestTimeout bounds one call. Generous, because what is under test is whether a request is
// answered at all - a slow answer during a rollout is a different finding from a refused one, and
// this has to be able to tell them apart (CLAUDE.md rule 7).
const requestTimeout = 30 * time.Second

// tick is the resolution of the timeline. Five seconds is fine enough to place a fault inside a
// rollout and coarse enough that the report stays readable.
const tick = 5 * time.Second

// stagger is how far apart the workers begin.
const stagger = 60 * time.Millisecond

func main() {
	url := flag.String("url", "", "the installation, without a trailing slash")
	collection := flag.String("collection", "", "the collection to write into, as a UUID")
	duration := flag.Duration("duration", 3*time.Minute, "how long to keep the load on")
	workers := flag.Int("workers", 8, "concurrent clients")
	rate := flag.Int("rate", 40, "requests per second, in total - keep it under the tenant's budget")
	out := flag.String("out", "", "write the result as JSON to this file as well")
	flag.Parse()

	if err := run(*url, *collection, os.Getenv(envToken), *duration, *workers, *rate, *out); err != nil {
		fmt.Fprintf(os.Stderr, "rt8: %s\n", err)
		os.Exit(1)
	}
}

// Result is the whole finding, and it is JSON because it ends up quoted in an evidence document.
type Result struct {
	URL             string         `json:"url"`
	StartedAt       time.Time      `json:"started_at"`
	EndedAt         time.Time      `json:"ended_at"`
	Workers         int            `json:"workers"`
	Requests        int            `json:"requests"`
	ByStatus        map[string]int `json:"by_status"`
	TransportErrors int            `json:"transport_errors"`
	// ErrorExamples keeps the first few verbatim. A count says something went wrong; the text
	// says whether it was a reset connection or a name that stopped resolving.
	ErrorExamples []string `json:"transport_error_examples,omitempty"`
	LatencyMillis Latency  `json:"latency_ms"`
	Timeline      []Bucket `json:"timeline"`

	ItemsCreated int `json:"items_created"`
	// ItemsMissing is the data loss question. Every identifier here is one the installation
	// answered 201 for and then could not produce again.
	ItemsMissing   int      `json:"items_missing_afterwards"`
	MissingExample []string `json:"items_missing_examples,omitempty"`

	NoFailedRequests bool `json:"verdict_no_failed_requests"`
	NoDataLoss       bool `json:"verdict_no_data_loss"`
}

// Latency is what the run felt like, not what it promised. RT-6 is the test with a target; this
// is here so that a rollout which answered everything slowly is not filed as uneventful.
type Latency struct {
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
	Max int64 `json:"max"`
}

// Bucket is one interval of the timeline.
type Bucket struct {
	SecondsIn       int `json:"seconds_in"`
	Requests        int `json:"requests"`
	NonSuccess      int `json:"non_success"`
	ServerErrors    int `json:"server_errors"`
	TransportErrors int `json:"transport_errors"`
}

// recorder collects what every worker sees. One mutex rather than per-worker counters merged at
// the end: the timeline needs each observation placed in time, and the contention of a few
// thousand requests a minute is not worth a lock-free design.
type recorder struct {
	mu        sync.Mutex
	start     time.Time
	byStatus  map[string]int
	transport int
	examples  []string
	latencies []int64
	buckets   map[int]*Bucket
	created   []string
}

func newRecorder(start time.Time) *recorder {
	return &recorder{
		start:    start,
		byStatus: map[string]int{},
		buckets:  map[int]*Bucket{},
	}
}

func (r *recorder) bucketAt(now time.Time) *Bucket {
	resolution := int(tick.Seconds())
	seconds := int(now.Sub(r.start).Seconds()) / resolution * resolution
	b, known := r.buckets[seconds]
	if !known {
		b = &Bucket{SecondsIn: seconds}
		r.buckets[seconds] = b
	}
	return b
}

// lastClosed is the interval that has just ended, and it never creates one. The ticker fires on
// the boundary, so the interval that is current at that moment is always the one nobody has
// written to yet - reporting it printed a run of zeroes through a run that was working.
func (r *recorder) lastClosed(now time.Time) Bucket {
	resolution := int(tick.Seconds())
	seconds := int(now.Sub(r.start).Seconds())/resolution*resolution - resolution
	if b, known := r.buckets[seconds]; known {
		return *b
	}
	return Bucket{SecondsIn: seconds}
}

func (r *recorder) observe(status int, took time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	b := r.bucketAt(now)
	b.Requests++

	if err != nil {
		r.transport++
		b.TransportErrors++
		b.NonSuccess++
		if len(r.examples) < 5 {
			r.examples = append(r.examples, err.Error())
		}
		return
	}

	r.byStatus[fmt.Sprint(status)]++
	r.latencies = append(r.latencies, took.Milliseconds())
	if status >= 300 {
		b.NonSuccess++
	}
	if status >= 500 {
		b.ServerErrors++
	}
}

func (r *recorder) remember(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, id)
}

func run(url, collection, credentials string, duration time.Duration, workers, rate int, out string) error {
	tokens := splitTokens(credentials)
	switch {
	case url == "":
		return fmt.Errorf("--url is required")
	case collection == "":
		return fmt.Errorf("--collection is required")
	case len(tokens) == 0:
		return fmt.Errorf("%s is not set", envToken)
	case rate < 1:
		return fmt.Errorf("--rate must be at least 1")
	}

	client := &http.Client{Timeout: requestTimeout}
	start := time.Now()
	rec := newRecorder(start)

	ctx, stop := context.WithDeadline(context.Background(), start.Add(duration))
	defer stop()

	pace := newPacer(ctx, rate)
	fmt.Fprintf(os.Stderr, "rt8: %d workers, %d credentials, %d req/s against %s for %s\n",
		workers, len(tokens), rate, url, duration)

	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			load(ctx, client, url, collection, tokens[worker%len(tokens)], worker, rec, pace)
		}(worker)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		progress(ctx, rec, start)
	}()

	wg.Wait()
	<-done
	ended := time.Now()

	fmt.Fprintf(os.Stderr, "rt8: the load is off, looking for what was written\n")
	// A fresh context: the run's own has expired by now, and that is what ended the load.
	missing, examples := verify(context.Background(), client, url, tokens[0], rec.created)

	result := assemble(url, start, ended, workers, rec, missing, examples)
	return report(result, out)
}

// load is one worker: write something, read it back, and read the level it is in. The mix is
// deliberate - a rollout that drops writes and a rollout that drops reads look identical in a
// summary that only counts requests.
func load(ctx context.Context, client *http.Client, url, collection, token string, worker int, rec *recorder, pace *pacer) {
	// Each worker staggers its start, so that a rollout does not meet all of them mid-request at
	// exactly the same instant and turn one fault into several. Spread by index rather than at
	// random: the same run twice should be the same run twice, and randomness here would buy
	// nothing that counting does not.
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Duration(worker) * stagger):
	}

	for n := 0; ctx.Err() == nil; n++ {
		body := fmt.Sprintf(`{"collection_id":%q,"type":"TASK","title":"rt8 w%d n%d"}`, collection, worker, n)
		id, ok := call(ctx, client, rec, pace, token, http.MethodPost, url+"/api/v1/items", body)
		if ok && id != "" {
			rec.remember(id)
			call(ctx, client, rec, pace, token, http.MethodGet, url+"/api/v1/items/"+id, "")
		}
		call(ctx, client, rec, pace, token, http.MethodGet,
			url+"/api/v1/items?collection_id="+collection+"&page_size=20", "")
	}
}

// pacer hands out permission to make a request, at a fixed rate shared by every worker. A rate
// held by the workers themselves would drift with latency: eight workers waiting 200ms each is
// forty requests a second only while every answer is instant.
type pacer struct {
	permits chan struct{}
}

func newPacer(ctx context.Context, perSecond int) *pacer {
	p := &pacer{permits: make(chan struct{})}
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(perSecond))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				close(p.permits)
				return
			case <-ticker.C:
				select {
				case p.permits <- struct{}{}:
				case <-ctx.Done():
					close(p.permits)
					return
				}
			}
		}
	}()
	return p
}

// wait blocks until this request may be made, and reports false when the run is over.
func (p *pacer) wait(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case _, open := <-p.permits:
		return open
	}
}

// splitTokens reads the comma separated credentials, ignoring the spacing somebody put in.
func splitTokens(credentials string) []string {
	var tokens []string
	for _, candidate := range strings.Split(credentials, ",") {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			tokens = append(tokens, trimmed)
		}
	}
	return tokens
}

// call makes one request and records it. It returns the `id` of the answer when there is one, so
// that a created item can be remembered and looked for later.
func call(ctx context.Context, client *http.Client, rec *recorder, pace *pacer, token, method, url, body string) (string, bool) {
	if !pace.wait(ctx) {
		return "", false
	}

	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return "", false
	}
	request.Header.Set("Authorization", "Bearer "+token)
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
			return "", false
		}
		rec.observe(0, took, err)
		return "", false
	}
	defer func() { _ = response.Body.Close() }()

	payload, _ := io.ReadAll(response.Body)
	rec.observe(response.StatusCode, took, nil)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", false
	}

	var answer struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(payload, &answer)
	return answer.ID, true
}

// verify asks for every identifier the run was told it had created. A 404 here is the data loss
// RT-8 is about: the installation said 201, and then the row was not there.
func verify(ctx context.Context, client *http.Client, url, token string, created []string) (int, []string) {
	// In parallel, because a run of a few minutes creates tens of thousands of items and asking
	// for them one after another would take longer than the test did.
	const verifiers = 16

	ids := make(chan string)
	var mu sync.Mutex
	missing, examples := 0, []string(nil)

	var wg sync.WaitGroup
	for range verifiers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range ids {
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/v1/items/"+id, nil)
				if err != nil {
					continue
				}
				request.Header.Set("Authorization", "Bearer "+token)
				response, err := client.Do(request)
				if err != nil {
					continue
				}
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				if response.StatusCode != http.StatusNotFound {
					continue
				}
				mu.Lock()
				missing++
				if len(examples) < 5 {
					examples = append(examples, id)
				}
				mu.Unlock()
			}
		}()
	}
	for _, id := range created {
		ids <- id
	}
	close(ids)
	wg.Wait()

	return missing, examples
}

// progress prints a line per interval, so that whoever triggers the rollout can see where in the
// run it landed.
func progress(ctx context.Context, rec *recorder, start time.Time) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rec.mu.Lock()
			b := rec.lastClosed(time.Now())
			fmt.Fprintf(os.Stderr, "  %4ds  requests=%-6d non-2xx=%-4d 5xx=%-4d transport=%d\n",
				b.SecondsIn, b.Requests, b.NonSuccess, b.ServerErrors, b.TransportErrors)
			rec.mu.Unlock()
		}
	}
}

func assemble(url string, start, ended time.Time, workers int, rec *recorder, missing int, missingExamples []string) Result {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	requests := rec.transport
	for _, n := range rec.byStatus {
		requests += n
	}

	timeline := make([]Bucket, 0, len(rec.buckets))
	for _, b := range rec.buckets {
		timeline = append(timeline, *b)
	}
	sort.Slice(timeline, func(i, j int) bool { return timeline[i].SecondsIn < timeline[j].SecondsIn })

	serverErrors := 0
	for status, n := range rec.byStatus {
		if len(status) == 3 && status[0] == '5' {
			serverErrors += n
		}
	}

	return Result{
		URL:              url,
		StartedAt:        start.UTC(),
		EndedAt:          ended.UTC(),
		Workers:          workers,
		Requests:         requests,
		ByStatus:         rec.byStatus,
		TransportErrors:  rec.transport,
		ErrorExamples:    rec.examples,
		LatencyMillis:    percentiles(rec.latencies),
		Timeline:         timeline,
		ItemsCreated:     len(rec.created),
		ItemsMissing:     missing,
		MissingExample:   missingExamples,
		NoFailedRequests: serverErrors == 0 && rec.transport == 0,
		NoDataLoss:       missing == 0,
	}
}

func percentiles(latencies []int64) Latency {
	if len(latencies) == 0 {
		return Latency{}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	at := func(q float64) int64 {
		index := int(float64(len(latencies)-1) * q)
		return latencies[index]
	}
	return Latency{P50: at(0.50), P95: at(0.95), Max: latencies[len(latencies)-1]}
}

func report(result Result, out string) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if out != "" {
		if err := os.WriteFile(out, append(encoded, '\n'), 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", out, err)
		}
	}
	fmt.Println(string(encoded))

	if !result.NoFailedRequests || !result.NoDataLoss {
		return fmt.Errorf("RT-8 did not hold: %d server errors or transport failures, %d items lost",
			result.TransportErrors, result.ItemsMissing)
	}
	return nil
}
