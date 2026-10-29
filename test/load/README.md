# The load suite

Two tiers, because they answer different questions and only one of them can be answered on a
shared runner. The rule is
[`observability-reliability.md` §13.2](../../docs/architecture/observability-reliability.md).

| | The nightly tier | The release tier |
|---|---|---|
| Runs | Every night, in `nightly.yml` | Per release, by hand |
| Where | `ubuntu-latest`, shared | Named hardware — the integration server (CI-1) |
| Dataset | 40 000 items over 20 tenants, seeded into a container | 2 000 000 items over 200 tenants, seeded by script into a real stack |
| Answers | Did anything get significantly worse than the stored baseline? | What does this installation actually do, per vCPU, at a held P95? |
| Publishes | Nothing | Nothing yet — internal until the figures are stable |

A shared runner varies by 10–30 % between runs. That is why the nightly tier compares against a
baseline with a band instead of against a target, and why it compares only against a baseline
recorded on the same hardware: comparing across machines measures the machine.

## Running it

```bash
make gate-load                       # everything, against a container it brings up itself
go test -tags load ./test/load/ -run RT6 -v
```

Both need Docker. The suite starts PostgreSQL, applies the migrations, seeds the dataset, builds
`cmd/server` and runs the real binary — RT-6 asks what a process does under overload, and a stack
wired together inside a test is a different process from the one that is deployed.

| Variable | Default | What it is |
|---|---|---|
| `HUBTASK_LOAD_TENANTS` | `20` | Tenants in the seeded dataset |
| `HUBTASK_LOAD_ITEMS` | `40000` | Items across all of them, in a long tail |
| `HUBTASK_LOAD_HARDWARE` | `unnamed-<os>-<arch>` | The machine's name. The guard compares only against a baseline recorded under the same one |
| `HUBTASK_LOAD_EVIDENCE_DIR` | a temporary directory | Where each run leaves its JSON |

## The capacity run (F-10)

```bash
make capacity                                   # 10⁶ entries over 200 workspaces
make capacity ITEMS=10000000 TENANTS=2000       # 10⁷, on named hardware only
```

`capacity_test.go` seeds the dataset into its own container (through files, so 10⁷ rows never sit
in memory), starts the real binary and holds each read path that grows with a workspace - a
collection's page, an open-entries query sorted by due date, the board grouped by completion, a
count over the hub, the hub's level - for the largest, a middle and the smallest credentialled
workspace: 30 s each after a 5 s warm-up, eight callers, paced at 400 requests a second. It writes
`capacity-<items>.json` with requests per second per vCPU and P50 / P95 / P99 per path and
workspace, which is how the decay with entries per workspace is read. It asserts only that every
path answered without a server error; what the figures mean is the release tier's to judge, and a
cell no run filled stays *not measured*. It skips unless `HUBTASK_LOAD_CAPACITY` is set, so
`make gate-load` does not run it.

## The release tier, step by step

```bash
export HUBTASK_DB_DSN='postgres://postgres:…@…/hubtask?sslmode=disable'   # the owner, not the app role
scripts/seed-load-dataset.sh --items 2000000 --tenants 200
HUBTASK_LOAD_HARDWARE=integration HUBTASK_LOAD_ITEMS=2000000 HUBTASK_LOAD_TENANTS=200 \
  HUBTASK_LOAD_EVIDENCE_DIR=./load-evidence make gate-load
```

Then the run that is worth keeping is written up under [`docs/evidence/`](../../docs/evidence/),
with the JSON beside it. The figure that is recorded is **requests per second per vCPU at a held
P95, and how it decays with items per tenant**. A concurrent-user count is a derived figure and
never a headline: it is throughput divided by a behaviour model, and the model moves it by an order
of magnitude — 500 req/s is 5 000 users at six requests a minute each, or 500 users at sixty. If a
user count appears anywhere, the model appears beside it.

## What is here

| | |
|---|---|
| `harness/` | The generator: the ramp, the pacer, the recorder, and the baseline comparison. Untagged, so its arithmetic runs in `gate-unit` — a guard that has quietly stopped working must redden a pull request rather than pass a nightly |
| `seed/` | The dataset generator. Writes COPY text, holds no database driver |
| `baselines/` | The stored figures, each with the band and the floor that say what "significantly worse" means |
| `stack_test.go` | The installation: a container, the migrations, the dataset, the real binary |
| `rt6_test.go` | RT-6 — overload, shedding, the interactive target, memory |
| `storm_test.go` | The automation storm, and the fairness of multi-tenancy §4 asserted rather than eyeballed |
| `guard_test.go` | The relative regression guard |
| `capacity_test.go` | The capacity run at 10⁶ / 10⁷ entries (`make capacity`) |

## Refreshing a baseline

A baseline is taken from the **worst** of several runs of unchanged code, not the best. The
quietest minute of a machine is not a figure that machine can reproduce, and a baseline it cannot
reproduce is a red build every other night.

The interactive P99 is measured and recorded and deliberately not guarded: at this scale it is the
tail of a few hundred requests, and one garbage collection during the run moves it by an order of
magnitude. A figure that reports the machine is a figure somebody turns off.

A baseline is refreshed when the change that moved it is understood — never to make a red build
green. Take the figures from `guard-latest.json` of a run on the machine the baseline names,
write them into `baselines/steady-state.json` with the date, and say in the pull request what moved and why.
