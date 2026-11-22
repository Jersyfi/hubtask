# OpenSSF Best Practices badge: the answers

The answers a worker enters on <https://www.bestpractices.dev> for the passing level
([ci-cd.md](../architecture/ci-cd.md) §12), and for OSPS Baseline level 1 below them. The criteria
are the 67 of the questionnaire's passing level, read on 2026-11-21 from the badge project's own
criteria file (`criteria/criteria.yml` in `coreinfrastructure/best-practices-badge`). Today:
64 Met, 0 Unmet, 3 N/A. The facts were read on 2026-11-21 or 2026-11-22 and each row says which;
two rest on the owner's word and say so.

This file stays while the badge is shown. The weekly flow report reads the badge's level from the
badge site and opens a `finding` when it is below `passing` ([ci-cd.md](../architecture/ci-cd.md)
§11); these answers are then read again against the repository.
The links point at `main`, so they resolve once the change that adds a file has merged.

## Where the answers stand

The project is registered on the badge site as
[project 17000](https://www.bestpractices.dev/projects/17000), and the answers below are entered
there. The README's "Quality and security" section shows the badge, and `best_practices_badge` in
`tools/flowstats/thresholds.json` carries the same id, so the weekly flow report reads the level.

When an answer here changes, a worker changes it on the site too: the site's sign-in is the
owner's GitHub account, and the sign-in is the owner's one act. The answers of OSPS Baseline
level 1 are not entered: its two *Unmet* rows name one file of the Android build.

## The answers

### Basics

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `description_good` | Met | The README's first paragraph and list say what Hubtask is and does. | <https://github.com/Jersyfi/hubtask/blob/main/README.md> |
| `interact` | Met | The README's quick start says how to obtain it; CONTRIBUTING.md says how to report a bug, propose a feature and contribute. | <https://github.com/Jersyfi/hubtask/blob/main/CONTRIBUTING.md> |
| `contribution` | Met | Pull requests from a fork, described in "Ways to contribute" and "A pull request". | <https://github.com/Jersyfi/hubtask/blob/main/CONTRIBUTING.md> |
| `contribution_requirements` | Met | The rules for a change, the gates and the description template are named. | <https://github.com/Jersyfi/hubtask/blob/main/AGENTS.md> |
| `floss_license` | Met | Apache-2.0. | <https://github.com/Jersyfi/hubtask/blob/main/LICENSE> |
| `floss_license_osi` | Met | Apache-2.0 is approved by the OSI. | <https://github.com/Jersyfi/hubtask/blob/main/LICENSE> |
| `license_location` | Met | `LICENSE` in the repository's root. | <https://github.com/Jersyfi/hubtask/blob/main/LICENSE> |
| `documentation_basics` | Met | Installation, operation and architecture are documented; `docs/README.md` is the entry. | <https://github.com/Jersyfi/hubtask/blob/main/docs/README.md> |
| `documentation_interface` | Met | The REST contract is `api/openapi.yaml`; the README documents the command line client. | <https://github.com/Jersyfi/hubtask/blob/main/api/openapi.yaml> |
| `sites_https` | Met | The repository, the releases and hubtask.eu are served over HTTPS. | <https://github.com/Jersyfi/hubtask> |
| `discussion` | Met | GitHub issues and GitHub Discussions: searchable, addressable by URL, open to anyone with a GitHub account. | <https://github.com/Jersyfi/hubtask/discussions> |
| `english` | Met | Everything is in English (rule 13). | <https://github.com/Jersyfi/hubtask/blob/main/AGENTS.md> |
| `maintained` | Met | Commits and releases every week; the Scorecard's *Maintained* check reads the same. | <https://github.com/Jersyfi/hubtask/commits/main> |

### Change control

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `repo_public` | Met | Public on GitHub. | <https://github.com/Jersyfi/hubtask> |
| `repo_track` | Met | git records what, who and when. | <https://github.com/Jersyfi/hubtask/commits/main> |
| `repo_interim` | Met | Every pull request's commits are public before a release. | <https://github.com/Jersyfi/hubtask/pulls> |
| `repo_distributed` | Met | git. | <https://github.com/Jersyfi/hubtask> |
| `version_unique` | Met | Every release is a tag `vX.Y.Z`. | <https://github.com/Jersyfi/hubtask/releases> |
| `version_semver` | Met | Semantic Versioning. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/versioning-release.md> |
| `version_tags` | Met | Each release is a git tag; `release.yml` runs on it. | <https://github.com/Jersyfi/hubtask/tags> |
| `release_notes` | Met | From v0.10.2 on every release has a written summary of its major changes in `docs/releases/vX.Y.Z.md`, which `release.yml` puts above GitHub's generated list and without which it stops (versioning-release.md §3). The published page of v0.10.2 carries its summary above the generated list (written and read 2026-11-22). | <https://github.com/Jersyfi/hubtask/releases/tag/v0.10.2> |
| `release_notes_vulns` | N/A | No release has fixed a publicly known vulnerability: the repository has published no security advisory (read 2026-11-21, `gh api repos/Jersyfi/hubtask/security-advisories`: none). From then on the release's file names the CVE (versioning-release.md §3). | <https://github.com/Jersyfi/hubtask/security/advisories> |

### Reporting

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `report_process` | Met | The bug form on GitHub issues. | <https://github.com/Jersyfi/hubtask/blob/main/.github/ISSUE_TEMPLATE/02-bug.yml> |
| `report_tracker` | Met | GitHub issues. | <https://github.com/Jersyfi/hubtask/issues> |
| `report_responses` | Met | No account other than the maintainer's and the repository's own jobs has opened an issue (read 2026-11-21), so no report waits for an answer. A bug report becomes a `finding` issue, which a worker answers. | <https://github.com/Jersyfi/hubtask/issues?q=label%3Afinding> |
| `enhancement_responses` | Met | As above: no enhancement request from another account exists (read 2026-11-21). | <https://github.com/Jersyfi/hubtask/issues> |
| `report_archive` | Met | GitHub issues are public and searchable. | <https://github.com/Jersyfi/hubtask/issues?q=is%3Aissue> |
| `vulnerability_report_process` | Met | SECURITY.md. | <https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md> |
| `vulnerability_report_private` | Met | A private GitHub security advisory, or `security@hubtask.eu`. | <https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md> |
| `vulnerability_report_response` | N/A | No vulnerability report was received in the last six months: no private report exists on GitHub (no advisory in any state, read 2026-11-21), and none reached `security@hubtask.eu` (the owner, 2026-11-22). | <https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md> |

### Quality

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `build` | Met | `make build`, and the container image by `make docker-build`. | <https://github.com/Jersyfi/hubtask/blob/main/Makefile> |
| `build_common_tools` | Met | make, the Go toolchain, pnpm, Docker. | <https://github.com/Jersyfi/hubtask/blob/main/Makefile> |
| `build_floss_tools` | Met | Every build tool is FLOSS. | <https://github.com/Jersyfi/hubtask/blob/main/Makefile> |
| `test` | Met | `make gate-unit` and the suites under `test/`, all in the repository under Apache-2.0. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/engineering-guidelines.md> |
| `test_invocation` | Met | `go test ./...`, `pnpm -r test`, and `make check`. | <https://github.com/Jersyfi/hubtask/blob/main/AGENTS.md> |
| `test_most` | Met | Coverage thresholds per package (85 % domain, 75 % application), and the proof state (`make proof-state`, the weekly flow report) counts the use case checks a `checked_by` file cites. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md#12-what-the-repository-shows> |
| `test_continuous_integration` | Met | `ci.yml` on every ready pull request and every push to `main`. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `test_policy` | Met | The loop for every task, step 5: tests for the domain, each repository method and each use case check. | <https://github.com/Jersyfi/hubtask/blob/main/AGENTS.md> |
| `tests_are_added` | Met | The use cases' `checked_by` names the tests added with each change; the proof state counts the checks they cite. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md#12-what-the-repository-shows> |
| `tests_documented_added` | Met | The Definition of Done and AGENTS.md's loop. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/engineering-guidelines.md> |
| `warnings` | Met | `golangci-lint` with fifteen linters for Go; `svelte-check --fail-on-warnings` under TypeScript's `strict` for the clients; `cargo clippy -D warnings` for the Rust shell. | <https://github.com/Jersyfi/hubtask/blob/main/.golangci.yml> |
| `warnings_fixed` | Met | `make gate-quick` fails on any finding; none is carried. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `warnings_strict` | Met | `.golangci.yml` switches the default set off and names fifteen linters, among them `staticcheck`, `gosec`, `revive` and `errorlint`; no finding is carried in a baseline, no preset exclusion applies, and a suppression needs its linter and a reason (`nolintlint`). TypeScript is `strict`, and a warning of `svelte-check` or `clippy` fails the build. It is a chosen list, not every linter there is. | <https://github.com/Jersyfi/hubtask/blob/main/.golangci.yml> |

### Security

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `know_secure_design` | Met | The maintainer's statement; the threat model and the security architecture are its evidence. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `know_common_errors` | Met | The maintainer's statement; the threat table names the common errors and their mitigations. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_published` | Met | Argon2id, AES-256-GCM, TLS, HMAC-SHA-256: published and reviewed (security.md §8). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_call` | Met | Go's standard library and `golang.org/x/crypto`; no own primitive (security.md §8). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_floss` | Met | Every cryptographic dependency is FLOSS. | <https://github.com/Jersyfi/hubtask/blob/main/THIRD-PARTY-LICENSES.md> |
| `crypto_keylength` | Met | AES-256, 256-bit secrets (security.md §8). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_working` | Met | No security mechanism depends on a broken algorithm. SHA-1 occurs once, in TOTP (`core/domain/model/identity/Totp.go`): HMAC-SHA-1 is RFC 6238's algorithm, the one authenticator apps expect, and HMAC does not rest on SHA-1's collision resistance (identity.md §8). MD5 occurs once, outside any security mechanism: the generated Zapier app derives an idempotency key with it (`packages/zapier-app/scripts/generate.mjs`). No DES, no RC4 (read 2026-11-21 by a search of the Go imports and the client sources). `gosec` flags an import of a weak hash; the one in TOTP is suppressed with its reason. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/identity.md> |
| `crypto_weaknesses` | Met | A SHOULD. Everything else is HMAC-SHA-256, AES-256-GCM, Argon2id and HKDF-SHA256 (security.md §8). The one algorithm with a known weakness is SHA-1 inside TOTP's HMAC, as crypto_working says; the weakness is of its collision resistance, which HMAC does not use. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_pfs` | Met | A SHOULD. The server speaks plain HTTP behind the operator's proxy, which terminates TLS (security.md §8: TLS 1.2 and above). Every TLS client in the code - outbound HTTP, SMTP, IMAP, LDAP - sets TLS 1.2 as its minimum and leaves the cipher suites to Go's defaults, which agree keys by ECDHE (read 2026-11-21: no `CipherSuites` is set anywhere). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_password_storage` | Met | Argon2id with a per-password salt (identity.md §5.3). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/identity.md> |
| `crypto_random` | Met | Keys and tokens come from the `RandomSource` port, backed by `crypto/rand` (rule 4). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `delivery_mitm` | Met | The image from ghcr.io and the release files from GitHub, both over HTTPS, signed. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `delivery_unsigned` | Met | `SHA256SUMS` is served over HTTPS, and the image and the release record are signed. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `vulnerabilities_fixed_60_days` | Met | Read 2026-11-22: no Dependabot alert, no code scanning alert of the static analysis and no secret scanning alert is open. The advisory database lists six notices for the shell's lock file; each says a crate is unmaintained or has an unsound interface and carries no severity, and `apps/webapp/src-tauri/osv-scanner.toml` names each with its reason. | <https://github.com/Jersyfi/hubtask/blob/main/apps/webapp/src-tauri/osv-scanner.toml> |
| `vulnerabilities_critical_fixed` | Met | A SHOULD. No critical vulnerability is reported: no Dependabot alert, no code scanning alert of the static analysis and no secret scanning alert is open, and no advisory exists (read 2026-11-22). SECURITY.md: critical first. | <https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md> |
| `no_leaked_credentials` | Met | Secret scanning as a push rule and gitleaks over the history (SG-7). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |

### Analysis

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `static_analysis` | Met | `gosec` in `golangci-lint` on every pull request; CodeQL for Go, JavaScript and TypeScript, and the workflows. | <https://github.com/Jersyfi/hubtask/blob/main/.github/workflows/codeql.yml> |
| `static_analysis_common_vulnerabilities` | Met | `gosec` and CodeQL's `security-extended` queries. | <https://github.com/Jersyfi/hubtask/blob/main/.github/workflows/codeql.yml> |
| `static_analysis_fixed` | Met | The static analysis reads Go, JavaScript and TypeScript, and the workflows. Its first JavaScript run on 2026-11-22 opened 24 alerts: 17 were fixed in code the same day and 7 dismissed with their reason as false positives or test-only patterns. No alert of it is open on `main` (read 2026-11-22). A red CodeQL is a finding to fix (ci-cd.md §2). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `static_analysis_often` | Met | On every ready pull request and every push to `main`. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `dynamic_analysis` | Met | Fuzzing of the parsers that take untrusted input, nightly (SG-8). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `dynamic_analysis_unsafe` | N/A | Go, TypeScript and Rust: no memory-unsafe language. | <https://github.com/Jersyfi/hubtask/blob/main/go.mod> |
| `dynamic_analysis_enable_assertions` | Met | The unit tests run with Go's race detector (`make gate-unit`). | <https://github.com/Jersyfi/hubtask/blob/main/Makefile> |
| `dynamic_analysis_fixed` | Met | The eight fuzz targets of the nightly of 2026-11-21 passed, and no open `finding` names a fuzz crash (read 2026-11-21). A crash fails the nightly and files a `finding`. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |

## OSPS Baseline level 1

The site's criteria page lists *Baseline Level 1* as a set of its own beside passing, silver and
gold (read 2026-11-21, <https://www.bestpractices.dev/en/criteria>); the page does not say whether
it is granted as a badge of its own. The controls are the 24 of `baseline-1` in the badge project's
`criteria/baseline_criteria.yml`, drawn from the Baseline of 2026-08-28. Today: 22 Met, 2 Unmet.
Every control allows N/A with a justification; none is answered that way.

| Control | Answer | Justification | URL |
|---|---|---|---|
| `osps_ac_01_01` | Met | The repository has one collaborator, its owner, whose GitHub account has two-factor authentication on (the owner's word, 2026-11-22; the API answers this to the account's owner alone). | <https://github.com/Jersyfi> |
| `osps_ac_02_01` | Met | The repository belongs to a personal account and has one collaborator, its owner (read 2026-11-21). GitHub adds a collaborator only by an invitation the owner sends by hand. | <https://github.com/Jersyfi/hubtask> |
| `osps_ac_03_01` | Met | `main` is protected: a pull request and the check `CI required` are required, force pushes are off, and the protection applies to administrators (`enforce_admins` on, set and read 2026-11-22), so no account pushes to `main` directly. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `osps_ac_03_02` | Met | The branch protection of `main` forbids its deletion (read 2026-11-21). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `osps_br_01_01` | Met | `after-ci.yml`, the workflow that runs with write rights after a fork's run, hands what the run carries to its script as environment variables, not as script text; CodeQL reads every workflow for injection. The other workflows were not read line by line here. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `osps_br_01_03` | Met | No workflow runs on `pull_request_target`; a fork's pull request runs without secrets. `after-ci.yml` runs on `workflow_run` with write rights and checks out the default branch's scripts only, nothing of the run (ci-cd.md §3.5). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `osps_br_03_01` | Met | The repository, the releases, the registry and hubtask.eu are served over HTTPS only. | <https://github.com/Jersyfi/hubtask> |
| `osps_br_03_02` | Met | The image comes from ghcr.io and the release files from GitHub, over HTTPS; the image and the release record are signed, and `SHA256SUMS` covers every file (security.md §11). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `osps_br_07_01` | Met | Secret scanning as a push rule, and gitleaks over the history in CI (SG-7). No secret scanning alert is open (read 2026-11-21). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `osps_do_01_01` | Met | The README's quick start, the installation and operation documents, and the use cases, which say for each function what a person does and sees. | <https://github.com/Jersyfi/hubtask/blob/main/docs/README.md> |
| `osps_do_02_01` | Met | CONTRIBUTING.md, "Ways to contribute", and the bug form. | <https://github.com/Jersyfi/hubtask/blob/main/CONTRIBUTING.md> |
| `osps_gv_02_01` | Met | GitHub issues and GitHub Discussions, both open (read 2026-11-21). | <https://github.com/Jersyfi/hubtask/discussions> |
| `osps_gv_03_01` | Met | CONTRIBUTING.md explains the way a contribution takes. | <https://github.com/Jersyfi/hubtask/blob/main/CONTRIBUTING.md> |
| `osps_le_02_01` | Met | Apache-2.0, approved by the OSI. | <https://github.com/Jersyfi/hubtask/blob/main/LICENSE> |
| `osps_le_02_02` | Met | The released image, apps and chart are under Apache-2.0 as the source is. | <https://github.com/Jersyfi/hubtask/blob/main/LICENSE> |
| `osps_le_03_01` | Met | `LICENSE` in the repository's root. | <https://github.com/Jersyfi/hubtask/blob/main/LICENSE> |
| `osps_le_03_02` | Met | Every release carries GitHub's source archive, which holds `LICENSE`; `THIRD-PARTY-LICENSES.md` stands beside the release's files (read 2026-11-21 on v0.10.2). | <https://github.com/Jersyfi/hubtask/releases> |
| `osps_qa_01_01` | Met | <https://github.com/Jersyfi/hubtask>, public. | <https://github.com/Jersyfi/hubtask> |
| `osps_qa_01_02` | Met | git records every change, its author and its time, publicly. | <https://github.com/Jersyfi/hubtask/commits/main> |
| `osps_qa_02_01` | Met | `go.mod` with `go.sum`, `pnpm-lock.yaml` with each `package.json`, and `Cargo.lock`. | <https://github.com/Jersyfi/hubtask/blob/main/go.mod> |
| `osps_qa_04_01` | Met | The project is this one repository (read 2026-11-21: the account holds no other repository of the project), and the map in AGENTS.md names its parts. | <https://github.com/Jersyfi/hubtask/blob/main/AGENTS.md> |
| `osps_qa_05_01` | Unmet | One generated executable is committed: `apps/webapp/src-tauri/gen/android/gradle/wrapper/gradle-wrapper.jar`, the Android build's Gradle wrapper (read 2026-11-21 over the tracked files). It stays (the owner's word, 2026-11-22): `tauri android build` runs `gradlew`, which executes it, and Tauri's template writes it. Its SHA-256 equals the one Gradle publishes for the wrapper of 9.6.1 (read 2026-11-22); the control still counts the file. | <https://github.com/Jersyfi/hubtask/tree/main/apps/webapp/src-tauri/gen/android/gradle/wrapper> |
| `osps_qa_05_02` | Unmet | The same file is the one binary a person cannot review. The other tracked binaries are images, which the control leaves out, and a 66-byte test fixture of data (`apps/webapp/src-tauri/fixtures/update/package.bin`). | <https://github.com/Jersyfi/hubtask/tree/main/apps/webapp/src-tauri/gen/android/gradle/wrapper> |
| `osps_vm_02_01` | Met | SECURITY.md: a private GitHub security advisory, or `security@hubtask.eu`. | <https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md> |
