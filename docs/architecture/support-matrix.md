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
| Podman (Compose) | linux/amd64 | `supported` | `nightly.yml:matrix-podman` |
| Kubernetes ≥ 1.28 | linux/amd64 | `supported` | `nightly.yml:matrix-kind` |
| Kubernetes ≥ 1.28 | linux/arm64 | `best effort` | — the image is multi-arch and the chart is architecture-agnostic; no ARM cluster runs in CI |
| Docker Desktop | macOS, Windows | `best effort` | — the same Linux container; the runtime differences are Docker's, not ours |
| A bare binary (no container) | any | `unsupported` | — the image carries the migrator, the locales and the CA bundle; a loose binary is a support surface nobody has scoped |

## 3. The runtime environment

| Component | Version | Status | Proven by |
|---|---|---|---|
| PostgreSQL | 16 | `supported` | `ci.yml:integration` |
| PostgreSQL | 17 | `supported` | `nightly.yml:matrix-postgres` |
| PostgreSQL | ≤ 15 | `unsupported` | — the schema uses what 16 offers; nothing checks 15, so nothing may claim it. The hard floor is 15, which added `ON DELETE SET NULL (column)`; the tenant-scoped foreign keys need it ([ADR-0024](../adr/ADR-0024-tenant-scoped-foreign-keys.md)) |
| Go (building from source) | 1.26 | `supported` | `ci.yml:quick` and every other job |

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
claim than a smoke test and it runs on every pull request.

---

## 5. The browser clients

The web application is embedded in the binary ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) and
the website is prerendered ([ADR-0030](../adr/ADR-0030-svelte-frontend-framework.md)); both run in
whatever browser the reader has. Which browsers that is, is
[ADR-0044](../adr/ADR-0044-browser-support-row.md).

| Engine | Versions | Status | Proven by |
|---|---|---|---|
| Chromium (Chrome, Edge) | current and previous major | `best effort` | — no browser job runs in CI |
| Gecko (Firefox) | current and previous major | `best effort` | — no browser job runs in CI |
| WebKit (Safari) | current and previous major | `best effort` | — no browser job runs in CI |
| Anything older | — | `unsupported` | — the client uses `<dialog>`, `inert` and `:has()`, and none of the three has a fallback |

**Why `best effort` and not `supported`, when the scope is decided.** §1 defines `supported` as "a
CI job runs the software on it", and there is no browser job of any kind — the client's gates are
`build`, `lint`, `typecheck` and `node --test`. The row above says where a defect will be **fixed**;
it does not claim anyone has **looked**. Turning it into `supported` needs a browser job, which is a
separate decision ADR-0044 names and leaves open.

What the client needs is small and unexotic — `<dialog>`, `inert`, `:has()`, `popover`, logical
properties — and every engine in the row has all of it. That is what makes A affordable: nothing had
to be built to reach it. One feature, CSS Anchor Positioning, carries a fallback this project owns
([ADR-0039](../adr/ADR-0039-overlay-positioning.md)); under this row it is no longer reachable by a
supported engine, and removing it waits for the browser job rather than happening on its own.

## 6. Maintaining this table

1. A new row needs a job **in the same pull request**. The gate refuses the row otherwise, which is
   the point: a claim and its evidence land together or not at all.
2. Removing support is deleting the row *and* the job. Deleting only the job turns the build red —
   support does not lapse by neglect.
3. `best effort` and `unsupported` rows have no job and carry a dash plus the reason. The reason is
   what a bug reporter reads, so it says why rather than merely no.
4. A failing nightly matrix job files an issue automatically (`claude:task`), so a platform that
   broke does not stay broken until somebody happens to look.
