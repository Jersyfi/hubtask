# Support Matrix

What Hubtask is supported on, and — for every row — **the CI job that proves it**. The last column
is not documentation of the pipeline; it is the reason the row is allowed to exist. A gate
(`make gate-docs`) reconciles this table with the workflows in both directions: a row without a
job fails the build, and a matrix job without a row fails it too. Support can therefore neither be
claimed without evidence nor removed quietly — by anybody, including a pull request from outside.

* **Version:** 0.2.0 · **Decided:** 2026-08-18 · **Scope decision:** the server is a container,
  and only a container.
* **Concept:** [ADR-0014](../adr/ADR-0014-single-image-multi-role.md), [deployment.md](./deployment.md)

---

## 1. What "supported" means here

| Status | Meaning |
|---|---|
| `supported` | A CI job runs the software on it. A defect there is a release blocker. |
| `best effort` | Expected to work, not proven by a job. A defect there is an ordinary bug — reported, fixed when it can be, never a release blocker. |
| `unsupported` | Not intended. A defect there is closed with a pointer to this table. |

The distinction that shapes the whole table: **the server ships as a container** (ADR-0014). The
host operating system underneath is therefore almost irrelevant — a person on macOS or Windows runs
the same Linux container through Docker Desktop. What actually varies, and what is therefore what
gets tested, is the **container runtime**, the **CPU architecture**, and the **PostgreSQL major**.

---

## 2. The server

| Runtime | Architecture | Status | Proven by |
|---|---|---|---|
| Docker (Compose) | linux/amd64 | `supported` | `ci.yml:compose` |
| Docker (Compose) | linux/arm64 | `supported` | `nightly.yml:matrix-arm64` |
| Podman (Compose) | linux/amd64 | `supported` | `nightly.yml:matrix-podman` — the Podman engine, with the image built by `podman build` and the stack driven by Compose v2 against Podman's Docker-compatible socket, which is the path `podman compose` takes. **`podman-compose` 1.0.6, the Python reimplementation Debian and Ubuntu package, does not run this stack**: it starts each container with `podman start <name>`, and Podman refuses to resolve a dependency graph containing the migration container once that has exited, so the application container is created and never started. That is a limitation of that tool and not of the artefact — the half that *was* ours is fixed, and `ci.yml:compose` proves it on every pull request by starting the stack in the wrong order on purpose |
| Kubernetes ≥ 1.28 | linux/amd64 | `supported` | `nightly.yml:matrix-kind` |
| Kubernetes ≥ 1.28 | linux/arm64 | `best effort` | — the image is multi-arch and the chart is architecture-agnostic; no ARM cluster runs in CI |
| Docker Desktop | macOS, Windows | `best effort` | — the same Linux container; the runtime differences are Docker's, not ours |
| A bare binary (no container) | any | `unsupported` | — the image carries the migrator, the locales and the CA bundle; a loose binary is a support surface nobody has scoped |

## 3. The runtime environment

| Component | Version | Status | Proven by |
|---|---|---|---|
| PostgreSQL | 16 | `supported` | `ci.yml:integration` |
| PostgreSQL | 17 | `supported` | `nightly.yml:matrix-postgres` |
| PostgreSQL **with pgvector** (semantic search) | 16, 17 | `supported` | `ci.yml:integration`, which runs `pgvector/pgvector:pg16` since [ADR-0050](../adr/ADR-0050-pgvector-as-a-capability.md), and `nightly.yml:matrix-postgres`, which runs `pgvector/pgvector:pg17` — the 17 half of this row had no evidence at all until then, because that job composed a plain `postgres:17-alpine` and failed before it reached anything (#939). The extension is **detected, not demanded**: an installation without it migrates, runs, and searches lexically, and `/meta/capabilities` answers `semantic_search: false`. That path has its own test — `TestTheMigrationsApplyWithoutPgvector` starts a plain `postgres:16-alpine` and migrates it — because every other gate runs a database that has the extension |
| PostgreSQL **built with ICU** (names sort the same everywhere) | 16, 17 | `supported` | `ci.yml:integration` — `TestNamesSortUnderTheICURootCollation` proves migration `0080` copied `und-x-icu` into `hubtask_name`. Every image in this table has ICU, and so do the managed services above; a build without it is **not unsupported**: the migration falls back to the database's own locale, names still sort, only in that locale's order, and `/meta/capabilities` answers `natural_ordering: false`. The fallback branch is proved on the same database (`TestTheFallbackCollationIsTheDatabasesOwnLocale`), because no image in the matrix lacks ICU |
| PostgreSQL | ≤ 15 | `unsupported` | — the schema uses what 16 offers; nothing checks 15, so nothing may claim it. The hard floor is 15, which added `ON DELETE SET NULL (column)`; the tenant-scoped foreign keys need it ([ADR-0024](../adr/ADR-0024-tenant-scoped-foreign-keys.md)) |
| PostgreSQL without a superuser (a managed service) | 16, 17 | `supported` | `ci.yml:integration` — the migrations applied by an owner with `CREATEROLE` and no superuser, and the tenant boundary asserted afterwards ([ADR-0052](../adr/ADR-0052-managed-postgresql-support.md)). The operator creates the two roles and owns the database with `hubtask_migrator`; [multi-tenancy.md §2.1](./multi-tenancy.md) says how |
| Go (building from source) | 1.27 | `supported` | `ci.yml:quick` and every other job |

## 4. `hubctl` (the CLI)

It is the one artefact that runs **natively** on a user's machine rather than in a container, so
its matrix is about operating systems in a way the server's is not. What a job here proves is not
that the binary compiles — cross-compilation does that for every platform on every release — but
that the binary a platform produces *starts* on it.

| Platform | Architecture | Status | Proven by |
|---|---|---|---|
| Linux | amd64 | `supported` | `ci.yml:e2e` |
| Linux | arm64 | `supported` | `nightly.yml:matrix-hubctl` |
| macOS | arm64 (Apple silicon) | `supported` | `nightly.yml:matrix-hubctl` |
| Windows | amd64 | `supported` | `nightly.yml:matrix-hubctl` |
| macOS | amd64 (Intel) | `best effort` | — cross-compiled and published; GitHub's Intel macOS runners are on their way out, and a row may not rest on a runner that is being withdrawn |
| Windows | arm64 | `best effort` | — cross-compiled and published; no ARM Windows runner exists on the free tier this project builds on |

The Linux amd64 row points at `ci.yml:e2e` rather than at a nightly job on purpose: that job runs
the whole end-to-end session through the binary against the reference stack, which is a stronger
claim than a smoke test and it runs on every pull request. The rows are unchanged by J-16 and the
claim behind them grew: the session now also configures an AI provider, receives and accepts a
suggestion, searches both ways, and speaks MCP to `/mcp` from outside the process — the same job,
proving more.

---

## 5. The browser clients

The web application is embedded in the binary ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) and
the website is prerendered ([ADR-0030](../adr/ADR-0030-svelte-frontend-framework.md)); both run in
whatever browser the reader has. Which browsers that is, is
[ADR-0044](../adr/ADR-0044-browser-support-row.md).

| Engine | Versions | Status | Proven by |
|---|---|---|---|
| Chromium (Chrome, Edge) | current and previous major | `supported` | `engines` job in `ci.yml`: Playwright's Chromium at the pinned package version, the built bundle loaded and ADR-0044's feature table asserted (F6-02) |
| Gecko (Firefox) | current and previous major | `supported` | `engines` job: Playwright's Firefox, the same assertions |
| WebKit (Safari) | current and previous major | `supported` | `engines` job: Playwright's **WebKit build**, the same assertions. That is the engine and not Safari — a Linux build of WebKit, without Safari's shell, its settings or its release cadence — so what the job proves is that the client runs in the engine Safari is made of, at roughly Safari's current version |
| Anything older | — | `unsupported` | — the client uses `<dialog>`, `inert` and `:has()`, and none of the three has a fallback |

**What `supported` stands on here.** §1 defines it as "a CI job runs the software on it", and the
`engines` job is that job ([ADR-0048](../adr/ADR-0048-browser-job-driver.md), built in F6-02): the
bundle that ships, served the way the binary serves it, loaded in the three engines, with each fact
the client is built on asserted in each — not a journey. It gates through `CI required`, which is
what makes a job count (`ci-cd.md` §3.2). The versions the job runs are the ones Playwright's
pinned release bundles, which track the current major; the previous major is not run and stays on
the row on the strength of the features being years old in every engine.

What the client needs is small and unexotic — `<dialog>`, `inert`, `:has()`, `popover`, logical
properties, CSS Anchor Positioning — and the job asks each engine for each. The fallback
[ADR-0039](../adr/ADR-0039-overlay-positioning.md) kept for the last of them is gone with this
row: unreachable by any engine on it, and proven so by the job rather than assumed.

## 6. Maintaining this table

1. A new row needs a job **in the same pull request**. The gate refuses the row otherwise, which is
   the point: a claim and its evidence land together or not at all.
2. Removing support is deleting the row *and* the job. Deleting only the job turns the build red —
   support does not lapse by neglect.
3. `best effort` and `unsupported` rows have no job and carry a dash plus the reason. The reason is
   what a bug reporter reads, so it says why rather than merely no.
4. A failing nightly matrix job files an issue automatically (`claude:task`), so a platform that
   broke does not stay broken until somebody happens to look.
