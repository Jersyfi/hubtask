# The plan for the performance runs

The owner asked for this plan before the long runs start (accepted.md, "The owner's word on the
performance tests", 2026-10-31): what runs, how long it really takes — measured by short probes,
not guessed — and how every error a run sees is collected, deduplicated and afterwards fixed.
Several hours of load on the workstation is too much. The commands and the dataset are in
[README.md](./README.md); the rule they answer is
[`observability-reliability.md` §13.2](../../docs/architecture/observability-reliability.md), the
bars are its SLOs (§2: SLO-2 read P95 < 200 ms, SLO-3 write P95 < 300 ms, SLO-4 delivery < 30 s).

Every figure below names the run or probe that produced it; a figure no run produced is written
*unmeasured*, and a duration for an unbuilt scenario is its **cap** (the deadline the scenario
gets), not a measurement.

## Where things stand (2026-11-05)

The first round was accepted on 2026-10-31 (accepted.md, "The owner's fourth answers", item 13)
and has run:

| # | Run | Outcome | Measured duration |
|---|---|---|---|
| P1 | 10⁵ entries over 20 workspaces | no server error on any path | 14 min (839 s), 2026-10-31 |
| P2 | 10⁶ entries over 200 workspaces, with the mixed window | [evidence](../../docs/evidence/capacity-1000000-2026-10-31.md), "The hub's level under writers" | 16.5 min, 2026-11-01 |
| R1 + R3 | 10⁷ entries over 2 000 workspaces, with the browser walk | [evidence](../../docs/evidence/capacity-10000000-2026-11-01.md): five findings, fixed | 42.6 min, 2026-11-01 |
| R2 (twice) | R1 + R3 again on the fixes | no server error, no browser row; 24 cancelled figure statements left | 42.5 min (the second), 2026-11-01 |
| L1 | 64 change streams under 10 writes a second, default dataset (40 000 entries) | [evidence](../../docs/evidence/live-l1-2026-11-05.md): passes | 5 min 19 s (319 s), 2026-11-05 |
| C1 | One hub at the CMDB ceiling (20 000 CIs, 19 800 relations), default dataset: health, software and the service map | [evidence](../../docs/evidence/cmdb-ceiling-2026-11-05.md): no server error, peak 753 MiB resident; P95 257, 171 and 583 ms | 2 min 33 s (153 s), 2026-11-05 |
| S10⁶ | The second round at 10⁶: S1, S2, S7, C1, S4, S5, S3 in one invocation, S5 again twice after two fixes | [evidence](../../docs/evidence/second-round-2026-11-05.md): S5 failed twice and passed on the fixes; C1's memory over its bar; no log finding | 26 min (1 562 s) and S5's reruns 305 s and 299 s, 2026-11-05 |
| S10⁷ | The second round at 10⁷: S1, S2, S7, S3 | the same evidence: passes; no log finding, 23 expected rows | 46 min 45 s (2 804 s), 2026-11-05 |

What has **not** run: the first synchronisation of a large workspace, live documents with many
writers, a connector's reconciliation under volume, the bundle per route, a drain (L2), the small
host's footprint (F1), backup and export at size (B1), and L1 at 10⁶ beyond a 60-second probe.
The second round below closes the first four, which the owner's word of 2026-10-31 names, and keeps L2, F1 and B1 where
they were (not built, each its own probe once it is). It ran on 2026-11-05 (rows S10⁶ and S10⁷
above): the first synchronisation, live documents, the bundle per route and L1 at 10⁶ for its
two-minute window ran; a connector's reconciliation (S6) did not, as it is not built.

## The second round

### The scenarios and what each proves

| # | Scenario | Proves | Built | Pass |
|---|---|---|---|---|
| S1 | **Read**: five read paths that grow with a workspace (a collection's page, open entries by due date, the board by completion, a count over the hub, the hub's level), for the largest, a middle and the smallest credentialled workspace, eight callers paced at 400 a second, 5 s warm-up and 30 s held | how each read decays with entries per workspace | yes — `TestCapacityAtScale` | no 5xx; P95 under SLO-2's 200 ms; a P95 more than twice the previous size's for the same rank is a finding |
| S2 | **Mixed read and write**: the hub's level read by eight callers at 400 a second while two writers add entries at 40 a second, 35 s per workspace | what a write makes the next read pay | yes — the `mixed` part of `TestCapacityAtScale` | reads as S1, writes under SLO-3's 300 ms |
| S3 | **Sync, initial walk**: one null-cursor walk of the largest workspace to its end (`sync:snapshot`, then `:pull` page by page): pages, bytes, total time, how long it holds its snapshot connection; then the browser's narrowed snapshot of the hubs opened lately (offline-sync.md §1) | that a device joining a large workspace gets its copy without a 5xx or a held connection | yes — `TestS3InitialWalk` (`HUBTASK_LOAD_SYNC=1`) | every page within `HUBTASK_REQUEST_TIMEOUT` (30 s), no 5xx; total time and bytes recorded as the first baseline |
| S4 | **Sync, stream**: L1 — 64 change streams of eight credentials while 10 writes a second land | that live delivery keeps pace and the writes stay fast | yes — `TestL1LiveUpdatesInOneWorkspace` | no stream refused, writes P95 < 300 ms, no record later than 30 s or missing, NOTIFY queue under half (A-36) |
| S5 | **Live documents with many writers**: 30 clients on one page's live document across the run's process, each sending an edit once a second and a cursor ten times a second for 2 minutes | that the relay holds its bound and no update is lost | yes — `TestS5LiveDocumentsWithManyWriters` (`HUBTASK_LOAD_DOCUMENTS=1`); the run has one process, whose relay opens no transaction for a cursor (offline-sync.md §13), so the transaction bound is met by construction and the outcomes are what it measures | at most 10 relaying transactions a second per process (offline-sync.md §13), no `too_large` or `failed` in `hubtask_realtime_awareness_relayed_total`, every client's document equal at the end |
| S6 | **Connector reconciliation**: a scheduled pull from a local fake source (the `NETBOX` kind's two endpoints, served by the test) holding more devices than one run's 50-page bound takes, run twice (the second continues from the kept cursor), then a third with 1 % changed | that a run's pages reconcile and commit one by one without holding a transaction, the cursor is kept, and a changed record changes only its CI | yes - `TestS6ConnectorReconciliation` (`HUBTASK_LOAD_CONNECTOR=1`); on demand through `:run` rather than waiting for the schedule's 15 minutes, as many runs as the walk takes before the change (at least two), each page timed by the interval between its run record and the next | no 5xx and no `connector.*` failure; each page's transaction under SLO-3's 300 ms at P95; the third run touches only the changed CIs |
| C1 | **The CMDB ceiling**: the largest workspace's hub filled to `MaxCIsPerHub` (20 000 CIs, 19 800 relations), then `cmdb-health`, `software` and `service-map` each held by eight callers paced at 400 a second, 5 s warm-up and 30 s held | what a view that judges a hub's whole inventory costs at the bound, and that the process stays under its memory target | yes — `TestCmdbViewsAtTheHubCeiling`, run by `make capacity` after S1 | no 5xx; peak resident under `GOMEMLIMIT` (768 MiB); each P95 recorded against SLO-2's 200 ms |
| S7 | **Bundle per route**: the walk of R3 records, per screen, the script and style bytes the browser fetched (gzip as served) and the time until the screen settled | what each product's first visit costs, beyond the first screen's budget (`apps/webapp/build/budget.test.js`, 200 000 gzip bytes) | yes — `walk-<items>.json` beside the capacity run's walk (`HUBTASK_LOAD_WALK=1`) | every screen settles within 30 s with no console error; the bytes per route recorded as the first baseline |

S6's fake source listens on loopback, which the run's server already reaches: the harness opens
the egress guard by exact host (`HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS=true`,
`HUBTASK_HTTP_ALLOWED_HOSTS=127.0.0.1`, `stack_test.go`, for the storm's webhook target). A
connector's address is https only, so the harness mints a root for 127.0.0.1 before the server
starts and names it in `HUBTASK_HTTP_CA_FILE` (accepted.md, seventh answers, item 14;
`source_cert_test.go`); the source serves under it.

### The sizes

| Size | Workspaces | Largest workspace | Runs |
|---|---|---|---|
| 10⁶ | 200 | 170 124 entries | S1 to S7 |
| 10⁷ | 2 000 | 1 222 737 entries | S1, S2, S3, S7 |

At 10⁷ the browser's full snapshot of the largest workspace is about a gigabyte (R3's finding 5);
S3 measures the server's walk there and the browser's narrowed snapshot, never a full browser copy.
S4, S5 and S6 grow with viewers and sources, not with the entries in other workspaces, so they run
at 10⁶ only — one workspace of 170 124 entries is their worst case here, and 10⁷ would add most
of R2's 42.5 minutes to prove nothing more about them.

### How it is shortened without losing what it proves

1. **One seed per size, reused by every scenario.** The package already brings the installation
   up once per `go test` invocation (`runningStack`, `stack_test.go`), so every scenario of a size
   runs in **one** invocation: `-run` names them all. `TestCapacityAtScale` runs first - its S1
   reads, then S2, then the walk of S7 -, then S3, then the writing S4, S5 and S6, so no read of
   S1 measures another scenario's rows. The writers add about 2 600 entries to the largest workspace at 10⁶ (S2 1 400: one pacer of 40 a second
   for 35 s; S4 1 200 in its two-minute window; S5 writes a page, S6 CIs in their own tables) -
   1.5 % of its 170 124.
2. **Seeding once, by COPY.** Already so: COPY files in parts of 500 000 rows, then one
   `VACUUM (ANALYZE)`. A rerun after a fix (as R2 was) seeds again, because the container ends with
   the invocation. A kept data volume (the seeded container left standing under a name, and the
   fix's migrations applied to it by `goose up`) would save the seed on a rerun and time the fix's
   migrations at size as a bonus; it is **not built** and its saving is the seed's share below.
3. **Shorter windows, with the confidence stated.** A P95 from *n* samples is the order
   statistic at rank 0.95 *n* ± 1.96 √(*n* · 0.95 · 0.05) (distribution-free, 95 % confidence):

   | Window at 400 a second | Samples | The reported P95 is a percentile within |
   |---|---|---|
   | 30 s (today) | 12 000 | 94.6 – 95.4 |
   | 15 s | 6 000 | 94.4 – 95.6 |
   | 30 s of a saturated path at 8 a second | 240 | 92.2 – 97.8 |

   S1's and S2's windows are 10.5 minutes at every size, and the setup dominates at 10⁷, so the
   windows stay at 30 s: halving all 18 of them saves 4.5 minutes per size and widens the slow paths' band,
   which are exactly the ones a finding rests on. S4's window is cut from 5 minutes to **2**:
   at 10 writes a second and 64 streams that is 1 200 writes and 76 800 deliveries, and its bars
   are a maximum (no record later than 30 s), which a shorter window tests the same way. The
   harness writes each P95's band beside it (a small change to `harness.Latency`), so a reader
   sees the confidence instead of assuming it.
4. **Sampling.** S3 walks the largest workspace once per size, not every workspace: the walk is
   linear in the workspace's entries, and the largest is the bound. S7 walks the three measured
   workspaces' 11 screens (Today, a hub, a collection and the eight product homes, `productHomes`)
   — 33 screens, the same the R3 walk already visits. S6 reconciles one connector; several
   connectors are separate jobs that do not share a transaction (automation.md §6.1).

### How the errors are collected

Every invocation writes into `HUBTASK_LOAD_EVIDENCE_DIR`:

| What | Level collected | Where it goes | How |
|---|---|---|---|
| The server's log | every `WARN` and `ERROR` (the server runs with `HUBTASK_LOG_LEVEL=warn`, `stack_test.go`) | `server.log` beside the evidence, JSON | `harness.ServerErrors`: one row per message, with its count |
| PostgreSQL's log | every `WARNING`, `ERROR`, `FATAL`, `PANIC` | the container's own output, read at the end | `harness.PostgresErrors`: numbers, quoted values and identifiers masked, then one row per message; a statement's row names its sqlc query (`(in ListWorkItems)`) |
| Failed requests | every 5xx the scenario did not ask for, every status that is not the path's, every refused stream, every failed write | the scenario's own JSON (`server_errors` and `refused` per path, `writes_failed_by_status` and `streams_refused_by_status` in L1's) | the recorder |
| The browser | every console error and warning, uncaught error, failed request and 5xx | `browser` rows | `apps/webapp/e2e/scalewalk.mjs`, folded by `scalewalk-errors.mjs` the same way |

`collectErrors` runs **once per invocation**, in `TestMain`
after the last scenario, over the whole server log and PostgreSQL log, and writes
`errors-<items>.json` with each row's first and last time and the scenarios whose windows it fell
in (each scenario records its start and end in the run's JSON). Expected rows — such as the figure
deadline's cancelled statements while a late count answers (the 10⁷ write-up, finding 4) — are
listed by message in the harness with the reason, counted, and kept out of the findings; any other
row is a finding.

### What happens with what was collected

1. Every row of `errors-<items>.json` that is not expected, every failed request and every
   failed bar is a finding. It is fixed on the branch — a test first that proves it — or, where
   the fix is bigger than one step, becomes a `finding` issue (AGENTS.md).
2. A path whose P95 at 10⁷ is more than twice its P95 at 10⁶ for the same rank is a finding: its
   plan is read (`HUBTASK_LOAD_EXPLAIN=1`, `plans-<items>.json`) and fixed or explained.
3. After a fix, only the scenario it touches runs again, at the size that showed it — at 10⁶ by
   choice, since a 10⁶ rerun costs 3.1 minutes of setup and a 10⁷ one most of R2's 42.5.
4. The runs that are kept are written up under `docs/evidence/` with their JSON, in the shape of
   the 10⁶ and 10⁷ write-ups; the S-12 row of `docs/backlog/suite/README.md` names them.

### Time, per step and in all

From the probes and runs above. The setup at a size is building the binary, the container, the
migrations, the seed, the vacuum and the server's start.

Measured is what a run or probe on this machine took; *cap* is the deadline an unbuilt scenario
gets, so for those the sum is an upper bound.

| Step | 10⁶ | 10⁷ | Source |
|---|---|---|---|
| Setup (build, container, migrations, COPY seed, vacuum, server) | 3.1 min | inside R2's 42.5 min | the probe of 2026-11-05 at 10⁶ |
| S1 read + S2 mixed | 13.4 min | inside R2's 42.5 min | P2's 16.5 min in all less the setup; at 10⁷ R2 measured the setup, S1, S2 and the walk together |
| S7 the walk of 33 screens, bytes per screen | cap 10 min | inside R2's 42.5 min | the walk is R3's; at 10⁶ it has not run |
| S3 sync initial walk | cap 10 min | cap 20 min | not built |
| S4 stream, 2-minute window | about 2.5 min | — | derived from the probe: its 60 s window, the streams' opening and the end within 67 s after the setup |
| S5 live documents, 2-minute window | cap 4 min | — | not built |
| S6 connector, three runs | cap 5 min | — | built; its trial at 6 000 devices took 54 s after the setup, 22 s once its findings were fixed |
| Error collection and teardown | under 1 min | under 1 min | the probe's container ended in the second its test passed |
| **One invocation** | **about 49 min**, 19 of them measured or derived | **about 63 min**, 42.5 of them measured | |

**In all: about 1 hour 50 minutes of load**, in two invocations that can run on different days.
61.5 minutes of it are measured or derived from a probe; 49 minutes are the caps of S3, S5, S6
and the walk at 10⁶, and the first run replaces them by what they take. Not in this sum: building the six pieces below (code
work, not load), one trial of the new scenarios at the default dataset (its setup about 20 s, by
L1 of 2026-11-05), and the reruns after a fix - each at 10⁶ costs the setup's 3.1 minutes plus the
one scenario it reruns.

What these figures rest on:

* **The setup at 10⁶**, measured by a probe on 2026-11-05: `HUBTASK_LOAD_LIVE=1
  HUBTASK_LOAD_LIVE_SECONDS=60 HUBTASK_LOAD_ITEMS=1000000 HUBTASK_LOAD_TENANTS=200 go test
  -tags=load -run '^TestL1LiveUpdatesInOneWorkspace$' ./test/load/`, 247 s in all, the binary
  already in Go's build cache. The container was ready after 8 s, the entries' COPY ran until 2 min
  53 s, the vacuum took 2 s; the 247 s less the 60 s window leave 187 s for the setup, the streams'
  opening and the teardown - at most 3.1 minutes of setup, the seed about 2.7 minutes per 10⁶
  entries.
* **The setup at 10⁷** is not measured on its own: R2's 42.5 minutes hold the setup, S1's and
  S2's 10.5 minutes of windows and R3's walk of 33 screens, and no log of that run keeps the
  phases apart. The table therefore carries R2's total, not a split.
* **S1 and S2** are 3 × (5 × 35 s + 35 s) = 10.5 minutes at every size — the window is fixed, a
  slow path answers fewer requests in it. P2 spent 13.4 minutes after its setup; what the other
  three were (the kept figures' warm-up, the tokens, the end) no log of it records.
* **S4** at 10⁶ is the same probe: 64 of 64 streams, 600 writes with P95 59 ms, delivery P95 301 ms
  and max 325 ms, none late or missing, 1 096 transactions a second, the NOTIFY queue never
  measurably used - every bar of S4 held at 10⁶ for 60 s.
* **S3, S5, S6 and S7's per-route recording** are not built: their figures are the caps each
  scenario gets in the harness, and the run ends a scenario at its cap with a finding rather than
  holding the machine. The first run replaces each cap by its measurement in this table.

What the 10⁷ invocation needs on the workstation: Docker's VM with at least 16 GB of disk for the
database (about 12 GB, 70 % of it indexes) beside its WAL, and about 6 GB in the host's temporary
directory for the COPY files. Nothing else runs on the machine meanwhile: the figures are the
machine's.

### What the round needs built first, in order

1. `collectErrors` once per invocation with the scenarios' windows and the expected rows (above).
   Built: `errors_test.go`, `harness.Report`; trial with L1 for 20 s at the default dataset,
   2026-11-05: one window (S4), no row.
2. Each P95's confidence band in `harness.Latency`. Built: `p95_low`, `p95_high`.
3. S3, the sync walk, as `TestS3InitialWalk` against the shared stack. Built; trial at the default
   dataset, 2026-11-05: the largest workspace's 11 132 records, the snapshot 10 996 379 bytes held
   228 ms, 23 pages in 391 ms (page P95 16 ms), the narrowed snapshot the same records (the
   workspace has one hub), no 5xx; 20 s in all.
4. S5, live documents, beside L1 in `live_test.go`. Built; trial for 20 s at the default dataset,
   2026-11-05: 30 writers, 570 edits, 5 940 cursors, every document and a fresh opener's equal,
   no edit unsent, no connection closed, 5 971 awareness messages `local`, none `too_large` or
   `failed`; 39 s in all.
5. S6, the connector run, with its fake source in the test. Built once the owner's seventh
   answers (item 14) gave the setting that adds a trusted root (`HUBTASK_HTTP_CA_FILE`,
   `0601e7780`). Trial on 2026-11-06 at a small dataset (3 workspaces, 3 000 entries; not the
   10⁶/10⁷ load), 6 000 devices: run 1 33 pages in 25 676 ms, run 2 29 pages in 17 678 ms, run 3
   after 60 changed 2 pages in 444 ms; no 5xx, every run `OK`. Its first trial found that 50
   pages outlast the request's 30 s: the deadline cut the run off, its pages stayed written and
   its cursor did not, and the next run walked them again - fixed (`d0ddb387f`: a run near its
   deadline stops and keeps its cursor; runs 1 and 2 above are that). That trial found two more:
   a page's interval was 670 ms at P50 and 1 133 ms [1 035–2 336] at P95 over 61 pages, over
   SLO-3's 300; and the third run created 13 CIs and updated 47 of the 60 changed - the identifier
   lookup (`FindCmdbIdentifiers`, `LIMIT 5000`) answered every CI sharing the item's manufacturer,
   and the cut dropped the item's own native-key and hostname rows. Both fixed: each lookup (the
   native key, then each whole rule) asks for CIs carrying all its pairs and refuses past its bound
   rather than read a part (`d7d78fb28`), walked from its narrowest pair (`411f0e7da`). Trial again
   on 2026-11-06 at the same dataset, 6 000 devices: run 1 50 pages in 10 493 ms, run 2 12 pages in
   2 135 ms, run 3 after 60 changed 2 pages in 130 ms, created 0 and updated 60; a page's interval
   206 ms at P50 and 227 ms [215–245] at P95 over 61 pages; no 5xx, every run `OK`. The scenario
   passes at the small dataset; the 10⁶/10⁷ load has not run it.
6. S7, per-screen bytes and settle time in `scalewalk.mjs`, the walk of all three workspaces
   under one 10-minute deadline. Built; trial with the capacity run at the default dataset,
   2026-11-05: 33 screens, each settled in 939 to 1 255 ms, no browser row; every screen fetched
   the same 469 files, 2 231 784 script and 197 097 style bytes gzip-compressed (7 978 333 and
   766 165 raw) - each screen's full navigation loads the whole set, a finding for the round
   against the first screen's 200 000. The same trial found S1's three `items:query` paths shed
   (4 660 to 7 060 refusals each, `capacity.shed`, class deferrable): the per-workspace stage
   caps a workspace's deferrable calls at a quarter of the harness's threshold of 8. The capacity
   run now measures at the installation's default threshold (`shedEnv`, `stack_test.go`, `ca3014143`);
   rerun 2026-11-05: no refusal and no server error on any path, every P95 18 ms or less.

None of these is load; each is proven by its own unit test and a run at the default dataset
(about a minute) before the round starts.

### Results of the second round (2026-11-05)

Ran on the owner's workstation (Apple M4 Max, 14 cores, 36 GB; Docker VM 7.9 GB) on the owner's
word (accepted.md, "The owner's word on the second performance round"); every figure is from
[the evidence](../../docs/evidence/second-round-2026-11-05.md) and its JSON. Milliseconds;
P95 with its 95 % band; "largest" is the largest credentialled workspace (170 124 entries at 10⁶,
1 222 737 at 10⁷).

| # | Size | Throughput | P50 / P95 [band] / P99 | Peak resident | Bar |
|---|---|---|---|---|---|
| S1 | 10⁶ | 373.5 to 400 a second per path | every path P95 7 to 25; the largest: page 8/8 [8–8]/10, by due 7/8 [8–8]/9, board 9/11 [10–11]/12, the web app's board 15/17 [17–17]/22, count over the hub 20/25 [25–25]/27 (373.5/s), level 8/8 [8–8]/10 | 209 MiB (S1 and S2) | passes |
| S1 | 10⁷ | 400 a second, but the largest's count over the hub 71.8 and the web app's board 199.1 | the largest: page 8/9 [8–9]/11, by due 8/9 [9–9]/12, board 9/10 [10–10]/12, the web app's board 39/47 [47–48]/52, count 110/119 [119–119]/122, level 8/8 [8–8]/10 | 209 MiB (S1 and S2) | passes SLO-2; the web app's board and the count over the hub over twice their 10⁶ P95 at every rank - explained by entries ×7.2, not fixed |
| S2 | 10⁶ | reads 399.9 to 400, writes 39.8 to 40 a second | reads P95 11 to 13, writes P95 21 to 24; the largest: reads 9/11 [11–11]/15, writes 19/22 [22–24]/30 | in S1's | passes |
| S2 | 10⁷ | the same | reads P95 11 to 12, writes P95 22; the largest: reads 9/11 [11–11]/15, writes 19/22 [22–24]/30 | in S1's | passes |
| S3 | 10⁶ | 426 pages in 6 402 ms | pages 15/16 [16–17]/20, max 42; the snapshot 212 641 records, 198 MB, held 2 734 ms, with its cursor | not measured | passes |
| S3 | 10⁷ | 2 451 pages in 41 635 ms, 1 225 170 records, 1.2 GB | pages 16/17 [17–17]/18, max 69; both snapshots ended at the 512 MiB budget without the cursor (543 127 records, held about 8 s), by design | not measured | passes |
| S4 | 10⁶ | 1 200 writes, 76 800 deliveries in 120 s; 1 096 transactions a second | writes 20/60 [58–62]/69; delivery 185/302 [302–302]/314, max 329; none late or missing; NOTIFY queue 0 | not measured | passes |
| S5 | 10⁶ | 3 570 edits, 35 430 cursors in 120 s; 304 transactions a second | - (it records outcomes, not latencies) | not measured | failed twice, fixed (`b01afe7fc`, `ba349641d`), then passes: documents equal, no edit unsent, no connection closed, 35 461 relayed `local` |
| S6 | - | - | - | - | **did not run** in this round: built afterwards (item 5 above), its trial only |
| S7 | 10⁶, 10⁷ | 33 screens each | settled in 949 to 1 260 (10⁶) and 945 to 1 262 (10⁷) | not measured | passes; every screen fetched the whole precache, 472 files, 2 246 158 script and 198 224 style gzip bytes |
| C1 | 10⁶ | 34.0, 56.2, 21.8 a second | health 235/248 [247–250]/256, software 142/152 [151–152]/157, service map 365/390 [386–398]/429 | **796 MiB, over** `GOMEMLIMIT` 768 MiB | no 5xx; memory over its bar; health's and the service map's P95 over SLO-2 |

The two S5 findings: the live document route inherited the request timeout and every socket
closed at 30 s (`b01afe7fc`); the connection's ping was awaited on its own loop, so a client that
kept sending stalled it until the write bound and was closed as idle at 40 s (`ba349641d`). The
trial of 20 s could see neither. No other row was collected: the server's log and PostgreSQL's
held no finding at either size; at 10⁷ the expected rows were 14 + 9 cancelled figure counts.

Over a target and not fixed: C1's resident memory (796 MiB against 768 MiB; 753 MiB in the
morning's run) and C1's P95 over SLO-2, both of the whole-inventory judgement already named a
finding (final-runs-leftovers.md, "Needs a finding issue"); the 10⁷ throughput of the count over
the hub and the web app's board in the largest workspace. Measured durations replace the caps:
the 10⁶ invocation took 26 min (setup 3 min 8 s, S1 and S2 15 min 45 s, S7 40 s, C1 2 min 16 s,
S4 2 min 1 s, S5 2 min 1 s, S3 12 s), the 10⁷ one 46 min 45 s (setup 29 min 23 s, S1 and S2
15 min 45 s, S7 39 s, S3 58 s). Go ran S3 after S4 and S5 (file order), not before them.

## Kept from the first round, not built

Each is its own probe once it is built, outside the two invocations above.

| # | Run | Command | Machine | Measured or estimated duration |
|---|---|---|---|---|
| B1 | Backup and workspace export of the largest workspace at 10⁶ entries: one backup run to a local and to a throttled S3 target, one tenant export, their durations against `HUBTASK_LONG_JOB_TIMEOUT` (6 h) | not built: the harness has no backup or export path yet | the owner's workstation | **unmeasured** - no backup or export duration at any size is on record |
| L2 | A drain: 256 streams on one of two processes, that process sent `SIGTERM`; when each stream is back, and every refusal | not built | the owner's workstation | **unmeasured**. Pass: every stream back on the other process within 75 s (the 15 s deregistration, the 30 s grace and the client's 30 s backoff cap), each refusal a `503` with `Retry-After`, no other 5xx, no record lost (a `:pull` from the last cursor returns it). Alert threshold: A-26 silent once the streams are back |
| F1 | The footprint of the single small machine: the Compose stack idle for 10 minutes, then P1's mixed window against it; the resident memory of `app` and `db` (`docker stats`) at the end of each | not built: P1 runs the binary, not the Compose stack | the owner's workstation | **unmeasured for the Compose stack.** One reading of the bare binary is on record (support-matrix.md, *Host size*). Pass: the figures are written into support-matrix.md as the minimum (idle) and the recommended size (the peak under load divided by 0.9, the `GOMEMLIMIT` share, rounded up), and the commented `mem_limit` in `compose.yaml` is set to the recommendation |

No soak run is planned: several hours of load on the workstation is too much (the owner,
2026-10-31); one would be its own proposal with its own probe.
