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
