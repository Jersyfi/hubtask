<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Jérôme Bastian Winkel
-->

# The NIS2 supplier pack

**What this document is.** The answers an operator who is an essential or important entity under
NIS2 (Directive (EU) 2022/2555; in Germany the BSIG as the NIS2UmsuCG wrote it) needs from a
supplier, for its own supply-chain security (Art. 21(2)(d), BSIG §30(1) no. 4) and, where it is a
cloud or managed service provider, for Implementing Regulation (EU) 2024/2690 (Annex, point 5).
Each row names what the project supplies and where it lives, so a security questionnaire is
answered by a link rather than a new text. A provider who sells Hubtask hands this on with its own
rows beside it ([security.md](../architecture/security.md) §14.4).

**What it is not.** No attestation, certification or contractual commitment: the project sells
nothing, gives no warranty and commits to no deadline ([versioning-release.md](../architecture/versioning-release.md)
§5); every period below is an aim. Whether a control is met is judged for an installation, by its
operator ([control-mapping.md](./control-mapping.md)). Not legal advice.

## 1. What the software is made of

| The operator asks | The project supplies | Where |
|---|---|---|
| A software bill of materials | A CycloneDX SBOM of every release image, attached to the GitHub release | `sbom.cdx.json`; `release.yml` (SG-10) |
| That the image is the project's | A keyless cosign signature and SLSA build provenance per image | The registry; `release.yml`, verified as [deployment.md](../architecture/deployment.md) §7 describes |
| The third-party licences | The licence list regenerated per release; no copyleft in the image | `THIRD-PARTY-LICENSES.md` on the release; the licence gate |
| What a release was | The signed release record: commit, image digest, pipeline run, gates, the SBOM's hash, the advisories it fixes | `release-record.json` on the release ([versioning-release.md](../architecture/versioning-release.md) §3) |

## 2. How vulnerabilities are handled

| The operator asks | The project supplies | Where |
|---|---|---|
| A disclosure policy and contact | Coordinated disclosure after ISO/IEC 29147 and 30111; a private advisory or `security@hubtask.eu`; `security.txt` | [`SECURITY.md`](../../SECURITY.md), [security.md](../architecture/security.md) §14.1, `/.well-known/security.txt` |
| Advisories it can process | GitHub advisories with CVE; CSAF 2.0 documents and VEX *not affected* statements | `/.well-known/csaf/` ([security.md](../architecture/security.md) §14.2) |
| How fast a fix comes | Acknowledgement, assessment and fix as `SECURITY.md` states its aims; a fix on every line in its support period | [`SECURITY.md`](../../SECURITY.md), [versioning-release.md](../architecture/versioning-release.md) §5 |
| How long a version is fixed | A published support period per line; no release withdrawn | [versioning-release.md](../architecture/versioning-release.md) §5 |
| That exploited flaws are reported | The CRA Art. 14 runbook: early warning in 24 h, notification in 72 h, final report, users informed through the advisory | [security.md](../architecture/security.md) §14.3 |
| How the code is checked | The security gates on every pull request and release: static analysis, dependency reachability, secrets, fuzzing and image scans nightly | [security.md](../architecture/security.md) §13 |

## 3. What the software does for the operator's own measures

| NIS2 Art. 21(2) / 2024/2690 Annex | The product capability | Where |
|---|---|---|
| (a) risk analysis, (e) secure development | Threat model, secure defaults, the gates | [security.md](../architecture/security.md) §1, §13 |
| (b) incident handling, 2024/2690 point 3 | An append-only, hash-chained audit trail and its verification; structured logs without user content; alerts with runbooks | [audit.md](../architecture/audit.md), [observability-reliability.md](../architecture/observability-reliability.md) |
| (c) business continuity, 2024/2690 point 4 | Backups, restore drills per release, point-in-time recovery, a full workspace export and import | [backup-restore.md](../architecture/backup-restore.md), [tenant-export.md](../architecture/tenant-export.md) |
| (h) cryptography | Envelope encryption of stored credentials, TLS for every connection | [security.md](../architecture/security.md) §8 |
| (i) access control, (j) MFA | Roles and custom roles from a permission catalogue, second factor enforceable per workspace, passkeys, step-up, scoped tokens, revocable sessions | [identity.md](../architecture/identity.md), [control-mapping.md](./control-mapping.md) §1 |
| 2024/2690 point 6 (configuration management) | The configuration document: export, plan, apply, history and rollback of a workspace's configuration | [configuration.md](../architecture/configuration.md) |

## 4. What stays the operator's

Its own risk assessment, the supplier register entry for Hubtask, the installation's hardening
(network, ingress, the database, the host), its NIS2 registration and reporting to its CSIRT, its
staff training, and — where it sells Hubtask on — everything a manufacturer owes under the CRA
([security.md](../architecture/security.md) §14.4).
