<!--
SPDX-License-Identifier: BUSL-1.1
Copyright (c) 2026 Jérôme Bastian Winkel
-->

# Technical and organisational measures (Art. 32 GDPR)

**What this document is.** The description an operator of Hubtask needs for their record of
processing activities, and the annex a processor is asked for. It is derived from
[security.md](../architecture/security.md) and
[data-protection.md](../architecture/data-protection.md) rather than written beside them: every
measure below names where it is decided and, where one exists, the gate that keeps it true.

**What it is not.** It is not a certification, and it is not a promise about an installation nobody
here operates. Hubtask is self-hosted software: the measures marked **product** are properties of
this software and hold wherever it runs; the ones marked **operator** are the operator's own — this
document says what the product gives them to work with, and the rest is theirs.

---

## 1. Pseudonymisation and encryption (Art. 32(1)(a))

| Measure | Where it is decided | Kind |
|---|---|---|
| Passwords are stored as Argon2id hashes; tokens as SHA-256 with a server-side pepper that is not in the database | [security.md](../architecture/security.md) §8 | product |
| Integration credentials, webhook secrets and backup target credentials are encrypted with AES-256-GCM under envelope encryption: one data key per value, the master key from the environment keyring, the key ID persisted so a rotation needs no data migration | [security.md](../architecture/security.md) §8, [ADR-0015](../adr/ADR-0015-security-baseline.md) | product |
| Backup archives are encrypted with AES-256-GCM under a key derived from a passphrase (Argon2id, RFC 9106's second recommended cost). The passphrase is stored nowhere | [backup-restore.md](../architecture/backup-restore.md) §4 | product |
| The audit trail answers a pseudonym rather than a name once a person is erased: the trail cannot be edited in place, so the substitution happens at the boundary | [audit.md](../architecture/audit.md) §6 | product |
| An access export and a data subject export carry pseudonymised references rather than other people's names | [data-protection.md](../architecture/data-protection.md) §4 | product |
| Transport is TLS 1.2 or better, HSTS where TLS is terminated | [security.md](../architecture/security.md) §8, §9 | operator (the terminating proxy is theirs) |
| Randomness for tokens, identifiers and nonces comes from `crypto/rand` only | [security.md](../architecture/security.md) §8 | product |
| Home-grown cryptography is forbidden, and a build that imports a cipher outside `infrastructure/crypto` fails | `make gate-architecture` | product, gated |

## 2. Confidentiality (Art. 32(1)(b))

| Measure | Where it is decided | Kind |
|---|---|---|
| Every workspace is a tenant, and every query runs inside a transaction that sets `app.tenant_id`; PostgreSQL row-level security enforces the boundary in the database rather than in the application | [multi-tenancy.md](../architecture/multi-tenancy.md) §3, [ADR-0010](../adr/ADR-0010-multi-tenancy.md) | product, gated (SG-3, SG-4) |
| Every repository method has a cross-tenant negative test; a new one without it fails the build | `make gate-security` (SG-3) | product, gated |
| Authorisation is decided in the application layer, never in an adapter, against a capability matrix per role and scope | [ADR-0005](../adr/ADR-0005-authn-authz.md) | product, gated |
| Multi-factor authentication can be required per workspace; personal access tokens are scoped and expire | [security.md](../architecture/security.md) §5 | product / operator (the requirement is theirs) |
| No user content reaches a log, a metric, a trace or an error response — checked by the name of every attribute in the source | `make gate-privacy` (PG-4), [ADR-0017](../adr/ADR-0017-audit-trail.md) | product, gated |
| A field written into the audit trail carries a classification, and the masking derived from it decides whether the value is written in clear, as a fingerprint, or not at all | `make gate-privacy` (PG-1), [audit.md](../architecture/audit.md) §4 | product, gated |
| No outbound connection happens without configuration; every one that does goes through a guarded client that refuses private ranges and the cloud metadata address | `make gate-privacy` (PG-6), [ADR-0015](../adr/ADR-0015-security-baseline.md) | product, gated |
| Secrets come from environment variables or mounted files only, and a missing one stops the process rather than defaulting | [security.md](../architecture/security.md) §8 | product |

## 3. Integrity (Art. 32(1)(b))

| Measure | Where it is decided | Kind |
|---|---|---|
| The audit trail is a per-tenant hash chain: each entry covers the previous entry's hash, and `POST /audit:verify` recomputes the chain and reports where it breaks | [audit.md](../architecture/audit.md) §3, §5 | product |
| The trail cannot be edited or deleted in place: the grants revoke `UPDATE` and `DELETE`, and a trigger refuses what is left | [audit.md](../architecture/audit.md) §3 | product, gated (AT-1) |
| Every action marked auditable produces exactly one entry, checked against the registry rather than against a reviewer's memory | `make gate-architecture` (SG-13) | product, gated |
| Outbound webhooks are signed with HMAC-SHA-256 and a timestamp, one secret per subscription | [security.md](../architecture/security.md) §8 | product |
| Migrations are forward-only and safe for a rolling update; an existing migration is never changed | [ADR-0003](../adr/ADR-0003-postgresql-as-single-datastore.md) | product, gated |

## 4. Availability and resilience (Art. 32(1)(b), (c))

| Measure | Where it is decided | Kind |
|---|---|---|
| Backups run as a scheduled job with generational retention, and `:verify` reads an archive back rather than trusting that it was written | [backup-restore.md](../architecture/backup-restore.md) §2, §5 | product |
| A restore is a listing at the target and six modes, and it writes a deletion journal so that what was erased does not come back | [backup-restore.md](../architecture/backup-restore.md) §6, §7 | product |
| An alert fires when no backup has succeeded in 24 hours, and when the last restore drill is older than 90 days | `deploy/observability/alerts/prometheus-rules.yaml` (A-12, A-20) | product / operator (the drill is theirs) |
| Timeouts, retries with backoff, circuit breakers and bulkheads on every external dependency; no call without a deadline | [observability-reliability.md](../architecture/observability-reliability.md) §8, [ADR-0016](../adr/ADR-0016-observability-reliability.md) | product, gated (RT-1…RT-12) |
| RPO and RTO are stated rather than implied, and the restore drill is what stands behind them | [backup-restore.md](../architecture/backup-restore.md) §10 | operator |

## 5. Regular review (Art. 32(1)(d))

| Measure | Where it is decided | Kind |
|---|---|---|
| Every pull request runs the gates: format, lint, unit, integration, contract, architecture, security SG-1…SG-13, privacy PG-1, PG-3…PG-6, PG-8, observability, licences, documentation | [ci-cd.md](../architecture/ci-cd.md) §3 | product |
| The nightly runs what needs a database or the network: PG-2 (an erasure sweeps every storage location) and PG-7 (the schema reconciled against the data catalogue), the support matrix, fuzzing, the resilience suite | [ci-cd.md](../architecture/ci-cd.md) §2 | product |
| `make gate-selftest` breaks each rule deliberately and expects the build to go red — the check that the checks are connected | [ci-cd.md](../architecture/ci-cd.md) §3 | product |
| Dependencies are scanned (`govulncheck`), pinned by commit, and shipped with an SBOM and a signature | [security.md](../architecture/security.md) §11 | product |
| Access review — who holds which role, which tokens exist, which are unused | [security.md](../architecture/security.md) §5 | operator |
| Penetration test before a `1.0.0` | [security.md](../architecture/security.md) §16 | operator |

## 6. Data minimisation, storage limitation, and the rights (Art. 5, 15–22)

| Measure | Where it is decided | Kind |
|---|---|---|
| Every field holding personal data is in the data catalogue with its class, its purpose, its retention and its deletion path; a table with personal content and no entry fails the nightly | [data-catalog.md](./data-catalog.md), `make gate-privacy-full` (PG-7) | product, gated |
| Retention rules per data kind, with a documented floor per kind and a ceiling that needs the operator's justification | [data-retention.md](../architecture/data-retention.md), `make gate-privacy` (PG-5) | product, gated |
| Access, portability, erasure, restriction, objection and rectification exist as use cases with a deadline, and the deadline is watched | [data-protection.md](../architecture/data-protection.md) §4 | product |
| An erasure sweeps every storage location, including the object store, and what may survive it is named per table and column with the reason | `make gate-privacy-full` (PG-2), [ADR-0018](../adr/ADR-0018-privacy-by-design.md) | product, gated |
| Privacy by default: AI off, no telemetry, no external search index, truncated IP addresses, notification mails without task content | [data-protection.md](../architecture/data-protection.md) §9 | product |
| Deletion journal and tombstones, so that a restore or a device that was offline does not bring an erased person back | [backup-restore.md](../architecture/backup-restore.md) §7, [offline-sync.md](../architecture/offline-sync.md) §7 | product |

## 7. Processing on behalf, and what stays with the operator

Hubtask is software, not a service. An operator running it for their own organisation is the
controller; one running it for others is a processor and needs their own contract, sub-processor
list and record of processing activities — this document is the annex to those, not a substitute.

What no version of this software can do for an operator: choose a hosting location, sign a data
processing agreement, appoint a data protection officer, run the access review, keep the drill
schedule, or decide the legal basis for a purpose. What it does do is make each of those possible
to evidence: the catalogue says what is stored, the trail says what happened, the gates say the
description still matches the code.

## 8. A personal data breach

The procedure, including the queries that answer *which workspaces, which data categories, which
period*, is [RB-GDPR-33](./RB-GDPR-33-personal-data-breach.md). The 72 hours of Art. 33 start when
the controller becomes aware, and an installation that cannot answer those three questions cannot
notify within them.

---

## 9. Open points

* **A DPA template and a sub-processor list** for provider operation
  ([security.md](../architecture/security.md) §12). Neither is written; both are needed before
  anybody operates this for somebody else.
* **PG-8** — third-country AI without an explicit confirmation — has nothing to refuse yet. There
  is no AI provider surface in this build; the gate is a tripwire that fires when one arrives, and
  the measure belongs to `0.7.0`.
* **`privacy_incident`** exists as a table and has no use case. Recording a breach in the product,
  rather than in the operator's own process, is not decided yet.
