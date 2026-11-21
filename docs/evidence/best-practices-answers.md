# OpenSSF Best Practices badge: the answers

The answers a worker enters on <https://www.bestpractices.dev> for the passing level
([ci-cd.md](../architecture/ci-cd.md) §12). The criteria are the 67 of the questionnaire's passing
level, read on 2026-11-21 from the badge project's own criteria file
(`criteria/criteria.yml` in `coreinfrastructure/best-practices-badge`). Today: 62 Met,
2 Unmet, 3 N/A; 9 of them are marked **Verify at submission**, because the fact
behind them was not read when this was written.

This file stays while the badge is shown: the weekly upkeep compares the badge's state with it.
The links point at `main`, so they resolve once the change that adds a file has merged.

## The steps at the close

1. Read each row marked **Verify at submission** against the tracker, the alerts and the
   advisories, and correct the answer here.
2. Close each *Unmet* row, or leave it and say so: the badge is granted only when every MUST is
   Met or N/A.
3. Open <https://www.bestpractices.dev/en/projects/new> in the browser and bring the page to the
   "Log in with GitHub" consent. **The consent is the owner's one act**: it lets the badge site
   read the owner's GitHub identity, and a worker stops there.
4. After the consent: register `https://github.com/Jersyfi/hubtask`, enter the answers below with
   their justification and URL, and submit.
5. Add the badge to the README's "Quality and security" section, with the project id the site
   assigned in place of `<id>`:
   `[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/<id>/badge)](https://www.bestpractices.dev/projects/<id>)`
6. OSPS Baseline level 1: the site lists it as a separate set of controls
   (`criteria/baseline_criteria.yml`). Its answers are not written yet; they are added here before
   the submission.

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
| `release_notes` | Unmet | The release pages hold GitHub's generated list of pull request titles (read: v0.10.2). The criterion asks for a human-readable summary and refuses a raw log. Met by a written summary of the major changes on each release page, which `release.yml` can take from a file. | <https://github.com/Jersyfi/hubtask/releases> |
| `release_notes_vulns` | N/A | **Verify at submission.** N/A while no release has fixed a publicly known vulnerability that had a CVE; not read here against the advisories. | <https://github.com/Jersyfi/hubtask/security/advisories> |

### Reporting

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `report_process` | Met | The bug form on GitHub issues. | <https://github.com/Jersyfi/hubtask/blob/main/.github/ISSUE_TEMPLATE/02-bug.yml> |
| `report_tracker` | Met | GitHub issues. | <https://github.com/Jersyfi/hubtask/issues> |
| `report_responses` | Met | **Verify at submission.** Bug reports become `finding` issues answered by a worker; the share acknowledged in the last 2 to 12 months was not counted here. | <https://github.com/Jersyfi/hubtask/issues?q=label%3Afinding> |
| `enhancement_responses` | Met | **Verify at submission.** As above, for the feature form; not counted here. | <https://github.com/Jersyfi/hubtask/issues> |
| `report_archive` | Met | GitHub issues are public and searchable. | <https://github.com/Jersyfi/hubtask/issues?q=is%3Aissue> |
| `vulnerability_report_process` | Met | SECURITY.md. | <https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md> |
| `vulnerability_report_private` | Met | A private GitHub security advisory, or `security@hubtask.eu`. | <https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md> |
| `vulnerability_report_response` | N/A | **Verify at submission.** N/A if no vulnerability report arrived in the last six months; with one, the answer is its response time. Not read here. | <https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md> |

### Quality

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `build` | Met | `make build`, and the container image by `make docker-build`. | <https://github.com/Jersyfi/hubtask/blob/main/Makefile> |
| `build_common_tools` | Met | make, the Go toolchain, pnpm, Docker. | <https://github.com/Jersyfi/hubtask/blob/main/Makefile> |
| `build_floss_tools` | Met | Every build tool is FLOSS. | <https://github.com/Jersyfi/hubtask/blob/main/Makefile> |
| `test` | Met | `make gate-unit` and the suites under `test/`, all in the repository under Apache-2.0. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/engineering-guidelines.md> |
| `test_invocation` | Met | `go test ./...`, `pnpm -r test`, and `make check`. | <https://github.com/Jersyfi/hubtask/blob/main/AGENTS.md> |
| `test_most` | Met | Coverage thresholds per package (85 % domain, 75 % application), and the proof state counts the use case checks a test cites. | <https://github.com/Jersyfi/hubtask/blob/main/docs/reference/proof-state.md> |
| `test_continuous_integration` | Met | `ci.yml` on every ready pull request and every push to `main`. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `test_policy` | Met | The loop for every task, step 5: tests for the domain, each repository method and each use case check. | <https://github.com/Jersyfi/hubtask/blob/main/AGENTS.md> |
| `tests_are_added` | Met | The use cases' `checked_by` names the tests added with each change; the proof state counts them. | <https://github.com/Jersyfi/hubtask/blob/main/docs/reference/proof-state.md> |
| `tests_documented_added` | Met | The Definition of Done and AGENTS.md's loop. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/engineering-guidelines.md> |
| `warnings` | Met | `golangci-lint` for Go, ESLint and `tsc` for TypeScript. | <https://github.com/Jersyfi/hubtask/blob/main/.golangci.yml> |
| `warnings_fixed` | Met | `make gate-quick` fails on any finding; none is carried. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `warnings_strict` | Met | The linters run with the project's full rule set and a frozen baseline. | <https://github.com/Jersyfi/hubtask/blob/main/.golangci.yml> |

### Security

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `know_secure_design` | Met | The maintainer's statement; the threat model and the security architecture are its evidence. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `know_common_errors` | Met | The maintainer's statement; the threat table names the common errors and their mitigations. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_published` | Met | Argon2id, AES-256-GCM, TLS, HMAC-SHA-256: published and reviewed (security.md §8). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_call` | Met | Go's standard library and `golang.org/x/crypto`; no own primitive (security.md §8). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_floss` | Met | Every cryptographic dependency is FLOSS. | <https://github.com/Jersyfi/hubtask/blob/main/THIRD-PARTY-LICENSES.md> |
| `crypto_keylength` | Met | AES-256, 256-bit secrets (security.md §8). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_working` | Met | No MD5, SHA-1, DES or RC4 in a security mechanism (security.md §8); `gosec` flags them. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_weaknesses` | Met | As crypto_working; AES in GCM mode. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_pfs` | Met | **Verify at submission.** TLS 1.2 and above, 1.3 as the target, terminated where the operator terminates it; the suites were not read here. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `crypto_password_storage` | Met | Argon2id with a per-password salt (identity.md §5.3). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/identity.md> |
| `crypto_random` | Met | Keys and tokens come from the `RandomSource` port, backed by `crypto/rand` (rule 4). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `delivery_mitm` | Met | The image from ghcr.io and the release files from GitHub, both over HTTPS, signed. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `delivery_unsigned` | Met | `SHA256SUMS` is served over HTTPS, and the image and the release record are signed. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `vulnerabilities_fixed_60_days` | Unmet | **Verify at submission.** Three Dependabot alerts were open when this was written, their severity and age not read here. Met once none of medium or higher severity is older than 60 days. | <https://github.com/Jersyfi/hubtask/security/dependabot> |
| `vulnerabilities_critical_fixed` | Met | **Verify at submission.** SECURITY.md: critical first. No critical report is known; not read against the alerts here. | <https://github.com/Jersyfi/hubtask/blob/main/SECURITY.md> |
| `no_leaked_credentials` | Met | Secret scanning as a push rule and gitleaks over the history (SG-7). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |

### Analysis

| Criterion | Answer | Justification | URL |
|---|---|---|---|
| `static_analysis` | Met | `gosec` in `golangci-lint` on every pull request; CodeQL for Go, JavaScript and TypeScript, and the workflows. | <https://github.com/Jersyfi/hubtask/blob/main/.github/workflows/codeql.yml> |
| `static_analysis_common_vulnerabilities` | Met | `gosec` and CodeQL's `security-extended` queries. | <https://github.com/Jersyfi/hubtask/blob/main/.github/workflows/codeql.yml> |
| `static_analysis_fixed` | Met | **Verify at submission.** A red CodeQL is a finding to fix (ci-cd.md §2); the open code scanning alerts were not read here. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `static_analysis_often` | Met | On every ready pull request and every push to `main`. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/ci-cd.md> |
| `dynamic_analysis` | Met | Fuzzing of the parsers that take untrusted input, nightly (SG-8). | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
| `dynamic_analysis_unsafe` | N/A | Go, TypeScript and Rust: no memory-unsafe language. | <https://github.com/Jersyfi/hubtask/blob/main/go.mod> |
| `dynamic_analysis_enable_assertions` | Met | The unit tests run with Go's race detector (`make gate-unit`). | <https://github.com/Jersyfi/hubtask/blob/main/Makefile> |
| `dynamic_analysis_fixed` | Met | **Verify at submission.** A fuzz crash fails the nightly and files a `finding`; whether one is open was not read here. | <https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/security.md> |
