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

What has **not** run: the first synchronisation of a large workspace, live documents with many
writers, a connector's reconciliation under volume, the bundle per route, a drain (L2), the small
host's footprint (F1), backup and export at size (B1), and L1 at 10⁶ beyond a 60-second probe.
The second round below closes the first four, which the owner's word of 2026-10-31 names, and keeps L2, F1 and B1 where
they were (not built, each its own probe once it is).

## The second round

### The scenarios and what each proves

| # | Scenario | Proves | Built | Pass |
|---|---|---|---|---|
| S1 | **Read**: five read paths that grow with a workspace (a collection's page, open entries by due date, the board by completion, a count over the hub, the hub's level), for the largest, a middle and the smallest credentialled workspace, eight callers paced at 400 a second, 5 s warm-up and 30 s held | how each read decays with entries per workspace | yes — `TestCapacityAtScale` | no 5xx; P95 under SLO-2's 200 ms; a P95 more than twice the previous size's for the same rank is a finding |
| S2 | **Mixed read and write**: the hub's level read by eight callers at 400 a second while two writers add entries at 40 a second, 35 s per workspace | what a write makes the next read pay | yes — the `mixed` part of `TestCapacityAtScale` | reads as S1, writes under SLO-3's 300 ms |
| S3 | **Sync, initial walk**: one null-cursor walk of the largest workspace to its end (`sync:snapshot`, then `:pull` page by page): pages, bytes, total time, how long it holds its snapshot connection; then the browser's narrowed snapshot of the hubs opened lately (offline-sync.md §1) | that a device joining a large workspace gets its copy without a 5xx or a held connection | **no** (was L3) | every page within `HUBTASK_REQUEST_TIMEOUT` (30 s), no 5xx; total time and bytes recorded as the first baseline |
| S4 | **Sync, stream**: L1 — 64 change streams of eight credentials while 10 writes a second land | that live delivery keeps pace and the writes stay fast | yes — `TestL1LiveUpdatesInOneWorkspace` | no stream refused, writes P95 < 300 ms, no record later than 30 s or missing, NOTIFY queue under half (A-36) |
| S5 | **Live documents with many writers**: 30 clients on one page's live document across the run's process, each sending an edit once a second and a cursor ten times a second for 2 minutes | that the relay holds its bound and no update is lost | **no** (L4, widened from awareness to edits) | at most 10 relaying transactions a second per process (offline-sync.md §13), no `too_large` or `failed` in `hubtask_realtime_awareness_relayed_total`, every client's document equal at the end |
| S6 | **Connector reconciliation**: a scheduled pull from a local fake source (the `NETBOX` kind's two endpoints, served by the test) holding more devices than one run's 50-page bound takes, run twice (the second continues from the kept cursor), then a third with 1 % changed | that a run's pages reconcile and commit one by one without holding a transaction, the cursor is kept, and a changed record changes only its CI | **no** | no 5xx and no `connector.*` failure; each page's transaction under SLO-3's 300 ms at P95; the third run touches only the changed CIs |
| C1 | **The CMDB ceiling**: the largest workspace's hub filled to `MaxCIsPerHub` (20 000 CIs, 19 800 relations), then `cmdb-health`, `software` and `service-map` each held by eight callers paced at 400 a second, 5 s warm-up and 30 s held | what a view that judges a hub's whole inventory costs at the bound, and that the process stays under its memory target | yes — `TestCmdbViewsAtTheHubCeiling`, run by `make capacity` after S1 | no 5xx; peak resident under `GOMEMLIMIT` (768 MiB); each P95 recorded against SLO-2's 200 ms |
| S7 | **Bundle per route**: the walk of R3 records, per screen, the script and style bytes the browser fetched (gzip as served) and the time until the screen settled | what each product's first visit costs, beyond the first screen's budget (`apps/webapp/build/budget.test.js`, 200 000 gzip bytes) | **partly**: the first screen's budget is a test; per route nothing | every screen settles within 30 s with no console error; the bytes per route recorded as the first baseline |

S6's fake source listens on loopback, which the run's server already reaches: the harness opens
the egress guard by exact host (`HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS=true`,
`HUBTASK_HTTP_ALLOWED_HOSTS=127.0.0.1`, `stack_test.go`, for the storm's webhook target).

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

Today only `TestCapacityAtScale` writes `errors-<items>.json`; L1 collects nothing from the logs.
The change this round needs first: `collectErrors` runs **once per invocation**, in `TestMain`
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
| S6 connector, three runs | cap 5 min | — | not built |
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
2. Each P95's confidence band in `harness.Latency`.
3. S3, the sync walk, as `TestS3InitialWalk` against the shared stack.
4. S5, live documents, beside L1 in `live_test.go`.
5. S6, the connector run, with its fake source in the test.
6. S7, per-screen bytes and settle time in `scalewalk.mjs`, the walk of all three workspaces
   under one 10-minute deadline (today 10 minutes per workspace, `capacity_test.go`).

None of these is load; each is proven by its own unit test and a run at the default dataset
(about a minute) before the round starts.

## Kept from the first round, not built

Each is its own probe once it is built, outside the two invocations above.

| # | Run | Command | Machine | Measured or estimated duration |
|---|---|---|---|---|
| B1 | Backup and workspace export of the largest workspace at 10⁶ entries: one backup run to a local and to a throttled S3 target, one tenant export, their durations against `HUBTASK_LONG_JOB_TIMEOUT` (6 h) | not built: the harness has no backup or export path yet | the owner's workstation | **unmeasured** - no backup or export duration at any size is on record |
| L2 | A drain: 256 streams on one of two processes, that process sent `SIGTERM`; when each stream is back, and every refusal | not built | the owner's workstation | **unmeasured**. Pass: every stream back on the other process within 75 s (the 15 s deregistration, the 30 s grace and the client's 30 s backoff cap), each refusal a `503` with `Retry-After`, no other 5xx, no record lost (a `:pull` from the last cursor returns it). Alert threshold: A-26 silent once the streams are back |
| F1 | The footprint of the single small machine: the Compose stack idle for 10 minutes, then P1's mixed window against it; the resident memory of `app` and `db` (`docker stats`) at the end of each | not built: P1 runs the binary, not the Compose stack | the owner's workstation | **unmeasured for the Compose stack.** One reading of the bare binary is on record (support-matrix.md, *Host size*). Pass: the figures are written into support-matrix.md as the minimum (idle) and the recommended size (the peak under load divided by 0.9, the `GOMEMLIMIT` share, rounded up), and the commented `mem_limit` in `compose.yaml` is set to the recommendation |

No soak run is planned: several hours of load on the workstation is too much (the owner,
2026-10-31); one would be its own proposal with its own probe.
