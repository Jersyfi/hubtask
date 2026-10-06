# Versioning & Release

Binding: **Semantic Versioning 2.0.0** (`MAJOR.MINOR.PATCH`). There is one product version for the
server and every first-party client ([ADR-0035](../adr/ADR-0035-one-product-version.md)).

---

## 1. What exactly gets versioned

| Artefact | Version | Rule |
|---|---|---|
| Product / git tag | `vX.Y.Z` | The single leading version. The maintainer chooses it from the changes since the last tag and creates the tag (§3) |
| Container image | `ghcr.io/jersyfi/hubtask:vX.Y.Z`, plus `:latest` for a stable version or `:rc` for a pre-release | Signed by digest; the digest is in the release. `latest` never points at a release candidate. The integration environment runs per-commit images (`:main-<sha>`), which are not releases |
| Helm chart | `version` = `appVersion` = product version | Both are set from the tag when the chart is packaged; a chart change is released with a product version. Published to `oci://ghcr.io/jersyfi/charts/hubtask` |
| Web app bundle | none of its own | Built from the same commit and embedded in the binary ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)). It is not released on its own, so it is not versioned on its own |
| Tauri shells | `appVersion` = product version, plus a platform build counter per build | The stores require a monotonic counter (`CFBundleVersion`, Android `versionCode`) that SemVer does not provide. A store-only fix — signing, metadata, a rejected listing — bumps the counter, never the product version. No shell exists yet ([support-matrix.md](./support-matrix.md) §7) |
| Website (`hubtask.eu`) | unversioned | Continuously deployed and identified by its commit; it carries no compatibility surface |
| Workspace packages (`apps/*`, `packages/*`, `sdk/typescript`) and the Python SDK | `private: true`; the `version` field is not a product statement | Nothing is published to a registry, and the release does not read them |
| REST API | Path major `/api/v1`; `info.version` = product version | A breaking change ⇒ a new path major *and* a new product major |
| Event types | Suffix `.v1` per event | Additive fields need no new version |
| Backup archive format | `format_version` in `manifest.json`, independent of the product version | It changes only when a reader that did not know about the change would misread the file. Every major adds a golden archive for the format it writes (§7); a reader refuses a version it does not know rather than importing part of it ([backup-restore.md](./backup-restore.md) §3) |
| MCP tools | The tool name is stable | Parameters may only be added |
| Go module | `module …/hubtask` (v0/v1); from v2 onwards the path suffix `/v2` | Relevant only if the module is used as a library |
| DB migrations | Sequential numbers | Forward only; an applied migration is never changed (§4) |
| Translations | Follow the release | A missing translation never blocks a release |

**Before 1.0.0:** `0.MINOR.PATCH`; a minor may contain breaking changes, and the release notes mark
them `BREAKING CHANGE`. With **1.0.0**, API v1 is promised stable.

---

## 2. What counts as a break

| Change | Classification |
|---|---|
| Removing or renaming a field, endpoint, or error code | MAJOR |
| Adding a required field, changing a type, semantics, or default | MAJOR |
| Removing an enum value from **requests** | MAJOR |
| Tightening a permission check | MAJOR (documented) |
| A migration that breaks older application versions | MAJOR |
| Removing a configuration variable | MAJOR |
| A new endpoint, a new optional field, a new enum value in **responses** | MINOR |
| A new `ItemType`, a new automation action, a new trigger | MINOR |
| A new language, a new adapter | MINOR |
| A bug fix without a contract change, performance, documentation | PATCH |
| A security fix without a contract change | PATCH (plus a security advisory) |

Clients ignore unknown fields. That is part of the contract, and it is the precondition for
additive changes staying MINOR.

---

## 3. Commits, branching, release

**Conventional Commits** for every commit and for the squash-merge title, in English:

```
feat(work): allow moving a work package between tasks
fix(scheduling): keep local time across DST transitions
feat(api)!: remove deprecated /tasks endpoint

BREAKING CHANGE: /tasks has been replaced by /items
```

Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `revert`. The
scope names the area (`work`, `identity`, `automation`, `api`, `webapp`, `architecture`, …). It is
a convention; no CI job lints it.

**Branching:** trunk-based. `main` is always releasable. Work happens on short-lived branches,
reaches `main` through a pull request, and is squash-merged with a Conventional Commit title. For
a supported older major, a `release/N.x` branch receives cherry-picked security and critical fixes.
There is no feature-flag mechanism: work lands on `main` in steps that each build and pass the
gates, and an optional surface is announced under `features` in `/meta/capabilities`
([api-guidelines.md](./api-guidelines.md) §1).

**Release:**

1. `make release-tag VERSION_TO_TAG=X.Y.Z` runs `make verify` on a clean tree, creates the signed
   tag `vX.Y.Z` and pushes it.
2. `release.yml` waits for approval of the GitHub environment `production`, runs `make verify`
   again on the tag, builds and pushes the multi-arch image, produces the CycloneDX SBOM, signs the
   image keylessly, attests provenance, regenerates the third-party licence list, packages and
   pushes the chart, and creates the GitHub release with generated notes, the SBOM, the chart and
   `THIRD-PARTY-LICENSES.md` ([deployment.md](./deployment.md) §7).

The release notes are the changelog; there is no `CHANGELOG.md` file. There is no version field in
the code: the version is burned in through `-ldflags` and reported by `/meta/health`.

---

## 4. Database migrations (forward only, expand / contract)

**Migrations are forward only.** An applied migration is never edited. Recovery is a restore, never
a down migration: a migration's `goose Down` section, where it has one, raises an exception
instead of undoing anything.

Rolling updates run the old and the new version at the same time, so every schema change follows
expand/contract:

1. **Expand** (release *n*): a new column or table, additive and nullable, backfilled as a
   background job; the code writes both old **and** new.
2. **Migrate** (release *n*): the code reads new, falling back to old.
3. **Contract** (release *n+1* or later): remove the old column once every instance runs the new
   code.

Rules:

* No blocking locks: `CREATE INDEX CONCURRENTLY`, `NOT VALID` constraints with a later `VALIDATE`.
* No destructive step in the same release as the code change that stops using the object.
* The schema stays backwards compatible with the previous minor, which is what makes a rollback
  possible ([deployment.md](./deployment.md) §5).
* A migration assumes nothing but the schema it names. Numbers are taken when a branch is cut and
  branches merge in the order review finishes them, so a lower number can arrive after a higher one
  is applied; the migrator applies it late (`goose` with missing migrations allowed). A number
  already taken on `main` or on another open branch is not reused.
* The migration runs as the database owner; the application connects as `hubtask_app`, without
  `SUPERUSER` or `BYPASSRLS` ([multi-tenancy.md](./multi-tenancy.md) §2.1).

---

## 5. Support and upgrade policy

None of this is a commitment. Hubtask is open source under Apache-2.0 and provided as is
([licensing-editions.md](./licensing-editions.md)): fixes are best effort and go into the current
minor, no version has a support period, and the project may be archived at any time. The rows
below are the policy the project *aims* to follow.

| Point | Aim |
|---|---|
| Supported versions | The current major plus the previous major for its declared support period (12 months intended) |
| Upgrade path | Any version can be upgraded to from any version of the previous major; a jump across two majors needs an intermediate step |
| Downgrade | Not supported (restore from backup) |
| Deprecation | At least two minor releases of notice: in the release notes, as `Deprecation`/`Sunset` headers, and in the capability manifest |
| Security updates | A patch on every supported major; an advisory with CVSS |
| Pre-releases | `X.Y.Z-rc.N` with the image tag `:rc`, marked as a pre-release on GitHub; never for production |
| Client maturity | `experimental` → `preview` → `stable`, stated per release in the release notes and by the application itself until it is `stable` ([design-system.md](../design/design-system.md)). It is a statement about a release, not a runtime capability, so it does not appear in `/meta/capabilities` ([ADR-0035](../adr/ADR-0035-one-product-version.md)) |

---

## 6. Quality gates in the release (CI gates)

A tag is cut from a commit on `main`, and every commit on `main` passed the whole pull request
pipeline with no filter ([ci-cd.md](./ci-cd.md) §3); `release.yml` runs `make verify` again on the
tag. The gates, and where each runs:

| Gate | Condition | Where |
|---|---|---|
| Unit/domain tests | Green; coverage `core/domain` ≥ 85 %, `core/application` ≥ 75 % | `unit` |
| Integration tests | Green against a real PostgreSQL | `integration` |
| Contract tests | Responses validate against `openapi.yaml`, events against their JSON schemas, the router against the specification | `integration` (`make gate-contract`) |
| Compatibility check | OpenAPI diff against the last tag; a breaking change without `!`/`BREAKING CHANGE` fails | **Not built** ([ci-cd.md](./ci-cd.md) §8) |
| Architecture tests | Layer and import rules | `architecture`, `quick` (`depguard`) |
| Use case parity | Every use case is registered in REST, MCP and automation | `architecture` |
| i18n check | Placeholders consistent, no unknown keys | `architecture` |
| Static analysis | `golangci-lint`, `go vet`, `govulncheck` | `quick`, `security` |
| Security | SG-1…SG-12 ([security.md](./security.md) §13); fuzzing and the image scan nightly; SBOM and signature per release | `security`, `secrets`, `nightly.yml`, `release.yml` |
| Reliability | RT-1…RT-5, RT-7, RT-10, RT-12 per pull request; RT-6, RT-8, RT-11 nightly; RT-9 (restore drill) per release ([observability-reliability.md](./observability-reliability.md) §12) | `resilience`, `nightly.yml`, the chart's drill |
| Observability completeness | The use case registry reconciled against metrics and spans (RT-12) | `architecture` |
| Audit completeness | Every action marked `auditable` produces exactly one entry (SG-13); grants on `audit_log` (AT-1); no user content in the audit (AT-4) | `architecture`, `data` |
| Data protection | PG-1, PG-3…PG-6, PG-8 (`make gate-privacy`); PG-2 and PG-7 against a database (`make gate-privacy-full`) ([data-protection.md](./data-protection.md) §10) | `security`, `data` |
| Backup | A round trip per target adapter (BK-1); the golden archives of every supported major import (BK-4); a restore triggers no automation (BK-5) | `data` |
| Retention | RE-1…RE-9, in particular the safeguards (legal hold, restriction, tombstone window) | `data` |
| Synchronisation | SY-1…SY-12; `hubctl sync-conformance` and the engine's conformance run against the reference stack | `data`, `e2e`, `engine-session` |
| Freedom from panics | No `hubtask_panics_recovered_total > 0`; no `go` statement outside `core/shared/concurrency` | `resilience`, `architecture` |
| Migration check | The migrations apply to an existing database; a rolling update with the N−1/N schema under load (RT-8) | `integration`, `nightly.yml` |
| Generated code | `make generate` produces no diff | `quick` |

`make gate-selftest` proves that each configured rule turns the build red when it is broken.

---

## 7. How a major is finished

A major is finished in three movements, in this order, for `1.0.0` and every major after it
([ADR-0035](../adr/ADR-0035-one-product-version.md) §5):

1. **Parallel development.** Tracks run alongside each other and may lag each other. The client
   track runs one milestone window behind the core, so that it builds against contracts that have
   settled. An incomplete client is the normal state; a client that does not build is a defect.
2. **Convergence.** A milestone of its own: the clients meet the capability matrix
   ([ADR-0032](../adr/ADR-0032-client-capability-matrix.md)), the maturity stage goes to `stable`,
   the **scope window closes**, and everything with external lead time — store review above all —
   is set in motion. New requirements are accepted up to the day it opens; after it, defects only.
   Anything else waits for the next minor or is an exception with its own ADR.
3. **Stabilisation.** The major's prerequisites are demonstrated and only defects are fixed. One
   prerequisite is the **golden archive**: before the major is tagged, an archive written by it is
   committed under `test/backup/golden/v<format-version>/`, and BK-4 imports every directory there.
   A major that changed nothing about the archive format adds nothing. None is ever removed.
   Regenerating one is a deliberate act at a release (`HUBTASK_WRITE_GOLDEN=1`), never something a
   test does when it disagrees.

**A major is released when the server, the clients and the website are finished together, from one
commit.** No client ships later under the same version.

The milestones of the current major are in [roadmap.md](../roadmap.md).
