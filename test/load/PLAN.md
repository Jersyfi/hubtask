# The plan for the long performance runs

The owner asked for this plan before the long runs start (accepted.md, "The owner's word on the
performance tests", 2026-10-31): what runs, how long it really takes — measured by short probes,
not guessed — and how every error a run sees is collected, deduplicated and afterwards fixed. It
was shown to the owner, who accepted it with R3 on 2026-10-31 (accepted.md, "The owner's fourth
answers", item 13): P2, R1 and R3 run as written, R2 only after a fix. The commands
and the dataset are in [README.md](./README.md); the rule they answer is
[`observability-reliability.md` §13.2](../../docs/architecture/observability-reliability.md).

## What the runs are for

F-10 promises up to 10⁷ entries per installation. The evidence so far is the 10⁶ run of
2026-10-31 ([capacity-1000000-2026-10-31.md](../../docs/evidence/capacity-1000000-2026-10-31.md)),
on which hash partitioning was decided against (§13.3). What a 10⁷ run adds is the one question
10⁶ cannot answer: whether a read path's plan or a lock changes shape once the largest workspace
holds about 1.7 million entries instead of 170 000.

## The runs, in order

| # | Run | Command | Machine | Measured or estimated duration |
|---|---|---|---|---|
| P1 | Probe, 10⁵ entries over 20 workspaces | `make capacity ITEMS=100000 TENANTS=20` | the owner's workstation | 14 minutes (839 s), the seed well under one; no server error on any path (measured 2026-10-31, commit `0b2f6ba47`) |
| P2 | Probe, 10⁶ entries over 200 workspaces - run again, because the 10⁶ evidence predates the mixed window and carries no `mixed` | `make capacity` | the owner's workstation | about 14 minutes, seed about 3 (measured 2026-10-31, the 10⁶ evidence) |
| R1 | The capacity run, 10⁷ entries over 2 000 workspaces | `make capacity ITEMS=10000000 TENANTS=2000` | the owner's workstation (third answers 9) | **about 50 to 60 minutes**, of which the seed about 30, the vacuum and the indexes up to 10, the measuring 11 |
| R2 | If R1 found and fixed anything: R1 again on the fix | the same | the same | the same |
| B1 | Backup and workspace export of the largest workspace at 10⁶ entries: one backup run to a local and to a throttled S3 target, one tenant export, their durations against `HUBTASK_LONG_JOB_TIMEOUT` (6 h) | not built: the harness has no backup or export path yet | the owner's workstation | **unmeasured** - no backup or export duration at any size is on record |
| L1 | Live updates in one workspace: 64 change streams (eight credentials, under the per-credential cap of 8) held open while 10 writes a second land for 5 minutes; database transactions per second, the writes' P95 and every stream's delivery delay recorded | not built: the harness opens no stream | the owner's workstation | **unmeasured**. Pass: no stream refused (64 is under the tenant cap of 512), the writes' P95 within SLO-3 (300 ms), no record later than SLO-4's 30 s. Alert threshold: A-26 (`hubtask_stream_refused_total`) and A-36 (NOTIFY queue over half full) stay silent |
| L2 | A drain: 256 streams on one of two processes, that process sent `SIGTERM`; when each stream is back, and every refusal | not built | the owner's workstation | **unmeasured**. Pass: every stream back on the other process within 75 s (the 15 s deregistration, the 30 s grace and the client's 30 s backoff cap), each refusal a `503` with `Retry-After`, no other 5xx, no record lost (a `:pull` from the last cursor returns it). Alert threshold: A-26 silent once the streams are back |
| L3 | The first synchronisation of the largest workspace of P2 (about 170 000 entries): one null-cursor walk to the end, its pages, bytes and time, and how long it holds its snapshot connection; then the browser's: the snapshot ended past 100 000 entries and the narrowed snapshot of the hubs opened lately (offline-sync.md §1), its time, bytes and entries held | not built: the harness reads five paths, not the walk | the owner's workstation | **unmeasured**. Pass: every page within `HUBTASK_REQUEST_TIMEOUT` (30 s) and without a 5xx; the total time and bytes are recorded as the baseline the next run is compared with (no bar until one exists) |
| L4 | Awareness: 30 clients on one live document across two processes, each sending a cursor ten times a second for 2 minutes; the NOTIFY rate on `hubtask_awareness` and the relay outcomes | not built | the owner's workstation | **unmeasured**. Pass: at most 10 relaying transactions a second per process (offline-sync.md §13's bound), no `too_large` or `failed` in `hubtask_realtime_awareness_relayed_total`. Alert threshold: A-22 and A-36 stay silent |
| F1 | The footprint of the single small machine: the Compose stack idle for 10 minutes, then P1's mixed window against it; the resident memory of `app` and `db` (`docker stats`) at the end of each | not built: P1 runs the binary, not the Compose stack | the owner's workstation | **unmeasured for the Compose stack.** One reading of the bare binary is on record (support-matrix.md, *Host size*). Pass: the figures are written into support-matrix.md as the minimum (idle) and the recommended size (the peak under load divided by 0.9, the `GOMEMLIMIT` share, rounded up), and the commented `mem_limit` in `compose.yaml` is set to the recommendation |

Why the durations are what they are, from the probes:

* **The measuring is fixed.** Three workspaces (the largest, a middle one, the smallest with a
  credential), five read paths each, 5 s warm-up and 30 s held, then the mixed read-and-write
  window of 35 s: 3 × (5 × 35 s + 35 s) = 10.5 minutes at every size. A slower path answers
  fewer requests in the window; it does not lengthen it.
* **The seed grows linearly.** COPY in parts of 500 000 rows (no statement queues ten million
  after-triggers, migration 0289), then `VACUUM (ANALYZE)`. P1 and P2 give the rate:
  10⁵ and 10⁶ both take 14 minutes in all, so everything but the seed - building the binary,
  the migrations, the start and the measuring - is about 11 minutes whatever the size, and the
  seed is about 3 minutes per 10⁶ entries; at that rate 10⁷ entries take about 30 minutes, if the
  index builds stay linear. That is the one figure no probe can measure below 10⁷.
* **The stack's own budget** is twenty minutes plus ten per 10⁶ entries — 120 minutes at 10⁷ — so
  a seed that stalls (as the first 10⁶ attempt did, quadratically) ends the run instead of holding
  the machine for hours.
* **The live paths are not in R1.** L1 to L4 measure what grows with the number of viewers rather
  than with the data - streams, a drain's reconnects, the first synchronisation, awareness - and F1
  the smallest host. None is built yet; each is a probe of a few minutes once it is, and none
  changes P2, R1 or R3.
* **Several hours of load is not planned.** No soak run is in this plan; one would be its own
  proposal with its own probe.

What R1 needs on the workstation: Docker's VM with at least 16 GB of disk for the database
(about 12 GB at 10⁷, 70 % of it indexes) beside its WAL, and about 6 GB in the host's temporary
directory for the COPY files. Nothing else should run on the machine meanwhile: the figures are
the machine's.

## How errors are collected and deduplicated

Every run writes, into `HUBTASK_LOAD_EVIDENCE_DIR`:

| File | What it holds | Source |
|---|---|---|
| `capacity-<items>.json` | per path and workspace: rate, P50 / P95 / P99, `server_errors` and the answers `by_status` | the harness's recorder — **failed requests** are every 5xx the harness did not ask for, and every status that is not the path's |
| `server.log` | the server's whole output, JSON | the real binary's stdout and stderr |
| `errors-<items>.json` | **one row per message**: source, level, message, count | `harness.ServerErrors` over `server.log` (every `WARN` and `ERROR` entry, grouped by its message) and `harness.PostgresErrors` over PostgreSQL's log (every `WARNING`, `ERROR`, `FATAL` and `PANIC` line, grouped with numbers, quoted values and identifiers masked) |

The run's log also prints each row (`server WARN ×12: …`), so a run read in the terminal shows the
same list. **Browser console errors** are not part of R1: the capacity run drives the API and no
browser. A walk of the web app against the 10⁷ dataset, with every console error and failed
request of the page counted the same way (`page.on('console')` and `page.on('requestfailed')`, as
`apps/webapp/e2e/fixture.mjs` already records `pageerror`), is run R3, accepted by the owner (fourth answers 13; about the seed's 40 minutes plus the walk).
It is `HUBTASK_LOAD_WALK=1` on R1's command ([README.md](./README.md)): `apps/webapp/e2e/scalewalk.mjs`
records `console`, `pageerror`, `requestfailed` and every 5xx, folded into `browser` rows of
`errors-<items>.json` (`scalewalk-errors.test.mjs`).

## What happens with what was collected

1. Every row of `errors-<items>.json`, every server error and every failed request is a finding.
   A finding is fixed on the branch — a test first that proves it — or, where the fix is bigger
   than one step, it becomes a `finding` issue (AGENTS.md).
2. A path whose P95 at 10⁷ is more than twice its P95 at 10⁶ for the largest workspace is a
   finding too: its plan is read (`EXPLAIN (ANALYZE, BUFFERS)`) and fixed or explained.
3. The run that is kept is written up as `docs/evidence/capacity-10000000-<date>.md` with its JSON
   beside it, in the shape of the 10⁶ evidence; the S-12 row of `docs/backlog/suite/README.md`
   names it.
4. R2 repeats R1 only when R1 led to a fix, and its write-up replaces R1's.
5. P2's `capacity-1000000.json` replaces the one in `docs/evidence/capacity-1000000-2026-10-31/`;
   the P95 of the hub's level under writers is added to that write-up and to
   `observability-reliability.md` §13.2, and the S-12 row's *Reads under a mix of writes are not
   measured* is closed by it - or stays open with what the run found.
