<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Jérôme Bastian Winkel
-->

# Control mapping — ISO/IEC 27001, BSI C5, OWASP ASVS

**What this document is.** A map from the controls this software has to the references an operator
cites when they build an information security management system or ask for an attestation:
ISO/IEC 27001:2022 Annex A, the BSI Cloud Computing Compliance Criteria Catalogue (C5:2020 domains)
and the chapters of OWASP ASVS 5.0. Like [tom.md](./tom.md), which it complements, it is derived
from the subject documents rather than written beside them: every row names where the control is
decided and, where one exists, the gate that keeps it true.

**What it is not.** No certification, attestation or conformity claim — Hubtask holds none, and the
project certifies nothing ([security.md](../architecture/security.md) §15). A reference here says
*which control this product capability supports*, not that the control is met: that is judged for
an installation, by its operator and their auditor. The organisational, people and physical
controls (ISO 27001 Annex A themes 5 in large part, 6 and 7; C5 OIS, SP, HR, PS, BCM as processes)
are the operator's own. The control numbers and domain codes are as the research of 2026-10 records
them ([foundation-law-standards-2026-10.md](../backlog/suite/research/foundation-law-standards-2026-10.md)
Part 2); an auditor checks them against the standard's text.

The C5 domains: OIS organisation, SP policies, HR personnel, AM assets, PS physical, OPS operations,
IDM identity and access, CRY cryptography, COS communication, PI portability and interoperability,
DEV development, SSO suppliers, SIM incident management, BCM continuity, COM compliance, INQ
investigation requests, PSS product safety and security. The ASVS 5.0 chapters: V1 encoding and
sanitisation, V2 validation and business logic, V3 web frontend, V4 API and web service, V5 file
handling, V6 authentication, V7 session management, V8 authorisation, V9 self-contained tokens, V10
OAuth and OIDC, V11 cryptography, V12 secure communication, V13 configuration, V14 data protection,
V15 secure coding and architecture, V16 security logging and error handling.

---

## 1. Access and identity

| Product control | Decided in | Kept true by | ISO 27001 Annex A | C5 | ASVS 5 |
|---|---|---|---|---|---|
| Every workspace is a tenant; every query runs in a transaction that sets the tenant, and PostgreSQL row-level security enforces it | [multi-tenancy.md](../architecture/multi-tenancy.md) §2 | SG-3, SG-4 | 8.3, 5.15 | IDM, OPS | V8 |
| Authorisation only in the application layer, against a role matrix and token scopes | [ADR-0005](../adr/ADR-0005-authn-authz.md) | SG-5 | 5.15, 5.18, 8.2 | IDM | V8 |
| Passwords as Argon2id; tokens stored as hashes; second factor (one-time codes, security keys and passkeys), enforceable per workspace; step-up for sensitive operations | [identity.md](../architecture/identity.md) §5, §8, §16, §23 | SG-11 | 5.17, 8.5 | IDM | V6 |
| Sessions listable and revocable, bounded lifetimes | [identity.md](../architecture/identity.md) §14 | SG-11 | 8.5 | IDM | V7 |
| Personal access tokens and service accounts with scopes; OpenID Connect sign-in; an OAuth2 provider for third-party apps | [identity.md](../architecture/identity.md) §10, §15 | SG-11 | 5.16, 8.5 | IDM | V9, V10 |
| An agent's call is the agent's (`AI_AGENT`), destructive tools need an explicit scope | [ai-first.md](../architecture/ai-first.md) §1.3 | the registry's door test | 8.2, 5.15 | IDM | V8 |
| An auditor role that reads the trail and the configuration and no work | [audit.md](../architecture/audit.md) §5 | the role matrix tests | 5.15, 5.35 | COM | V8 |

## 2. Cryptography and communication

| Product control | Decided in | Kept true by | ISO 27001 Annex A | C5 | ASVS 5 |
|---|---|---|---|---|---|
| Envelope encryption for stored credentials, key IDs persisted, master key rotation | [security.md](../architecture/security.md) §8 | `make gate-architecture` (no cipher outside `infrastructure/crypto`) | 8.24 | CRY | V11 |
| Randomness only from `crypto/rand`, through ports in the core (rule 4) | [security.md](../architecture/security.md) §8 | `make gate-architecture` | 8.24 | CRY | V11 |
| Backup archives encrypted before they leave the process, keys bound to the target | [backup-restore.md](../architecture/backup-restore.md) §4 | BK gates | 8.24, 8.13 | CRY, BCM | V11 |
| HTTP hardening: a content security policy without inline execution, security headers, CORS empty by default | [security.md](../architecture/security.md) §9 | the header tests | 8.26, 8.20 | COS | V3, V12 |
| Every outbound call through `GuardedClient`: allowlist, SSRF refusal, timeouts (rules 6, 7) | [security.md](../architecture/security.md) §7 | SG-6 | 8.20, 8.26 | COS | V1, V13 |

## 3. Development and supply chain

| Product control | Decided in | Kept true by | ISO 27001 Annex A | C5 | ASVS 5 |
|---|---|---|---|---|---|
| Parameterised SQL only, a closed query vocabulary, fuzzed parsers (rule 9) | [security.md](../architecture/security.md) §7 | `make gate-quick`, SG-8 | 8.28, 8.26 | DEV | V1, V2 |
| Uploads judged by content, never served inline as active content | [security.md](../architecture/security.md) §4 (T-11) | SG-12 | 8.26 | DEV | V5 |
| Layer boundaries, gates in CI, a self-test that proves each gate fails | [ci-cd.md](../architecture/ci-cd.md) §3 | `make gate-selftest` | 8.25, 8.27, 8.29, 8.32 | DEV | V15 |
| A new dependency only by ADR, pinned and licence-gated; Dependabot; `govulncheck` and a container scan | [security.md](../architecture/security.md) §11 | SG-1, SG-9 | 8.8, 5.21, 8.30 | DEV, SSO | V15 |
| A CycloneDX SBOM, a cosign signature and a provenance attestation per release | [security.md](../architecture/security.md) §11 | SG-10 | 8.32, 5.21 | DEV, PSS | V15 |
| Secure defaults: AI off, no self-registration, outbound HTTP only by allowlist, no default secret | [security.md](../architecture/security.md) §1 | PG-6 | 8.9 | OPS, PSS | V13 |
| Image distroless, non-root, read-only root filesystem | [security.md](../architecture/security.md) §11 | SG-9 | 8.9, 8.19 | OPS | V13 |
| Coordinated vulnerability disclosure, advisories with CVE, CSAF and VEX, a published support period, CRA reporting | [security.md](../architecture/security.md) §14 | — (a process) | 8.8, 5.5, 5.24–5.27 | SIM, PSS | — |

## 4. Logging, monitoring and incidents

| Product control | Decided in | Kept true by | ISO 27001 Annex A | C5 | ASVS 5 |
|---|---|---|---|---|---|
| A hash-chained audit trail, every auditable action registered, no update or delete grant | [audit.md](../architecture/audit.md) §3, §7 | SG-13, AT gates | 8.15, 5.28, 5.33 | OPS, SIM | V16 |
| No user content in logs, metrics, traces or audit entries (rule 10) | [data-protection.md](../architecture/data-protection.md) §10 | PG-4 | 8.11, 8.15, 5.34 | OPS | V16, V14 |
| Symptom-based alerts, each with a runbook; a health model | [observability-reliability.md](../architecture/observability-reliability.md) §5, §10 | the alert/runbook check | 8.16, 8.6 | OPS | V16 |
| Quotas and rate limits per tenant and credential | [multi-tenancy.md](../architecture/multi-tenancy.md) §4 | the limiter tests | 8.6 | OPS | V2 |
| The personal data breach runbook, answered from the trail | [RB-GDPR-33](./RB-GDPR-33-personal-data-breach.md) | — (a process) | 5.24–5.26, 5.34 | SIM | — |

## 5. Data protection, continuity and portability

| Product control | Decided in | Kept true by | ISO 27001 Annex A | C5 | ASVS 5 |
|---|---|---|---|---|---|
| A data catalogue of every personal field, reconciled with the schema | [data-catalog.md](./data-catalog.md) | PG-1, PG-7 | 5.34, 5.12 | AM, COM | V14 |
| Data subject rights as use cases with deadlines; erasure that reaches every store | [data-protection.md](../architecture/data-protection.md) §4 | PG-2, PG-3 | 5.34, 8.10 | COM | V14 |
| Retention by data kind with legal hold | [data-retention.md](../architecture/data-retention.md) §2 | PG-5, RE gates | 5.33, 8.10 | COM, OPS | V14 |
| Scheduled, verified backups; restore of a workspace or one item; a restore drill per release | [backup-restore.md](../architecture/backup-restore.md) §5 | BK gates, RT-9 | 8.13, 5.30 | BCM, OPS | — |
| A whole-workspace export in a documented format, importable as a new workspace | [tenant-export.md](../architecture/tenant-export.md) §12 | the export and restore tests | 5.23 | PI | — |
| AI opt-in per workspace, consent, declared jurisdiction, a person's objection; content is data, never instructions | [ai-first.md](../architecture/ai-first.md) §2 | PG-8 | 5.34, 5.23 | SSO, COM | — |

---

## 6. Keeping it current

A row changes in the pull request that changes the control it names, as the subject document does;
a control that a later step adds gets its row in that step. NIS2 Implementing Regulation
(EU) 2024/2690 and the CRA's Annex I are mapped in the same way when a provider asks for them;
until then the research's tables are the reference.
