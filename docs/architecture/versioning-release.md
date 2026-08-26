# Versioning & Release

Binding: **Semantic Versioning 2.0.0** (`MAJOR.MINOR.PATCH`).

---

## 1. What exactly gets versioned

| Artefact | Version | Rule |
|---|---|---|
| Product / git tag | `vX.Y.Z` | The single leading version, derived from the Conventional Commits |
| Container image | `hubtask/server:X.Y.Z`, plus `:X.Y`, `:X`, `:latest` (stable only), `:sha-<commit>` | The digest is recorded in the release |
| Helm chart | `version` (chart) independent, `appVersion` = product version | A chart change without a code change bumps only `version` |
| Web app bundle | none of its own | Built from the same commit and embedded in the binary ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)). It is not released on its own, so it is not versioned on its own ([ADR-0035](../adr/ADR-0035-one-product-version.md)) |
| Tauri shells | `appVersion` = product version, plus a platform build counter per build | The stores require a monotonic counter (`CFBundleVersion`, Android `versionCode`) that SemVer does not provide. A store-only fix — signing, metadata, a rejected listing — bumps the counter, never the product version |
| Website (`hubtask.eu`) | unversioned | Continuously deployed and identified by its commit; it carries no compatibility surface, so a version would state nothing |
| Workspace packages (`apps/*`, `packages/*`) | `private: true`; the `version` field is not a product statement | Nothing is published to a registry, and the release automation does not read them |
| REST API | Path major `/api/v1`; `info.version` = product version | A breaking change ⇒ a new path major *and* a new product major |
| Event types | Suffix `.v1` per event | Additive fields need no new version |
| Backup archive format | `format_version` in `manifest.json`, independent of the product version | It changes only when a reader that did not know about the change would misread the file. Every major adds a golden archive for the format it writes (§7); a reader refuses a version it does not know rather than importing part of it ([backup-restore.md](./backup-restore.md) §3) |
| MCP tools | The tool name is stable | Parameters may only be added |
| Go module | `module …/hubtask` (v0/v1); from v2 onwards the path suffix `/v2` | Relevant only if the module is used as a library |
| DB migrations | Sequential numbers | Never changed retroactively |
| Translations | Follow the release | Missing translations never block a release |

**Before 1.0.0:** `0.MINOR.PATCH`; minor bumps may contain breaking changes, but these are marked
`BREAKING CHANGE` in the changelog. With **1.0.0**, API v1 is promised stable.

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

Clients ignore unknown fields — that is part of the contract, and it is the precondition for
additive changes staying MINOR.

---

## 3. Commits, branching, automation

**Conventional Commits** (enforced by a CI lint):

```
feat(work): allow moving a work package between tasks
fix(scheduling): keep local time across DST transitions
feat(api)!: remove deprecated /tasks endpoint

BREAKING CHANGE: /tasks has been replaced by /items
```

Permitted types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`,
`revert`. Scopes correspond to the bounded contexts (`work`, `scheduling`, `automation`,
`identity`, `api`, `infra`, `i18n`, `mcp`, `deploy`).

**Branching:** trunk-based. `main` is always releasable; short-lived feature branches with a merge
request; squash merge with a Conventional Commit title. For supported older majors, `release/1.x`
branches with cherry-picks for security and critical fixes.

**Release automation:** a CI job computes the next version, updates `CHANGELOG.md`
(Keep a Changelog format), sets the tag, builds and signs the image and Helm chart, produces an
SBOM (CycloneDX), and publishes release notes. No manual version field in the code — the version is
burned in via `-ldflags` and reported through `/meta/health`.

**Feature flags** instead of long-lived branches: unfinished features are disabled behind
`HUBTASK_FEATURE_*` and become the default only when they are finished.

---

## 4. Database migrations (expand / contract)

Because rolling updates run old and new pods at the same time, every change follows:

1. **Expand** (release *n*): a new column or table, additive, nullable, backfill as a background
   job; the code writes both old **and** new.
2. **Migrate** (release *n*): the code reads new, falling back to old.
3. **Contract** (release *n+1* or later): remove the old column once every instance has been
   updated.

Rules: no blocking locks (`CREATE INDEX CONCURRENTLY`, `NOT VALID` constraints with a later
`VALIDATE`), no destructive steps in the same release as the code change, every migration tested
against a production-like data volume, and down migrations only for development environments (in
production the rule is: fix forwards).

---

## 5. Support and upgrade policy

| Point | Commitment |
|---|---|
| Supported versions | The current major plus the previous major for 12 months |
| Upgrade path | Any version can be upgraded to from any version of the previous major; jumps across two majors need an intermediate step |
| Downgrade | Not supported (restore from backup) |
| Deprecation | At least two minor releases of notice, announced in the changelog, `Deprecation`/`Sunset` headers, an entry in the capability manifest |
| Security updates | A patch on every supported major; an advisory with CVSS |
| Pre-releases | `X.Y.Z-rc.N` with the image tag `:rc`; no production commitment |
| Client maturity | `experimental` → `preview` → `stable`, stated per release in `CHANGELOG.md` and by the application itself until it is `stable`. It is a statement about a release, not a runtime capability, so it does not appear in `/meta/capabilities` ([ADR-0035](../adr/ADR-0035-one-product-version.md)) |

---

## 6. Quality gates in the release (CI gates)

| Gate | Condition |
|---|---|
| Unit/domain tests | Green, coverage `core/domain` ≥ 85%, `core/application` ≥ 75% |
| Integration tests | Green against a real PostgreSQL (Testcontainers) |
| Contract tests | Responses validate against `openapi.yaml`; events against JSON schemas |
| Compatibility check | OpenAPI diff against the last tag; a breaking change without `!`/`BREAKING CHANGE` fails the build |
| Architecture tests | Layer and import rules observed |
| Use case parity | Every use case is registered in REST, MCP, and automation |
| i18n check | Placeholders consistent, no unknown keys |
| Static analysis | `golangci-lint`, `go vet`, `govulncheck` |
| Security | The full gates SG-1…SG-12 from [security.md](./security.md) §13: `govulncheck`, `gosec`, cross-tenant suite, RLS/BYPASSRLS test, authorisation architecture test, SSRF suite, secret scan, fuzzing, container scan, SBOM + signature, auth negative tests, upload matrix |
| Reliability | Resilience tests RT-1…RT-5, RT-7, RT-10, RT-12 on the PR; RT-6, RT-8, RT-11 nightly; RT-9 (restore drill) per release ([observability-reliability.md](./observability-reliability.md) §12) |
| Observability completeness | Reconciliation of the use case registry against metrics/spans (RT-12); a new feature without signals fails the build |
| Audit completeness | Every action marked `auditable` produces exactly one entry (AU-1); grants on `audit_log` checked (AU-2); no user content in the audit (AU-4) |
| Data protection | Deletion tests per storage location from the data catalogue, leaving no orphans; metric and log output free of personal free text |
| Backup | A round trip per target adapter (BK-1) and an import of the golden archives of every supported major version (BK-4); a restore triggers no automation (BK-5) |
| Retention | RE-1…RE-9 green, in particular the safeguards (legal hold, restriction, tombstone window) |
| Synchronisation | SY-1…SY-12 green; the conformance test `hubctl sync-conformance` against the reference instance |
| Freedom from panics | Test runs with no `hubtask_panics_recovered_total > 0`; no `go` statement outside `core/shared/concurrency` |
| Migration check | Migration against the previous state plus a rolling update simulation |
| Generated code | `make generate` produces no diff |

---

## 7. How a major is finished

A major is not finished when its last feature merges. It is finished in three movements, and every
major — `1.0.0`, `2.0.0`, and each one after — runs them in this order
([ADR-0035](../adr/ADR-0035-one-product-version.md) §5):

1. **Parallel development.** Tracks run alongside each other and may lag each other. The client
   track runs one milestone window behind the core, so that it builds against contracts that have
   settled rather than against ones still moving. An incomplete client is the normal state here; a
   client that does not build is a defect like any other.
2. **Convergence.** A milestone of its own, whose purpose is that the tracks arrive at the same
   place: the clients meet the capability matrix
   ([ADR-0032](../adr/ADR-0032-client-capability-matrix.md)), the maturity stage goes to `stable`,
   the **scope window closes**, and everything with external lead time — store review above all —
   is set in motion. New requirements are accepted up to the day it opens. After it there are
   defects only; anything else waits for the next minor or is an exception with its own ADR.
3. **Stabilisation.** The major's prerequisites are demonstrated and only defects are fixed. One
   of those prerequisites is the **golden archive**: before the major is tagged, an archive written
   by it is committed under `test/backup/golden/v<format-version>/`, and BK-4 imports every
   directory that is there. A major that changed nothing about the archive format adds nothing —
   a second byte-identical copy proves nothing, and the directory it would go in already exists.
   None is ever removed: the oldest one is the only evidence that an archive from years ago still
   opens, and it becomes worth more, not less, with every release. Regenerating one is a deliberate
   act at a release (`HUBTASK_WRITE_GOLDEN=1`), never something a test does when it happens to
   disagree — an archive that rewrites itself when the reader changes proves nothing at all.

**A major is released when the server, the clients and the website are finished together, from one
commit.** There is no arrangement in which the product ships and a client follows later, because
with one version there is no number in which that could be said.

The milestones of the current major are in [roadmap.md](../roadmap.md); this section is the shape
they instantiate, and it outlives them.
