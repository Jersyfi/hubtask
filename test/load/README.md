# The load suite

Two tiers, because they answer different questions and only one of them can be answered on a
shared runner. The split is the owner's decision of 2026-08-21, recorded as decision 7 in
[`milestone-0.6.0.md`](../../docs/backlog/milestone-0.6.0.md).

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
| `baselines/` | The stored figures, each with the band that says what "significantly worse" means |
| `stack_test.go` | The installation: a container, the migrations, the dataset, the real binary |
| `rt6_test.go` | RT-6 — overload, shedding, the interactive target, memory |
| `storm_test.go` | The automation storm, and H-08's fairness asserted rather than eyeballed |
| `guard_test.go` | The relative regression guard |

## Refreshing a baseline

A baseline is refreshed when the change that moved it is understood — never to make a red build
green. Take the figures from `H-11-guard-latest.json` of a run on the machine the baseline names,
write them into `baselines/steady-state.json` with the date, and say in the pull request what moved and why.
