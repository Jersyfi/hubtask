# Versioning & Release

Binding: **Semantic Versioning 2.0.0** (`MAJOR.MINOR.PATCH`).

---

## 1. What exactly gets versioned

| Artefact | Version | Rule |
|---|---|---|
| Product / git tag | `vX.Y.Z` | The single leading version, derived from the Conventional Commits |
| Container image | `hubtask/server:X.Y.Z`, plus `:X.Y`, `:X`, `:latest` (stable only), `:sha-<commit>` | The digest is recorded in the release |
| Helm chart | `version` (chart) independent, `appVersion` = product version | A chart change without a code change bumps only `version` |
| REST API | Path major `/api/v1`; `info.version` = product version | A breaking change ⇒ a new path major *and* a new product major |
| Event types | Suffix `.v1` per event | Additive fields need no new version |
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
