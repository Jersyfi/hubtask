# The second performance round — the owner's workstation

**2026-11-05, the second round of [test/load/PLAN.md](../../test/load/PLAN.md), test code of
commit `9b801ebe3` (the 10⁶ invocation), `ba349641d` (S5's reruns and the 10⁷ invocation).**
One process serving every role, against PostgreSQL 16 (`pgvector/pgvector:pg16`, pulled afresh
before the round) in a container. Machine: Apple M4 Max, 14 cores, 36 GB; Docker VM 7.9 GB.
This is the workstation tier of accepted.md D, not the release tier on named hardware.
Latencies are milliseconds; a P95 is written with its 95 % band as `P95 [low–high]`.

## 10⁶ entries over 200 workspaces

```sh
HUBTASK_LOAD_CAPACITY=1 HUBTASK_LOAD_WALK=1 HUBTASK_LOAD_SYNC=1 HUBTASK_LOAD_LIVE=1 \
HUBTASK_LOAD_LIVE_SECONDS=120 HUBTASK_LOAD_DOCUMENTS=1 HUBTASK_LOAD_ITEMS=1000000 HUBTASK_LOAD_TENANTS=200 \
  go test -count=1 -tags=load -run '^(TestCapacityAtScale|TestCmdbViewsAtTheHubCeiling|TestS3InitialWalk|TestL1LiveUpdatesInOneWorkspace|TestS5LiveDocumentsWithManyWriters)$' -timeout=3h -v ./test/load/
```

26 min in all (1 562 s, 22:03:49 to 22:29:51 CET). The JSON is in
[`second-round-1000000-2026-11-05/`](second-round-1000000-2026-11-05/). Go runs the tests in the
order of their files, so the scenarios ran S1/S2 per workspace, S7, C1, S4, S5, S3 — S3 after the
writers of S4 and S5, not before them as the plan's order says; their writes (1 200 entries and
one page) are in the walked workspace.

| Phase | Window (from `errors-1000000.json`) | Took |
|---|---|---|
| Setup (build, container, migrations, COPY seed, vacuum, server) | 22:03:49 to 22:06:57 | 3 min 8 s |
| S1 and S2, three workspaces | 22:06:57 to 22:22:42 | 15 min 45 s |
| S7 the walk of 33 screens | 22:22:42 to 22:23:22 | 40 s |
| C1 the CMDB ceiling | 22:23:22 to 22:25:38 | 2 min 16 s |
| S4 (L1, 120 s window) | 22:25:38 to 22:27:39 | 2 min 1 s |
| S5 (120 s window) | 22:27:39 to 22:29:40 | 2 min 1 s |
| S3 | 22:29:40 to 22:29:52 | 12 s |

**S1, read**, eight callers paced at 400 a second, 30 s held after 5 s warm-up. No 5xx, no
refusal on any path. Peak resident memory of the server over S1 and S2 (read before the walk):
219 463 680 bytes (209 MiB).

| Path | Largest, 170 124 entries | Middle (rank 4), 34 024 | Smallest (rank 7), 21 265 |
|---|---|---|---|
| a collection's page | 400/s, P50 8, P95 8 [8–8], P99 10 | 400/s, 8, 8 [8–8], 10 | 400/s, 8, 9 [9–9], 10 |
| open entries by due date | 400/s, 7, 8 [8–8], 9 | 400/s, 7, 9 [9–9], 10 | 400/s, 8, 9 [9–9], 11 |
| the board by completion | 400/s, 9, 11 [10–11], 12 | 400/s, 10, 11 [11–11], 13 | 400/s, 10, 11 [11–11], 13 |
| the web app's board | 399.6/s, 15, 17 [17–17], 22 | 399.9/s, 13, 14 [14–14], 17 | 400/s, 12, 14 [14–14], 19 |
| a count over the hub | 373.5/s, 20, 25 [25–25], 27 | 400/s, 8, 8 [8–8], 9 | 400/s, 7, 7 [7–7], 8 |
| the hub's level | 400/s, 8, 8 [8–8], 10 | 400/s, 7, 8 [8–8], 10 | 399.1/s, 8, 10 [10–10], 17 |

**S2, mixed**: the hub's level read at 400 a second beside two writers at 40 a second. No 5xx.

| Workspace | Reads | Writes |
|---|---|---|
| Largest, 170 124 | 399.9/s, P50 9, P95 11 [11–11], P99 15 | 40/s, P50 19, P95 22 [22–24], P99 30 |
| Middle, 34 024 | 400/s, 9, 11 [11–11], 14 | 39.8/s, 19, 21 [21–22], 30 (max 205) |
| Smallest, 21 265 | 399.9/s, 10, 13 [13–13], 17 | 40/s, 20, 24 [24–24], 32 |

**S7, the bundle per route**: 33 screens, every one settled, in 949 to 1 260 ms; no browser
row. Every screen fetched the same 472 files: 2 246 158 script and 198 224 style bytes
gzip-compressed. That is eleven times the first screen's budget of 200 000 gzip bytes
(`apps/webapp/build/budget.test.js`): a screen's full navigation loads the whole set.

**C1, the CMDB ceiling** (20 000 CIs, 19 800 relations), eight callers: no 5xx.

| View | Answers a second | P50 | P95 [band] | P99 | Max |
|---|---|---|---|---|---|
| `cmdb-health` | 34.0 | 235 | 248 [247–250] | 256 | 358 |
| `software` | 56.2 | 142 | 152 [151–152] | 157 | 226 |
| `service-map` | 21.8 | 365 | 390 [386–398] | 429 | 614 |

Peak resident memory: 835 059 712 bytes (796 MiB), **over** the run's `GOMEMLIMIT` of
768 MiB (805 306 368 bytes), which the plan's row names as the bar; under the chart's 1 GiB
limit. The run of the morning measured 753 MiB. Health's and the service map's P95 lie over
SLO-2's 200 ms, as in the morning's run.

**S4 (L1)**, 64 change streams of eight credentials, 10 writes a second for 120 s: 64 of 64
opened, none refused or ended early; 1 200 writes, all accepted, P50 20, P95 60 [58–62], P99 69,
max 78; 76 800 deliveries, P50 185, P95 302 [302–302], P99 314, max 329, none later than 30 s or
missing; 1 096 database transactions a second; the NOTIFY queue's highest use 0.

**S5, live documents**, 30 writers for 120 s. **Failed twice, fixed twice, then passed:**

| Run | Commit | Edits, cursors | Documents equal | Closed connections |
|---|---|---|---|---|
| In the invocation | `9b801ebe3` | 3 570, 35 340 | yes, the fresh opener too | 89 × 1006 (the first at 30.0 s), 1 × 1001 |
| Rerun, 22:32 | `b01afe7fc` | 3 570, 35 400 | yes | 35 × 1008 `realtime.idle` (the first at 40.1 s) |
| Rerun, 22:39 | `ba349641d` | 3 570, 35 430 | yes | none |

The passing run: no edit unsent, 35 461 awareness messages relayed `local`, none `too_large` or
`failed`, 304 database transactions a second; 299 s in all with its own setup.

1. *Every live document dropped at 30 s.* The route `GET /realtime/documents/{kind}/{id}`
   inherited the request timeout (`HUBTASK_REQUEST_TIMEOUT`, 30 s) like an ordinary request;
   its reader's context ended and the socket with it. Fixed by `b01afe7fc`: the route is held like
   the change stream (`TestTheStreamOutlivesTheRequestTimeout`).
2. *A busy connection closed as idle at 40 s.* The 30 s ping was awaited on the connection's own
   loop, while its pong could only be read by the reader that waited for that loop to take the
   message before it; a client that keeps sending stalled for the write bound (10 s) and was
   closed. Fixed by `ba349641d`: the ping is awaited beside the loop
   (`TestAPingDoesNotStallABusyConnection`).

The trial of 2026-11-05 held 20 s and could see neither.

**S3, the sync walk** of the largest workspace (170 124 entries, 212 641 records): the snapshot
198 071 336 bytes, first byte 37 ms, held 2 734 ms; the pages 426 at 500, 196 677 567 bytes in
6 402 ms, page P50 15, P95 16 [16–17], P99 20, max 42, none over 30 s, every one 200; the
narrowed snapshot the same 212 641 records (the workspace has one hub), held 2 695 ms. No 5xx.

**Errors collected** (`errors-1000000.json`, eleven windows; `errors-1000000-s5.json` for the
passing rerun): no finding and no expected row from the server's log or PostgreSQL's. Both S5
findings were the scenario's own failed bar (closed connections), not log rows: the server's
log at `WARN` and above held nothing for them.

**Peak resident memory per scenario.** Recorded by the capacity run (S1 and S2 together:
209 MiB) and C1 (796 MiB). S3, S4, S5 and S7 do not record the server's memory: *not measured*.

**S6** did not run: it is not built (PLAN.md, "What the round needs built first", item 5).

**What S7's bytes are.** A signed-in session fetches every screen's and part's chunk once it is
idle, so that a screen never opened still opens offline (`apps/webapp/src/App.svelte`,
`precacheWhenIdle`). The walk opens each screen by a full navigation and counts every script and
style it fetched until the screen settled, so its 472 files are the whole application's
precache, the same on every screen; they do not show what one route costs. The first screen's own
files (`index.html`'s scripts and preloads) are held to 200 000 gzip bytes by `budget.test.js`.
No error came of it; whether a precache that size is wanted is not decided here.

## 10⁷ entries over 2 000 workspaces

```sh
HUBTASK_LOAD_CAPACITY=1 HUBTASK_LOAD_WALK=1 HUBTASK_LOAD_SYNC=1 HUBTASK_LOAD_ITEMS=10000000 HUBTASK_LOAD_TENANTS=2000 \
  go test -count=1 -tags=load -run '^(TestCapacityAtScale|TestS3InitialWalk)$' -timeout=4h -v ./test/load/
```

46 min 45 s in all (2 804 s, 22:44:29 to 23:31:14 CET), on `ba349641d`. The JSON is in
[`second-round-10000000-2026-11-05/`](second-round-10000000-2026-11-05/). The host's free disk
went from 75 to 58 GiB over the run (Docker's VM disk keeps what it grew); the container left no
volume.

| Phase | Window | Took |
|---|---|---|
| Setup | 22:44:29 to 23:13:52 | 29 min 23 s |
| S1 and S2, three workspaces | 23:13:52 to 23:29:37 | 15 min 45 s |
| S7 the walk of 33 screens | 23:29:37 to 23:30:16 | 39 s |
| S3 | 23:30:16 to 23:31:14 | 58 s |

**S1, read.** No 5xx, no refusal. Peak resident memory over S1 and S2: 219 643 904 bytes
(209 MiB).

| Path | Largest, 1 222 737 entries | Middle (rank 4), 244 547 | Smallest (rank 7), 152 842 |
|---|---|---|---|
| a collection's page | 399.7/s, P50 8, P95 9 [8–9], P99 11 | 400/s, 8, 9 [9–9], 10 | 400/s, 8, 9 [9–9], 10 |
| open entries by due date | 400/s, 8, 9 [9–9], 12 | 400/s, 8, 9 [9–9], 12 | 400/s, 8, 9 [9–9], 12 |
| the board by completion | 400/s, 9, 10 [10–10], 12 | 400/s, 9, 10 [10–10], 12 | 400/s, 9, 10 [10–10], 12 |
| the web app's board | **199.1/s**, 39, 47 [47–48], 52 | 328.6/s, 23, 35 [34–35], 37 | 386.2/s, 19, 33 [33–33], 35 |
| a count over the hub | **71.8/s**, 110, 119 [119–119], 122 | 282.2/s, 27, 32 [32–32], 33 | 390.9/s, 19, 25 [25–25], 26 |
| the hub's level | 400/s, 8, 8 [8–8], 10 | 399.9/s, 8, 9 [9–9], 14 | 400/s, 8, 8 [8–8], 10 |

Every P95 lies under SLO-2's 200 ms. Two paths no longer hold the 400 a second the eight callers
are paced at in the largest workspace: the count over the hub answers 71.8 a second (R2 of
2026-11-01: 128 ms and 65 a second, the same order) and the web app's board 199.1.

**More than twice the P95 at 10⁶ for the same rank** (PLAN.md, "What happens with what was
collected", 2): the web app's board (17 → 47, 14 → 35, 14 → 33 ms) and the count over the hub
(25 → 119, 8 → 32, 7 → 25 ms). The same ranks hold 7.2, 7.2 and 7.2 times the entries at 10⁷
(170 124 → 1 222 737, 34 024 → 244 547, 21 265 → 152 842), and both paths read every entry they
cover: the count from `wi_completion_idx` alone (migration 0325, its plan read in the first
round's 10⁷ run), the board's exact count (`"count":"exact"`) over its collection, which the
harness's comment on the path names. Their P95 grew by 2.4 to 4.8 times, less than the entries.
**Explained, not fixed**; no plan was read in this round (`HUBTASK_LOAD_EXPLAIN` logs a statement
over 100 ms, and the board's are under it).

**S2, mixed.** No 5xx.

| Workspace | Reads | Writes |
|---|---|---|
| Largest, 1 222 737 | 399.9/s, P50 9, P95 11 [11–11], P99 15 | 40/s, P50 19, P95 22 [22–24], P99 30 |
| Middle, 244 547 | 399.9/s, 10, 12 [12–12], 16 | 40/s, 19, 22 [22–23], 27 |
| Smallest, 152 842 | 399.8/s, 10, 12 [12–12], 17 | 40/s, 19, 22 [22–23], 29 |

**S7**: 33 screens, every one settled, in 945 to 1 262 ms; no browser row; the same 472 files,
2 246 158 script and 198 224 style gzip bytes, on every screen.

**S3, the sync walk** of the largest workspace (1 222 737 entries):

| Walk | Records | Bytes | Time | Ended with its cursor |
|---|---|---|---|---|
| `sync:snapshot` | 543 127 | 536 870 315 | first byte 39 ms, held 7 976 ms | **no** |
| `:pull`, 2 451 pages of 500 | 1 225 170 | 1 203 118 468 | 41 635 ms; page P50 16, P95 17 [17–17], P99 18, max 69 | yes |
| the narrowed snapshot (the one hub) | 543 127 | 536 870 315 | first byte 25 ms, held 7 911 ms | **no** |

Both snapshots ended at the snapshot's byte budget of 512 MiB (`snapshotByteBudget`,
`presentation/rest/SnapshotController.go`) without the cursor line: by design
(offline-sync.md §3.1), a device then starts again and after two such snapshots walks `:pull`,
which reached the end with no page over 30 s and every page 200. The workspace has one hub, so the
narrowed snapshot is the whole workspace too. No 5xx.

**Errors collected** (`errors-10000000.json`, eight windows): no finding. Two expected rows, both
in S1: 14 cancelled `CountFigureProgress` and 9 cancelled `CountFigureDue` statements - the
figure deadline cutting a late count off (the first round's 10⁷ write-up, finding 4); R2 left 24.

## What did not run

* **S6**, the connector's reconciliation: not built. A connector's address is https only and the
  run's server trusts only the platform's roots; on macOS Go ignores `SSL_CERT_FILE`, so the
  test's own source cannot be reached without a setting that adds a trusted root, which is the
  owner's (PLAN.md, "What the round needs built first", item 5).
* The server's resident memory during S3, S4, S5 and S7: those scenarios do not record it.
