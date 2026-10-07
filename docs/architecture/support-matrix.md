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
| Gecko (Firefox) | current and previous major | `supported` | `engines` job: Playwright's Firefox, the same assertions |
| WebKit (Safari) | current and previous major | `supported` | `engines` job: Playwright's **WebKit build**, the same assertions. That is a Linux build of the engine Safari is made of, at roughly Safari's current version — not Safari's shell, settings or release cadence |
| Anything older | — | `unsupported` | — the client uses `<dialog>`, `inert` and `:has()`, and none of the three has a fallback |

**What `supported` stands on here.** The `engines` job ([ADR-0048](../adr/ADR-0048-browser-job-driver.md))
loads the bundle that ships, served the way the binary serves it, in the three engines, and asserts
in each every fact the client is built on — `<dialog>`, `inert`, `:has()`, `popover`, logical
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

The installable clients are **Tauri 2 shells** for Windows, macOS, Linux, iOS and Android, wrapping
the one web codebase ([ADR-0031](../adr/ADR-0031-tauri-app-shell.md)) — the one install path per
platform. The web app is a browser application only: no manifest-based install, no install prompt,
no service-worker install. The installed clients carry the offline promise; the browser holds a
best-effort copy ([offline-sync.md](./offline-sync.md) §1).

No shell exists yet, so no row claims one. A shell's row arrives with the job that starts it on its
platform, under §6's rule 1.
