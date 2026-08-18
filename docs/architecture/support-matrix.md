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
| PostgreSQL | ≤ 15 | `unsupported` | — the schema uses what 16 offers; nothing checks 15, so nothing may claim it |
| Go (building from source) | 1.25 | `supported` | `ci.yml:quick` and every other job |

## 4. `hubctl` (the CLI)

The rows arrive with B-13, when the client exists. It is the one artefact that runs **natively** on
a user's machine rather than in a container, so its matrix is about operating systems in a way the
server's is not.

| Platform | Status | Proven by |
|---|---|---|
| — | — | — arrives with B-13 |

---

## 5. Maintaining this table

1. A new row needs a job **in the same pull request**. The gate refuses the row otherwise, which is
   the point: a claim and its evidence land together or not at all.
2. Removing support is deleting the row *and* the job. Deleting only the job turns the build red —
   support does not lapse by neglect.
3. `best effort` and `unsupported` rows have no job and carry a dash plus the reason. The reason is
   what a bug reporter reads, so it says why rather than merely no.
4. A failing nightly matrix job files an issue automatically (`claude:task`), so a platform that
   broke does not stay broken until somebody happens to look.
