# Support Matrix

What Hubtask is supported on, and for every row **the CI job that proves it**. `make gate-docs`
reconciles this table with the workflows in both directions: a row naming a job that does not
exist fails the build, and so does a `matrix-*` job that no row names or that the nightly's
reporting job does not wait on.

* **Scope:** the server is a container, and only a container
  ([ADR-0014](../adr/ADR-0014-single-image-multi-role.md), [deployment.md](./deployment.md)).

---

## 1. What "supported" means here

| Status | Meaning |
|---|---|
| `supported` | A CI job runs the software on it. A defect there is a release blocker. |
| `best effort` | Expected to work, not proven by a job. A defect there is an ordinary bug — reported, fixed when it can be, never a release blocker. |
| `unsupported` | Not intended. A defect there is closed with a pointer to this table. |

**The server ships as a container**, so the host operating system hardly matters — macOS and
Windows run the same Linux container through Docker Desktop. What varies, and is tested, is the
**container runtime**, the **CPU architecture** and the **PostgreSQL major**.

---

## 2. The server

| Runtime | Architecture | Status | Proven by |
|---|---|---|---|
| Docker (Compose) | linux/amd64 | `supported` | `ci.yml:compose` |
| Docker (Compose) | linux/arm64 | `supported` | `nightly.yml:matrix-arm64` |
| Podman (Compose) | linux/amd64 | `supported` | `nightly.yml:matrix-podman` — `podman build`, then Compose v2 against Podman's Docker-compatible socket, the path `podman compose` takes. **`podman-compose` 1.0.6** (Debian's and Ubuntu's Python reimplementation) **does not run this stack**: it starts each container with `podman start`, and Podman refuses a dependency graph holding the exited migration container. `ci.yml:compose` starts the stack out of order to prove the artefact's half |
| Kubernetes ≥ 1.28 | linux/amd64 | `supported` | `nightly.yml:matrix-kind` |
| Kubernetes ≥ 1.28 | linux/arm64 | `best effort` | — the image is multi-arch and the chart architecture-agnostic; no ARM cluster runs in CI |
| Docker Desktop | macOS, Windows | `best effort` | — the same Linux container; the runtime differences are Docker's |
| A bare binary (no container) | any | `unsupported` | — the image carries the migrator, the locales and the CA bundle; a loose binary is a support surface nobody has scoped |

## 3. The runtime environment

| Component | Version | Status | Proven by |
|---|---|---|---|
| PostgreSQL | 16 | `supported` | `ci.yml:integration` |
| PostgreSQL | 17 | `supported` | `nightly.yml:matrix-postgres` |
| PostgreSQL **with pgvector** (semantic search) | 16, 17 | `supported` | `ci.yml:integration` (`pgvector/pgvector:pg16`), `nightly.yml:matrix-postgres` (`pg17`). The extension is **detected, not demanded** ([ADR-0050](../adr/ADR-0050-pgvector-as-a-capability.md)): without it an installation migrates, runs and searches lexically, and `/meta/capabilities` answers `semantic_search: false`. `TestTheMigrationsApplyWithoutPgvector` proves that path on a plain `postgres:16-alpine` |
| PostgreSQL **built with ICU** (names sort the same everywhere) | 16, 17 | `supported` | `ci.yml:integration` — `TestNamesSortUnderTheICURootCollation` proves migration `0080` copied `und-x-icu` into `hubtask_name`. A build without ICU is **not unsupported**: names sort in the database's own locale and `/meta/capabilities` answers `natural_ordering: false` (`TestTheFallbackCollationIsTheDatabasesOwnLocale`) |
| PostgreSQL | ≤ 15 | `unsupported` | — the schema uses what 16 offers, and nothing checks 15. The hard floor is 15, which added `ON DELETE SET NULL (column)`; the tenant-scoped foreign keys need it ([ADR-0024](../adr/ADR-0024-tenant-scoped-foreign-keys.md)) |
| PostgreSQL without a superuser (a managed service) | 16, 17 | `supported` | `ci.yml:integration` — the migrations applied by an owner with `CREATEROLE` and no superuser, and the tenant boundary asserted afterwards ([ADR-0052](../adr/ADR-0052-managed-postgresql-support.md)). [multi-tenancy.md](./multi-tenancy.md) §2.1 says how the operator sets the roles up |
| Go (building from source) | 1.27 | `supported` | `ci.yml:quick` and every other job |

## 4. `hubctl` (the CLI)

The one artefact that runs **natively** on a user's machine, so its matrix is about operating
systems. A job here proves that the binary *starts* on its platform; cross-compilation already
proves that it links, for every platform on every release.

| Platform | Architecture | Status | Proven by |
|---|---|---|---|
| Linux | amd64 | `supported` | `ci.yml:e2e` |
| Linux | arm64 | `supported` | `nightly.yml:matrix-hubctl` |
| macOS | arm64 (Apple silicon) | `supported` | `nightly.yml:matrix-hubctl` |
| Windows | amd64 | `supported` | `nightly.yml:matrix-hubctl` |
| macOS | amd64 (Intel) | `best effort` | — cross-compiled and published; GitHub's Intel macOS runners are being withdrawn, and a row may not rest on a runner that is going away |
| Windows | arm64 | `best effort` | — cross-compiled and published; no ARM Windows runner exists on the free tier |

The Linux amd64 row points at the pull request's end-to-end job on purpose: it runs the whole
`hubctl` session against the reference stack on every pull request.

### 4.1 The SDKs

Generated from `api/openapi.yaml`, with no runtime dependency beyond what their language needs to
speak HTTP.

| SDK | Runtime | Status | Proven by |
|---|---|---|---|
| Go (`sdk/go`) | the Go of §3 | `supported` | `ci.yml:integration` — the contract tests drive the generated client against the in-process server |
| Python (`sdk/python`) | the runner image's `python3` (3.12 on `ubuntu-latest`) | `supported` | `ci.yml:integration` — the contract tests drive the SDK's example against the in-process server; `ci.yml:quick` proves the generated package parses |
| Python (`sdk/python`) | 3.11 | `best effort` | — **the SDK requires Python 3.11 or later** (`requires-python = ">=3.11"`); no job runs 3.11 itself |
| Python | ≤ 3.10 | `unsupported` | — below the package's declared floor |
| TypeScript (`sdk/typescript`) | a runtime with `fetch` | `best effort` | — the generator and its output are tested in the workspace job; no job runs the client against a server |

---

## 5. The browser clients

The web application is embedded in the binary ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) and
the website is prerendered ([ADR-0030](../adr/ADR-0030-svelte-frontend-framework.md)); which
browsers they run in is [ADR-0044](../adr/ADR-0044-browser-support-row.md).

| Engine | Versions | Status | Proven by |
|---|---|---|---|
| Chromium (Chrome, Edge) | current and previous major | `supported` | `engines` job in `ci.yml`: Playwright's Chromium at the pinned package version, the built bundle loaded and ADR-0044's feature table asserted |
| Gecko (Firefox) | current and previous major | `best effort` | — the client uses only standards Firefox ships; the job's assertions run here by hand with `ENGINES=firefox` ([ADR-0089](../adr/ADR-0089-check-once-where-it-is-cheap.md)) |
| WebKit (Safari) | current and previous major | `best effort` | — the same, with `ENGINES=webkit`: Playwright's WebKit is a Linux build of the engine, not Safari, and failed in CI more often by the runner than by the client ([ADR-0089](../adr/ADR-0089-check-once-where-it-is-cheap.md)) |
| Anything older | — | `unsupported` | — the client uses `<dialog>`, `inert` and `:has()`, and none of the three has a fallback |

**What `supported` stands on here.** The `engines` job ([ADR-0048](../adr/ADR-0048-browser-job-driver.md))
loads the bundle that ships, served the way the binary serves it, in Chromium, and asserts every
fact the client is built on — `<dialog>`, `inert`, `:has()`, `popover`, logical
properties and CSS Anchor Positioning, none with a fallback. It is not a journey. It gates through
`CI required` ([ci-cd.md](./ci-cd.md) §3.2). It runs the versions Playwright's pinned release
bundles, which track the current major; the previous major is not run and stays on the row because
the features are years old in every engine.

## 6. Maintaining this table

1. A new row needs a job **in the same pull request**; the gate refuses the row otherwise.
2. Removing support is deleting the row *and* the job. Deleting only the job turns the build red.
3. `best effort` and `unsupported` rows have no job and carry a dash plus the reason a bug reporter
   reads.
4. A failing nightly matrix job files an issue automatically (label `finding`)
   ([ci-cd.md](./ci-cd.md) §9).

## 7. The installed clients

The installable clients are **Tauri 2 shells** — up to `2.0` for Windows, Linux and Android as direct downloads outside every store, with macOS, iOS and the store listings in `2.0` ([ADR-0084](../adr/ADR-0084-the-first-majors-shells-cost-nothing-to-ship.md)); in the end for Windows, macOS, Linux, iOS and Android, wrapping
the one web codebase ([ADR-0031](../adr/ADR-0031-tauri-app-shell.md)) — the one install path per
platform. The web app is a browser application only: no manifest-based install, no install prompt,
no service-worker install. The installed clients carry the offline promise; the browser holds a
best-effort copy ([offline-sync.md](./offline-sync.md) §1).

Each app renders the web app in its system's own webview, so the row is the engine, and the job
walks the app in it: the smoke walk of UC-SYN-07 check 9 - sign in through the workspace's consent,
open a hub, create, edit and complete an entry, sign out - against a server built from the same
commit, through the engine's own WebDriver (`apps/webapp/e2e/webview/smoke.mjs`). The build walked
is the debug build with the `walk` feature, which stands in for the system's browser and nothing
else. A failure in one engine fails the job.

| App | Engine | Status | Proven by |
|---|---|---|---|
| Windows | WebView2, the Evergreen runtime | `supported` | `ci.yml:shell`, the Windows leg: the walk through the runner's msedgedriver |
| Linux (AppImage, `.deb`) | WebKitGTK 4.1 | `supported` | `ci.yml:shell`, the Linux leg: the walk through WebKitWebDriver under Xvfb |
| Android | Android System WebView | `supported` | `ci.yml:shell`, the Android leg: the walk through the runner's chromedriver on an API 35 emulator (x86_64) |

## 8. Host size

What is measured, and only that:

| Reading | `app` (all four roles in one process) | `db` | Where and how |
|---|---|---|---|
| Idle, a fresh installation, no workspace set up, 10 minutes after start | about 150 MiB resident, 2.5 s of CPU in 10.5 minutes | about 89 MiB (`docker stats`) | 2026-11-01, commit `15253e187`: the server built for macOS arm64 and run as a bare binary on the owner's workstation, against `pgvector/pgvector:pg16` in Docker, migrated to 0375; resident memory by `ps`, sampled every minute |

Not measured yet: the Compose stack's own containers (the Linux image, not a macOS binary), and any
installation under load. Until run F1 of the load plan ([test/load/PLAN.md](../../test/load/PLAN.md))
has measured them, no minimum or recommended host size is stated here; the idle figures above are a
floor, not a size to buy. `compose.yaml` carries a commented `mem_limit` and `GOMEMLIMIT` pair, the
example of [deployment.md](./deployment.md) §2, to be set once F1 gives the recommendation.

---

## 9. Sizes: what is bounded and what was measured

Two figures per size, never one for the other: the **ceiling** the code enforces (a write past it
is refused with a message code, nothing is cut short), and the **largest size a run measured**
with its result. A size no run reached is written *not measured*. The measured rows are the
owner's workstation (Apple M4 Max, 14 cores, 36 GB), not named hardware
([observability-reliability.md](./observability-reliability.md) §13.2), so they say what was
proven to work, not what to promise a customer.

| What | Ceiling (where) | Largest measured | Result |
|---|---|---|---|
| Entries per workspace | none (a plan or the operator may set one, [multi-tenancy.md](./multi-tenancy.md) §4) | 1 222 737 in one workspace, 10⁷ in the installation (second round, 2026-11-05) | every read path P95 under 200 ms with eight callers; the count over the hub (P95 119 ms) and the web app's board (47 ms) no longer keep pace at 400 a second |
| A device's first synchronisation | the snapshot's 512 MiB budget, then the paged walk ([offline-sync.md](./offline-sync.md) §3.1) | 1 225 170 records, 1.2 GB, 2 451 pages | 42 s, page P95 17 ms |
| Configuration items per hub | 20 000 (`MaxCIsPerHub`) | 20 000 (run C1, 2026-11-05) | no server error; health P95 248 ms and the service map 390 ms, over SLO-2; 796 MiB resident, over the run's 768 MiB `GOMEMLIMIT` - a known finding ([final-runs-leftovers.md](../backlog/suite/final-runs-leftovers.md), "Needs a finding issue") |
| A connector's run | 50 pages (`MaxPagesPerRun`), 2 000 items a delivery (`MaxPayloadItems`), 50 connectors a hub | not measured (S6 built, run at scale *not measured*) | its trial found and fixed duplicate CIs; the interval between two pages fell from 1 133 to 227 ms |
| A campaign's CIs | 5 000 (`MaxCampaignCIs`) | not measured | - |
| Objects on a whiteboard | 5 000 (`MaxElements`), 1 000 boards a hub (`MaxBoardsPerHub`) | 5 000 objects (`whiteboardframes.test.mjs`, Chromium) | opens under 3 s and keeps its frames while panned and zoomed |
| Pages per hub | 5 000 (`MaxPagesPerHub`), 12 levels deep, 1 MiB a page | not measured | - |
| A live document | 1 MiB an update, 16 MiB a snapshot | 30 writers on one document for two minutes (S5) | every document equal at the end, no edit unsent, no connection closed |
| Custom fields per entry | 100 (`MaxCustomFieldsPerItem`) | not measured | - |
| A bulk operation | 500 operations a request | 500 (`TestABulkOfFiveHundredAppliesEveryOperationButTheInvalidOne`) | every valid operation applied, the invalid one refused |
| An export or an aggregation | 5 000 rows (`MaxExportRows`, `MaxAggregateRows`); a spreadsheet 100 000 rows, 1 000 columns | not measured | - |
| A plan's schedule, a sprint, a baseline | 2 000 entries a schedule, 1 000 a sprint, 10 000 a baseline | not measured | - |
| A directory synchronisation (LDAP) | 50 000 entries (`MaxLdapEntries`) | not measured | - |
| People per workspace | none (a plan may set one) | not measured beyond the demo's | - |
| Live connections | 512 a workspace, 1 024 a process ([multi-tenancy.md](./multi-tenancy.md) §4) | 64 change streams while 10 writes a second land (S4) | none refused, none late or missing; delivery P95 302 ms |

**How a row changes.** A ceiling changes with its constant and the document that promises it
([domain-model.md](./domain-model.md)); a measured cell only with a run written up under
`docs/evidence/`, which the row then names. Raising a ceiling past its largest measured size
needs that run first.
