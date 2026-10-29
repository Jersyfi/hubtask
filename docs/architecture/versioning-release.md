# Versioning & Release

Binding: **Semantic Versioning 2.0.0** (`MAJOR.MINOR.PATCH`). There is one product version for the
server and every first-party client ([ADR-0035](../adr/ADR-0035-one-product-version.md)).

---

## 1. What exactly gets versioned

| Artefact | Version | Rule |
|---|---|---|
| Product / git tag | `vX.Y.Z` | The single leading version, chosen by the maintainer (§3) |
| Container image | `ghcr.io/jersyfi/hubtask:vX.Y.Z`, plus `:latest` for a stable version or `:rc` for a pre-release | Signed by digest. `latest` never points at a release candidate; the per-commit `:main-<sha>` images are not releases |
| Helm chart | `version` = `appVersion` = product version | Set from the tag; a chart change is released with a product version. `oci://ghcr.io/jersyfi/charts/hubtask` |
| Web app bundle | none of its own | Built from the same commit and embedded in the binary ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) |
| Tauri shells | `appVersion` = product version, plus a platform build counter per build: `X.Y.Z+N`, and Android's `versionCode` = `N` | `N` is `release.yml`'s run number, which only grows; build metadata orders nothing, so an app never takes its own release for a newer one. The stores require a monotonic counter (`CFBundleVersion`, `versionCode`); a store-only fix bumps the counter, never the product version ([support-matrix.md](./support-matrix.md) §7). Published as `hubtask-X.Y.Z-windows-x64-setup.exe`, `-linux-x86_64.AppImage`, `-linux-amd64.deb` and `-android-universal.apk`, each with its update signature (`.sig`), beside `latest.json` and one `SHA256SUMS` over every file of the release (§9) |
| Website (`hubtask.eu`) | unversioned | Continuously deployed; no compatibility surface |
| Workspace packages (`apps/*`, `packages/*`, `sdk/typescript`) and the Python SDK | `private: true` | Nothing is published to a registry; the `version` field is not a product statement |
| REST API | Path major `/api/v1`; `info.version` = product version | A breaking change ⇒ a new path major *and* a new product major |
| Event types | Suffix `.v1` per event | Additive fields need no new version |
| Backup archive format | `format_version` in `manifest.json`, independent of the product version | Changes only when an unaware reader would misread the file. Every major adds a golden archive (§7); a reader refuses an unknown version rather than importing part of it ([backup-restore.md](./backup-restore.md) §3) |
| MCP tools | The tool name is stable | Parameters may only be added |
| Go module | `module …/hubtask` (v0/v1); from v2 the path suffix `/v2` | Relevant only if the module is used as a library |
| DB migrations | Sequential numbers | Forward only; an applied migration is never changed (§4) |
| Translations | Follow the release | A missing translation never blocks a release |

**Before 1.0.0:** `0.MINOR.PATCH`; a minor may contain breaking changes, marked `BREAKING CHANGE`
in the release notes. With **1.0.0**, API v1 is promised stable.

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

Clients ignore unknown fields. That is part of the contract, and the precondition for additive
changes staying MINOR.

---

## 3. Commits, branching, release

**Conventional Commits** for every commit and for the squash-merge title, in English:

```
feat(api)!: remove deprecated /tasks endpoint

BREAKING CHANGE: /tasks has been replaced by /items
```

Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `revert`; the
scope names the area. The pull request title, which becomes the squash commit's, is checked by
`make gate-pr` in the `pr-description` job ([ci-cd.md](./ci-cd.md) §3.1) for pull requests opened
from 2026-10-08 on; the commits on a branch follow it by convention, and no job reads them.

**Branching:** trunk-based; `main` is always releasable. Work reaches `main` through short-lived
branches, squash-merged. A supported older major gets a `release/N.x` branch for cherry-picked
security and critical fixes. There are no feature flags: work lands in steps that each build and
pass the gates, and an optional surface is announced under `features` in `/meta/capabilities`
([api-guidelines.md](./api-guidelines.md) §1).

**Release:** `make release-tag VERSION_TO_TAG=X.Y.Z` runs `make verify` on a clean tree, creates
the signed tag `vX.Y.Z` and pushes it; `release.yml` builds the installed apps, and does the rest
after the `production` environment's approval ([deployment.md](./deployment.md) §7): the apps are
signed with the project's two keys there (§9).

The release notes are the changelog; there is no `CHANGELOG.md`. There is no version field in the
code: the version is burned in through `-ldflags` and reported by `/meta/health`.

---

## 4. Database migrations (forward only, expand / contract)

**Migrations are forward only.** An applied migration is never edited. Recovery is a restore: a
`goose Down` section, where one exists, raises an exception.

Rolling updates run the old and the new version at once, so every schema change follows
expand/contract:

1. **Expand** (release *n*): a new column or table, additive and nullable, backfilled as a
   background job; the code writes both old **and** new.
2. **Migrate** (release *n*): the code reads new, falling back to old.
3. **Contract** (release *n+1* or later): remove the old column once every instance runs the new
   code.

Rules:

* No blocking locks: `CREATE INDEX CONCURRENTLY`, `NOT VALID` constraints with a later `VALIDATE`.
* No destructive step in the same release as the code change that stops using the object.
* The schema stays backwards compatible with the previous minor, which makes a rollback possible
  ([deployment.md](./deployment.md) §5).
* A migration assumes nothing but the schema it names: a lower number can merge after a higher one
  is applied, and the migrator applies it late (`goose` with missing migrations allowed). A number
  already taken on `main` or on another open branch is not reused.
* The migration runs as the database owner; the application connects as `hubtask_app`, without
  `SUPERUSER` or `BYPASSRLS` ([multi-tenancy.md](./multi-tenancy.md) §2.1).

---

## 5. Support and upgrade policy

None of this is a commitment: Hubtask is provided as is ([licensing-editions.md](./licensing-editions.md)),
fixes are best effort and go into the current minor, no version has a support period, and the
project may be archived at any time. The rows are what the project *aims* for.

| Point | Aim |
|---|---|
| Supported versions | The current major plus the previous major for its declared support period (12 months intended) |
| Upgrade path | From any version of the previous major; across two majors with an intermediate step |
| Downgrade | Not supported (restore from backup) |
| Deprecation | At least two minor releases of notice: release notes, `Deprecation`/`Sunset` headers, the capability manifest |
| Security updates | A patch on every supported major; an advisory with CVSS |
| Pre-releases | `X.Y.Z-rc.N` with the image tag `:rc`, marked as a pre-release on GitHub; never for production |
| Client maturity | `experimental` → `preview` → `stable`, stated per release in the release notes and by the application until `stable` ([design-system.md](../design/design-system.md)); not a runtime capability, so not in `/meta/capabilities` |

---

## 6. Quality gates in the release (CI gates)

A tag is cut from a commit on `main`, every commit on `main` passed the whole pull request pipeline
unfiltered, and `release.yml` runs `make verify` again on the tag. The jobs and what each runs are
[ci-cd.md](./ci-cd.md) §3; a release requires:

| Gate | Condition | Where |
|---|---|---|
| Tests | Unit green with coverage `core/domain` ≥ 85 %, `core/application` ≥ 75 %; integration green against a real PostgreSQL; contract tests (`make gate-contract`) | `unit`, `integration` |
| Compatibility check | OpenAPI diff against the last tag; a breaking change without `!`/`BREAKING CHANGE` fails ([ci-cd.md](./ci-cd.md) §3) | `api-compat` |
| Structure | Layer and import rules, use case parity across REST, MCP and automation, the i18n check, RT-12, SG-13, no `go` statement outside `core/shared/concurrency`; `make generate` without a diff; `golangci-lint`, `go vet` | `architecture`, `quick` |
| Security | SG-1…SG-12 ([security.md](./security.md) §13), `govulncheck`; fuzzing and the image scan nightly; SBOM and signature per release | `security`, `secrets`, `nightly.yml`, `release.yml` |
| Reliability | RT-1…RT-5, RT-7, RT-10, RT-12 per pull request; RT-6, RT-8, RT-11 nightly; RT-9 (restore drill) per release ([observability-reliability.md](./observability-reliability.md) §12); no `hubtask_panics_recovered_total > 0` | `resilience`, `nightly.yml`, the chart's drill |
| Data guarantees | AT-1, AT-4; PG-1…PG-8 ([data-protection.md](./data-protection.md) §10); BK-1, BK-4 (the golden archives of every supported major import), BK-5; RE-1…RE-9; SY-1…SY-12 with `hubctl sync-conformance` and the engine's conformance run | `security`, `data`, `e2e`, `engine-session` |
| Migration check | The migrations apply to an existing database; a rolling update with the N−1/N schema under load (RT-8) | `integration`, `nightly.yml` |

`make gate-selftest` proves that each configured rule turns the build red when it is broken.

---

## 7. How a major is finished

Three movements, in this order, for `1.0.0` and every major after it
([ADR-0035](../adr/ADR-0035-one-product-version.md) §5):

1. **Parallel development.** The client track runs one milestone window behind the core, against
   settled contracts. An incomplete client is normal; a client that does not build is a defect.
2. **Convergence.** A milestone of its own: the clients meet the capability matrix
   ([ADR-0032](../adr/ADR-0032-client-capability-matrix.md)), the maturity stage goes to `stable`,
   the **scope window closes** (after it, defects only, unless an exception has its own ADR), and
   everything with external lead time — store review above all — is set in motion.
3. **Stabilisation.** The major's prerequisites are demonstrated; only defects are fixed. Before
   the major is tagged, a **golden archive** written by it is committed under
   `test/backup/golden/v<format-version>/` (none if the format did not change; none is ever
   removed), and BK-4 imports every directory there. Regenerating one is a deliberate act at a
   release (`HUBTASK_WRITE_GOLDEN=1`), never what a test does when it disagrees.

**A major is released when the server, the clients and the website are finished together, from one
commit.** No client ships later under the same version.

The milestones of the current major are in [roadmap.md](../roadmap.md).

---

There is no §8: ADR-0025 cites one for what §2 says ([Errata](../adr/README.md#errata)).

## 9. The installed apps' keys

Two keys sign what a release offers of the installed apps (F7 Decision 10,
[ADR-0084](../adr/ADR-0084-the-first-majors-shells-cost-nothing-to-ship.md)): the **APK signing
key**, which Android checks before it installs an update over an installed app, and the **update
key**, whose signatures the desktop updater checks
([ADR-0097](../adr/ADR-0097-android-stays-current-by-a-notice.md)). The Android app's notice reads
the same manifest without checking it: it installs nothing, opens the release's own page rather
than an address from the manifest, and Android checks the APK's key itself. Both are the project's; the
owner creates each once, on a machine of their own, and nobody else holds them.

| Key | Recorded |
|---|---|
| APK signing certificate, SHA-256 | `pending` |
| Update public key | `pending` |
| Update key ID | `pending` |

**How the desktop updater reads a release.** Tauri's updater checks a package's signature, not
the manifest's, so its endpoint is `update.json`: the newest release's manifest (`latest.json`) as
the one download, with `latest.json.sig` as its signature, in the updater's single-download
format. The app (`apps/webapp/src-tauri/src/update.rs`) fetches the manifest through it — checked
under the update key and for the version the key signed into it — reads the package for its system
from the manifest, downloads and checks that too, and only then offers *Install and restart*. A
signature that does not hold refuses the manifest or the package, and the app says which. The
`.deb` is updated by the system's package manager, so the app names the newer release and opens its
download instead. `releases/latest` never answers a pre-release, so a release candidate is offered
to nobody.

`release.yml` reads the first two rows (`apps/webapp/build/release-apps.js`): it publishes no APK
whose certificate is not the recorded one, and no app or `latest.json` whose signature does not
hold under the recorded update key, and none whose shell (`tauri.conf.json`) names another key
than the recorded one. While a row says `pending`, a release stops before it publishes anything;
until then the shell names a placeholder no release accepts. The download page shows the same values (milestone `0.9.5`).

**Creating them, once.**

1. The APK key, in a PKCS #12 store under the alias `hubtask`, valid for a hundred years (an
   Android app keeps its key for life unless it is rotated); the store's password is the key's:

   ```bash
   keytool -genkeypair -keystore hubtask-apk.p12 -storetype PKCS12 -alias hubtask \
     -keyalg RSA -keysize 4096 -validity 36500 -dname "CN=Hubtask"
   keytool -list -v -keystore hubtask-apk.p12 -alias hubtask     # the SHA256 line
   ```

2. The update key, with a password:

   ```bash
   pnpm --filter @hubtask/webapp exec tauri signer generate -w hubtask-update.key
   base64 -d < hubtask-update.key.pub                           # the key ID, then the key (RW…)
   ```

3. Four secrets of the environment `production` — environment secrets, so only the job behind
   the approval reads them ([ci-cd.md](./ci-cd.md) §6): `ANDROID_KEYSTORE_BASE64`
   (`base64 < hubtask-apk.p12`), `ANDROID_KEYSTORE_PASSWORD`, `TAURI_SIGNING_PRIVATE_KEY` (the
   content of `hubtask-update.key`) and `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`.
4. The table above, in a pull request: the certificate's SHA-256, the public key and its ID. In
   the same pull request `plugins.updater.pubkey` in `apps/webapp/src-tauri/tauri.conf.json`
   becomes the content of `hubtask-update.key.pub` (the base64 the signer writes).
5. Both files and both passwords on two offline media kept in two places; then the files are
   deleted from the machine that made them.

**What a lost or leaked key costs.**

| Key | Lost | Leaked |
|---|---|---|
| APK signing key | No installed Android app takes an update again: Android refuses an APK signed with another key. Each person uninstalls the app — which deletes its encrypted copy, and with it every change not yet synchronised — and installs the APK of a new key; the release notes and the download page say so. The installed app keeps working until then | The next APK rotates to a new key with a lineage the old key signs (`apksigner rotate`, APK Signature Scheme v3), so installed apps accept it; the release notes say which releases the leaked key signed |
| Update key | Installed desktop apps refuse every newer update, and the Android notice names none: each person downloads the next release by hand, and that release carries a new public key | A new key: the next release is signed one last time with the old key and carries the new public key, so installed apps move over; the old key is never used again |
