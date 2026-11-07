// SPDX-License-Identifier: Apache-2.0
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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Jersyfi/hubtask/test/load/harness"
)

// envSync turns S3 on (test/load/PLAN.md, the second round).
const envSync = "HUBTASK_LOAD_SYNC"

const (
	// syncPageBar is HUBTASK_REQUEST_TIMEOUT's default: a page that takes longer is one the
	// server's own deadline would have ended.
	syncPageBar = 30 * time.Second
	// syncPageSize is the browser's page (packages/sync-engine PULL_PAGE_SIZE).
	syncPageSize = 500
)

// SyncFinding is what S3 leaves behind.
type SyncFinding struct {
	Test       string    `json:"test"`
	RanAt      time.Time `json:"ran_at"`
	Items      int       `json:"workspace_items"`
	CapSeconds int       `json:"cap_seconds"`
	// Snapshot is the whole walk as one response; HeldMS is how long it held its connection.
	Snapshot SyncSnapshotRun `json:"snapshot"`
	Pages    SyncPagesRun    `json:"pages"`
	// Narrowed is the browser's snapshot past its bound: the hubs it opened lately as scopes
	// (offline-sync.md §1). The seeded workspace has one hub.
	Narrowed SyncSnapshotRun `json:"narrowed_snapshot"`
}

// SyncSnapshotRun is one `sync:snapshot` request.
type SyncSnapshotRun struct {
	Status int `json:"status"`
	harness.SnapshotRead
	HeldMS int64 `json:"held_ms"`
}

// SyncPagesRun is one null-cursor walk of `:pull` to its end.
type SyncPagesRun struct {
	Pages        int             `json:"pages"`
	Records      int             `json:"records"`
	Bytes        int64           `json:"bytes"`
	TotalMS      int64           `json:"total_ms"`
	PageLatency  harness.Latency `json:"page_latency_ms"`
	PagesOverBar int             `json:"pages_over_bar"`
	ByStatus     map[int]int     `json:"by_status"`
	Ended        bool            `json:"ended"`
}

// TestS3InitialWalk walks the largest workspace from a null cursor to its end, once as one
// snapshot and once page by page, then takes the browser's narrowed snapshot. It asserts the bars
// (every page within the request timeout, no 5xx, the walk ended within its cap) and records the
// rest as the first baseline.
func TestS3InitialWalk(t *testing.T) {
	if os.Getenv(envSync) == "" {
		t.Skip("S3 runs on request: " + envSync + "=1")
	}
	stack := runningStack(t)
	defer scenario("S3")()
	tenants, items := datasetSize(t)
	as := stack.tenants[0]
	// The plan's caps: ten minutes up to 10⁶ entries, twenty above.
	limit := 10 * time.Minute
	if items > 1_000_000 {
		limit = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()

	finding := SyncFinding{Test: "S3 sync initial walk", RanAt: time.Now().UTC(),
		Items: itemsOfRank(0, tenants, items), CapSeconds: int(limit / time.Second)}
	finding.Snapshot = snapshotOnce(ctx, t, stack.baseURL, as.walkToken, "")
	finding.Pages = pullToTheEnd(ctx, t, stack.baseURL, as.walkToken)
	finding.Narrowed = snapshotOnce(ctx, t, stack.baseURL, as.walkToken, as.hub)
	writeEvidence(t, fmt.Sprintf("sync-s3-%d.json", items), finding)

	t.Logf("S3: snapshot %d records, %d bytes, first byte %d ms, held %d ms, cursor %v; %d pages, %d records, %d bytes in %d ms (page P95 %d ms, max %d ms); narrowed %d records, %d bytes, held %d ms",
		finding.Snapshot.Records, finding.Snapshot.Bytes, finding.Snapshot.FirstByteMS, finding.Snapshot.HeldMS,
		finding.Snapshot.CursorReceived, finding.Pages.Pages, finding.Pages.Records, finding.Pages.Bytes,
		finding.Pages.TotalMS, finding.Pages.PageLatency.P95, finding.Pages.PageLatency.Max,
		finding.Narrowed.Records, finding.Narrowed.Bytes, finding.Narrowed.HeldMS)

	if ctx.Err() != nil {
		t.Errorf("S3 did not end within its cap of %s", limit)
	}
	for name, run := range map[string]SyncSnapshotRun{"snapshot": finding.Snapshot, "narrowed snapshot": finding.Narrowed} {
		if run.Status != http.StatusOK || run.Malformed > 0 {
			t.Errorf("the %s answered %d with %d malformed lines", name, run.Status, run.Malformed)
		}
	}
	for status, n := range finding.Pages.ByStatus {
		if status != http.StatusOK {
			t.Errorf("%d pages answered %d", n, status)
		}
	}
	if !finding.Pages.Ended {
		t.Errorf("the page walk did not reach its end")
	}
	if finding.Pages.PagesOverBar > 0 {
		t.Errorf("%d pages took longer than %s", finding.Pages.PagesOverBar, syncPageBar)
	}
}

// snapshotOnce reads one snapshot to its end; a hub narrows it to that hub's subtree, as the
// browser asks past its bound.
func snapshotOnce(ctx context.Context, t *testing.T, baseURL, token, hub string) SyncSnapshotRun {
	t.Helper()
	body := map[string]any{"device_id": newDevice(t), "platform": "load"}
	if hub != "" {
		body["scopes"] = []map[string]string{{"container_id": hub, "depth": "SUBTREE"}}
	}
	started := time.Now()
	response, err := syncPost(ctx, baseURL, token, "/api/v1/sync:snapshot", body)
	if err != nil {
		t.Errorf("snapshot: %v", err)
		return SyncSnapshotRun{}
	}
	defer func() { _ = response.Body.Close() }()
	run := SyncSnapshotRun{Status: response.StatusCode}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return run
	}
	run.SnapshotRead, err = harness.ReadSnapshot(response.Body, started)
	run.HeldMS = time.Since(started).Milliseconds()
	if err != nil {
		t.Errorf("reading the snapshot: %v", err)
	}
	return run
}

// pullToTheEnd walks `:pull` from a null cursor until a page says there is no more.
func pullToTheEnd(ctx context.Context, t *testing.T, baseURL, token string) SyncPagesRun {
	t.Helper()
	run := SyncPagesRun{ByStatus: map[int]int{}}
	device := newDevice(t)
	var cursor *string
	var latencies []int64
	started := time.Now()
	for ctx.Err() == nil {
		at := time.Now()
		response, err := syncPost(ctx, baseURL, token, "/api/v1/sync:pull",
			map[string]any{"device_id": device, "platform": "load", "cursor": cursor, "limit": syncPageSize})
		if err != nil {
			t.Errorf("page %d: %v", run.Pages+1, err)
			break
		}
		raw, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		took := time.Since(at)
		run.Pages++
		run.ByStatus[response.StatusCode]++
		run.Bytes += int64(len(raw))
		latencies = append(latencies, took.Milliseconds())
		if took > syncPageBar {
			run.PagesOverBar++
		}
		if err != nil || response.StatusCode != http.StatusOK {
			break
		}
		var page struct {
			Changes []json.RawMessage `json:"changes"`
			Cursor  string            `json:"cursor"`
			HasMore bool              `json:"has_more"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			t.Errorf("page %d: %v", run.Pages, err)
			break
		}
		run.Records += len(page.Changes)
		if !page.HasMore {
			run.Ended = true
			break
		}
		cursor = &page.Cursor
	}
	run.TotalMS = time.Since(started).Milliseconds()
	run.PageLatency = harness.Percentiles(latencies)
	return run
}

func syncPost(ctx context.Context, baseURL, token, path string, body any) (*http.Response, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(request)
}

// newDevice is a device identifier as a client mints one (UUIDv7).
func newDevice(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return strings.ToLower(id.String())
}
