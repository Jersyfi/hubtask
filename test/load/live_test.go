// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build load

package load

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tcexec "github.com/testcontainers/testcontainers-go/exec"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/test/load/harness"
)

// envLive turns run L1 on (test/load/PLAN.md); HUBTASK_LOAD_LIVE_SECONDS shortens its window.
const envLive = "HUBTASK_LOAD_LIVE"

// L1's shape and pass bars, as test/load/PLAN.md states them: eight credentials of one workspace
// holding eight streams each (the per-credential cap), ten writes a second, the writes' P95 within
// SLO-3 and no record later than SLO-4.
const (
	liveCredentials   = 8
	liveStreamsEach   = 8
	liveWritesPerSec  = 10
	liveWriteP95Bar   = 300 * time.Millisecond
	liveDeliveryBar   = 30 * time.Second
	liveDefaultWindow = 5 * time.Minute
)

// LiveFinding is what L1 leaves behind.
type LiveFinding struct {
	Test             string          `json:"test"`
	RanAt            time.Time       `json:"ran_at"`
	WindowSeconds    int             `json:"window_seconds"`
	StreamsAsked     int             `json:"streams_asked"`
	StreamsOpened    int             `json:"streams_opened"`
	StreamsRefused   map[int]int     `json:"streams_refused_by_status"`
	StreamsEnded     int64           `json:"streams_ended_early"`
	WritesSent       int64           `json:"writes_sent"`
	WritesAccepted   int             `json:"writes_accepted"`
	WritesFailed     map[int]int     `json:"writes_failed_by_status"`
	WriteLatency     harness.Latency `json:"write_latency_ms"`
	Delivery         harness.Latency `json:"delivery_delay_ms"`
	DeliveriesLate   int             `json:"deliveries_later_than_bar_or_missing"`
	TransactionsPerS float64         `json:"database_transactions_per_second"`
	NotifyQueueMax   float64         `json:"notify_queue_usage_max"`
}

// L1: live updates in one workspace. Every stream should see every write, soon, while the writes
// themselves stay fast.
func TestL1LiveUpdatesInOneWorkspace(t *testing.T) {
	if os.Getenv(envLive) == "" {
		t.Skip("L1 runs on request: " + envLive + "=1")
	}
	window := liveDefaultWindow
	if raw := os.Getenv("HUBTASK_LOAD_LIVE_SECONDS"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			t.Fatalf("HUBTASK_LOAD_LIVE_SECONDS=%q is not a number of seconds", raw)
		}
		window = time.Duration(seconds) * time.Second
	}
	stack := runningStack(t)
	defer scenario("S4")()
	as := stack.tenants[0]
	tokens := mintStreamTokens(t, stack.database, 0, liveCredentials)

	finding := LiveFinding{
		Test: "L1 live updates in one workspace", RanAt: time.Now().UTC(),
		WindowSeconds: int(window / time.Second), StreamsAsked: liveCredentials * liveStreamsEach,
		StreamsRefused: map[int]int{}, WritesFailed: map[int]int{},
	}

	streamCtx, closeStreams := context.WithCancel(context.Background())
	defer closeStreams()
	var streams []*liveStream
	for _, token := range tokens {
		for range liveStreamsEach {
			opened, status := openStream(streamCtx, stack.baseURL, token)
			if opened == nil {
				finding.StreamsRefused[status]++
				continue
			}
			streams = append(streams, opened)
		}
	}
	finding.StreamsOpened = len(streams)
	var ended atomic.Int64
	var readers sync.WaitGroup
	for _, s := range streams {
		readers.Add(1)
		go func() {
			defer readers.Done()
			s.read()
			if streamCtx.Err() == nil {
				ended.Add(1)
			}
		}()
	}

	commitsBefore := databaseCommits(t, stack.database)
	notify := watchNotifyQueue(streamCtx, stack.database)

	sent := map[string]time.Time{}
	var sentMu sync.Mutex
	var latencies []int64
	var sentCount atomic.Int64
	client := &http.Client{Timeout: 30 * time.Second}
	began := time.Now()
	ticker := time.NewTicker(time.Second / liveWritesPerSec)
	var writers sync.WaitGroup
	for n := 0; time.Since(began) < window; n++ {
		<-ticker.C
		writers.Add(1)
		go func(n int) {
			defer writers.Done()
			sentCount.Add(1)
			at := time.Now()
			id, status := createEntry(client, stack.baseURL, as, n)
			took := time.Since(at)
			sentMu.Lock()
			defer sentMu.Unlock()
			if id == "" {
				finding.WritesFailed[status]++
				return
			}
			sent[id] = at
			latencies = append(latencies, took.Milliseconds())
		}(n)
	}
	ticker.Stop()
	writers.Wait()
	writeWindow := time.Since(began)
	finding.TransactionsPerS = float64(databaseCommits(t, stack.database)-commitsBefore) / writeWindow.Seconds()

	// The stragglers get the delivery bar to arrive, and not a moment more.
	deadline := time.Now().Add(liveDeliveryBar)
	for time.Now().Before(deadline) && !allDelivered(streams, sent) {
		time.Sleep(250 * time.Millisecond)
	}
	closeStreams()
	readers.Wait()

	finding.StreamsEnded = ended.Load()
	finding.WritesSent = sentCount.Load()
	finding.WritesAccepted = len(sent)
	finding.WriteLatency = harness.Percentiles(latencies)
	finding.NotifyQueueMax = notify.peak()
	var delays []int64
	for _, s := range streams {
		s.mu.Lock()
		for id, at := range sent {
			arrived, seen := s.arrived[id]
			if !seen || arrived.Sub(at) > liveDeliveryBar {
				finding.DeliveriesLate++
			}
			if seen {
				delays = append(delays, arrived.Sub(at).Milliseconds())
			}
		}
		s.mu.Unlock()
	}
	finding.Delivery = harness.Percentiles(delays)
	writeEvidence(t, "live-l1-latest.json", finding)

	t.Logf("L1: %d of %d streams, %d writes (P95 %d ms), delivery P95 %d ms max %d ms, %d late or missing, %.1f tx/s, NOTIFY queue max %.4f",
		finding.StreamsOpened, finding.StreamsAsked, finding.WritesAccepted, finding.WriteLatency.P95,
		finding.Delivery.P95, finding.Delivery.Max, finding.DeliveriesLate, finding.TransactionsPerS,
		finding.NotifyQueueMax)

	if len(finding.StreamsRefused) > 0 {
		t.Errorf("streams were refused: %v (64 is under every cap)", finding.StreamsRefused)
	}
	if finding.StreamsEnded > 0 {
		t.Errorf("%d streams ended before the run did", finding.StreamsEnded)
	}
	if len(finding.WritesFailed) > 0 {
		t.Errorf("writes failed: %v", finding.WritesFailed)
	}
	if bar := liveWriteP95Bar.Milliseconds(); finding.WriteLatency.P95 > bar {
		t.Errorf("the writes' P95 is %d ms, past SLO-3's %d ms", finding.WriteLatency.P95, bar)
	}
	if finding.DeliveriesLate > 0 {
		t.Errorf("%d deliveries came later than %s or not at all", finding.DeliveriesLate, liveDeliveryBar)
	}
	if finding.NotifyQueueMax > 0.5 {
		t.Errorf("the NOTIFY queue was %.0f%% full, past A-36's half", finding.NotifyQueueMax*100)
	}
}

// liveStream is one open change stream and when each entry's record reached it.
type liveStream struct {
	body    io.ReadCloser
	mu      sync.Mutex
	arrived map[string]time.Time
}

func openStream(ctx context.Context, baseURL, token string) (*liveStream, int) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/stream", nil)
	if err != nil {
		return nil, 0
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, 0
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, response.StatusCode
	}
	return &liveStream{body: response.Body, arrived: map[string]time.Time{}}, http.StatusOK
}

// read notes the first arrival of every entry's record until the stream ends.
func (s *liveStream) read() {
	defer func() { _ = s.body.Close() }()
	lines := bufio.NewScanner(s.body)
	lines.Buffer(make([]byte, 64<<10), 4<<20)
	for lines.Scan() {
		data, isData := strings.CutPrefix(lines.Text(), "data: ")
		if !isData {
			continue
		}
		var record struct {
			Entity   string `json:"entity"`
			EntityID string `json:"entity_id"`
		}
		if json.Unmarshal([]byte(data), &record) != nil || record.EntityID == "" {
			continue
		}
		now := time.Now()
		s.mu.Lock()
		if _, seen := s.arrived[record.EntityID]; !seen {
			s.arrived[record.EntityID] = now
		}
		s.mu.Unlock()
	}
}

func allDelivered(streams []*liveStream, sent map[string]time.Time) bool {
	for _, s := range streams {
		s.mu.Lock()
		for id := range sent {
			if _, seen := s.arrived[id]; !seen {
				s.mu.Unlock()
				return false
			}
		}
		s.mu.Unlock()
	}
	return true
}

func createEntry(client *http.Client, baseURL string, as tenant, n int) (string, int) {
	body := fmt.Sprintf(`{"collection_id":%q,"type":"TASK","title":"live n%d"}`, as.collection, n)
	// The client's own 30 s timeout bounds the call; the deadline says so to the linter too (rule 7).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/items", bytes.NewReader([]byte(body)))
	if err != nil {
		return "", 0
	}
	request.Header.Set("Authorization", "Bearer "+as.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", 0
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return "", response.StatusCode
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil || created.ID == "" {
		return "", response.StatusCode
	}
	return created.ID, response.StatusCode
}

// mintStreamTokens writes n more credentials of one seeded workspace's account, each carrying a
// session's scopes, as mintTokens writes the load token.
func mintStreamTokens(t *testing.T, container *tcpostgres.PostgresContainer, rank, n int) []string {
	t.Helper()
	ctx := context.Background()
	tenantID, accountID := derived(ctx, "tenant", rank), derived(ctx, "account", rank)
	parsed, err := shared.ParseID(tenantID)
	if err != nil {
		t.Fatal(err)
	}
	hasher := security.NewTokenHasher(secret.New(installationSecret))
	tokens := make([]string, 0, n)
	for range n {
		material := make([]byte, identity.TokenSecretBytes)
		if _, err := rand.Read(material); err != nil {
			t.Fatal(err)
		}
		token, err := identity.NewToken(parsed, material)
		if err != nil {
			t.Fatal(err)
		}
		if err := psql(ctx, container, fmt.Sprintf(
			`INSERT INTO access_token (id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at)
			 VALUES (gen_random_uuid(), '%s', '%s', 'the live run', decode('%x', 'hex'), 'hbt_pat_',
			         %s, now() + interval '2 hours')`,
			tenantID, accountID, hasher.Hash(token.Secret()), scopeArray(catalogue.SessionScopes()),
		)); err != nil {
			t.Fatalf("minting a stream credential: %v", err)
		}
		tokens = append(tokens, token.Secret())
	}
	return tokens
}

// psqlValue runs one query inside the container and answers its single value.
func psqlValue(ctx context.Context, container *tcpostgres.PostgresContainer, query string) (string, error) {
	code, reader, err := container.Exec(ctx, []string{
		"psql", "-U", "postgres", "-d", "hubtask_load", "-tA", "-v", "ON_ERROR_STOP=1", "-c", query,
	}, tcexec.Multiplexed())
	if err != nil {
		return "", err
	}
	out, _ := io.ReadAll(reader)
	if code != 0 {
		return "", fmt.Errorf("psql exited %d: %s", code, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func databaseCommits(t *testing.T, container *tcpostgres.PostgresContainer) int64 {
	t.Helper()
	value, err := psqlValue(context.Background(), container,
		`SELECT xact_commit FROM pg_stat_database WHERE datname = 'hubtask_load'`)
	if err != nil {
		t.Fatalf("reading the commit count: %v", err)
	}
	commits, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("the commit count reads %q", value)
	}
	return commits
}

// notifyWatch samples pg_notification_queue_usage() once a second, keeping the peak.
type notifyWatch struct {
	mu  sync.Mutex
	max float64
}

func (w *notifyWatch) peak() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.max
}

func watchNotifyQueue(ctx context.Context, container *tcpostgres.PostgresContainer) *notifyWatch {
	watch := &notifyWatch{}
	go func() {
		for ctx.Err() == nil {
			if value, err := psqlValue(ctx, container, `SELECT pg_notification_queue_usage()`); err == nil {
				if usage, err := strconv.ParseFloat(value, 64); err == nil {
					watch.mu.Lock()
					watch.max = max(watch.max, usage)
					watch.mu.Unlock()
				}
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}()
	return watch
}

// envDocuments turns S5 on (test/load/PLAN.md); HUBTASK_LOAD_DOCUMENTS_SECONDS shortens its
// window.
const envDocuments = "HUBTASK_LOAD_DOCUMENTS"

const (
	documentsDefaultWindow = 2 * time.Minute
	// documentsCredentials hold the 30 writers within the per-credential cap of eight, and the
	// fresh opener at the end on a fifth.
	documentsCredentials = 5
	awarenessRelayed     = "hubtask_realtime_awareness_relayed_total"
)

// DocumentsFinding is what S5 leaves behind: the writers' own report (liveload.mjs) beside what
// the server counted over the window.
type DocumentsFinding struct {
	Test               string             `json:"test"`
	RanAt              time.Time          `json:"ran_at"`
	WindowSeconds      int                `json:"window_seconds"`
	Writers            json.RawMessage    `json:"writers"`
	AwarenessByOutcome map[string]float64 `json:"awareness_relayed_by_outcome"`
	TransactionsPerS   float64            `json:"database_transactions_per_second"`
}

// S5: thirty writers on one page's live document, through the editors' own connection. Every
// writer's document, and one opened afresh, is the same at the end, and the awareness relay
// neither drops a message as too large nor fails.
func TestS5LiveDocumentsWithManyWriters(t *testing.T) {
	if os.Getenv(envDocuments) == "" {
		t.Skip("S5 runs on request: " + envDocuments + "=1")
	}
	window := documentsDefaultWindow
	if raw := os.Getenv("HUBTASK_LOAD_DOCUMENTS_SECONDS"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			t.Fatalf("HUBTASK_LOAD_DOCUMENTS_SECONDS=%q is not a number of seconds", raw)
		}
		window = time.Duration(seconds) * time.Second
	}
	stack := runningStack(t)
	defer scenario("S5")()
	as := stack.tenants[0]
	tokens := mintStreamTokens(t, stack.database, 0, documentsCredentials)
	page := createPage(t, stack, as)

	before := counterBy(t, stack.opsURL, awarenessRelayed, "outcome")
	commitsBefore := databaseCommits(t, stack.database)
	began := time.Now()

	out := filepath.Join(t.TempDir(), "liveload.json")
	ctx, cancel := context.WithTimeout(context.Background(), window+3*time.Minute)
	defer cancel()
	document := "ws" + strings.TrimPrefix(stack.baseURL, "http") + "/api/v1/realtime/documents/page/" + page
	cmd := exec.CommandContext(ctx, "node", append([]string{"scripts/liveload.mjs", document, out,
		strconv.Itoa(int(window / time.Second))}, tokens...)...)
	cmd.Dir = filepath.Join(repositoryRoot(), "packages", "editor")
	output, err := cmd.CombinedOutput()
	t.Logf("the writers:\n%s", output)
	if err != nil {
		t.Fatalf("running the writers: %v", err)
	}
	elapsed := time.Since(began)

	after := counterBy(t, stack.opsURL, awarenessRelayed, "outcome")
	finding := DocumentsFinding{
		Test: "S5 live documents with many writers", RanAt: began.UTC(), WindowSeconds: int(window / time.Second),
		AwarenessByOutcome: map[string]float64{},
		TransactionsPerS:   float64(databaseCommits(t, stack.database)-commitsBefore) / elapsed.Seconds(),
	}
	for outcome, n := range after {
		finding.AwarenessByOutcome[outcome] = n - before[outcome]
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading the writers' report: %v", err)
	}
	finding.Writers = raw
	writeEvidence(t, "documents-s5-latest.json", finding)

	var writers struct {
		Writers    int            `json:"writers"`
		Edits      int            `json:"edits"`
		Unsent     int            `json:"unsent"`
		Equal      bool           `json:"equal"`
		FreshEqual bool           `json:"fresh_equal"`
		Closes     map[string]int `json:"closes_by_code"`
	}
	if err := json.Unmarshal(raw, &writers); err != nil {
		t.Fatalf("reading the writers' report: %v", err)
	}
	if !writers.Equal || !writers.FreshEqual {
		t.Errorf("the documents differ at the end: writers equal %v, a fresh opener equal %v", writers.Equal, writers.FreshEqual)
	}
	if writers.Unsent > 0 {
		t.Errorf("%d edits were never sent", writers.Unsent)
	}
	if len(writers.Closes) > 0 {
		t.Errorf("connections were closed: %v", writers.Closes)
	}
	if writers.Edits < writers.Writers*int(window/time.Second)/2 {
		t.Errorf("only %d edits by %d writers in %s", writers.Edits, writers.Writers, window)
	}
	for _, outcome := range []string{"too_large", "failed"} {
		if finding.AwarenessByOutcome[outcome] > 0 {
			t.Errorf("%v awareness messages relayed as %s", finding.AwarenessByOutcome[outcome], outcome)
		}
	}
}

// createPage switches the pages' module on for the workspace, as seedInventory the assets', writes
// one page in its hub and answers the page's identifier.
func createPage(t *testing.T, stack *stack, as tenant) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := psql(ctx, stack.database, fmt.Sprintf(
		`INSERT INTO setting_value (id, tenant_id, level, scope_id, key, value, updated_at)
		 VALUES (gen_random_uuid(), '%[1]s', 'WORKSPACE', '%[1]s', 'module.knowledge', 'true', now())
		 ON CONFLICT (tenant_id, level, scope_id, key) DO UPDATE SET value = 'true'`, as.id)); err != nil {
		t.Fatalf("switching the pages on: %v", err)
	}
	baseURL := stack.baseURL
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/hubs/"+as.hub+"/pages",
		strings.NewReader(`{"title":"the live document run"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+as.walkToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("creating the page: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	var created struct {
		ID string `json:"id"`
	}
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("creating the page answered %d: %s", response.StatusCode, body)
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil || created.ID == "" {
		t.Fatalf("creating the page: %v", err)
	}
	return created.ID
}

// counterBy reads one counter from the server's metrics, summed by a label.
func counterBy(t *testing.T, opsURL, name, label string) map[string]float64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, opsURL+"/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("reading the metrics: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the metrics answered %d", response.StatusCode)
	}
	values, err := harness.CounterBy(response.Body, name, label)
	if err != nil {
		t.Fatalf("reading the metrics: %v", err)
	}
	return values
}
