# C1, the CMDB views at the hub's ceiling — the owner's workstation

**2026-11-05, run C1 of [test/load/PLAN.md](../../test/load/PLAN.md), test code of commit
`ef3de3fa3` (run before the memory reader took its caller's context, which changes no figure).** `HUBTASK_LOAD_CAPACITY=1 go test -tags=load -run '^TestCmdbViewsAtTheHubCeiling$'
-timeout=30m -v ./test/load/` with the default dataset (40 000 entries over 20 workspaces). The
largest workspace's hub was filled to `MaxCIsPerHub`: 20 000 CIs (200 application services,
15 800 servers, 4 000 software CIs) and 19 800 relations (`runs_on`, `installed_on`), the assets
module on. Each view was then held by eight callers paced at 400 a second, 5 s warm-up and 30 s
held, one after another. One process serving every role, `GOMEMLIMIT` 768 MiB, against
PostgreSQL 16 (pgvector image) in a container. The run's JSON is
[`cmdb-ceiling-2026-11-05/cmdb-ceiling.json`](cmdb-ceiling-2026-11-05/cmdb-ceiling.json).

This is the workstation tier of accepted.md D, not the release tier on named hardware.

| | |
|---|---|
| Machine | Apple M4 Max, 14 cores, 36 GB; Docker VM 7.9 GB |
| Run time | 153 s, setup included |
| Server errors | none on any view; the server's log (warn and above) stayed empty |

| View | Answers a second | P50 ms | P95 ms | P99 ms | Max ms | Answers |
|---|---|---|---|---|---|---|
| `GET /hubs/{hubId}/cmdb-health` | 33.6 | 236 | 257 | 300 | 365 | 1 007 |
| `GET /hubs/{hubId}/software` | 54.9 | 137 | 171 | 327 | 730 | 1 647 |
| `GET /hubs/{hubId}/service-map` | 20.2 | 357 | 583 | 860 | 955 | 605 |

Peak resident memory of the server over the three windows: 790 085 632 bytes (753 MiB), read by
`ps` once a second, against the run's `GOMEMLIMIT` of 768 MiB and the chart's 1 GiB limit
(`k8s/values.yaml`).

An earlier run the same morning, before the memory was read on this platform, measured
35.1/s and P95 240 ms (health), 58.5/s and P95 145 ms (software), 22.3/s and P95 387 ms
(service map), no server error: the same order.

**Verdict.** The bound holds: every view answers at the ceiling without a server error, and the
process stays under its memory target. Each request costs the server a whole inventory read and
judgement — eight callers saturate it at 20 to 55 answers a second, so the latency is mostly the
request's own work. Health's and the service map's P95 lie above SLO-2's 200 ms at the ceiling;
software's lies under it. Moving the judgement into SQL aggregation is the fix and more than one
step (final-runs-leftovers.md, "Needs a finding issue").

What it does not show: several hubs at the ceiling at once, a hub at the ceiling under writers,
or a smaller hub's figures (the cost grows with the hub's own inventory, not the workspace's).
