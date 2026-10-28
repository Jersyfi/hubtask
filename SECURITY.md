# Security

## Reporting a vulnerability

**Please do not open issues for security problems.** Report them confidentially through
[GitHub Security Advisories](https://github.com/Jersyfi/hubtask/security/advisories/new) — the
preferred channel, because the report, the discussion, the fix and the advisory stay in one private
place.

Alternatively by email to `security@hubtask.eu`.

Please include: the affected version, a description, reproduction steps, and the possible impact.
Say whether you know it to be **exploited in the wild** — that changes how fast it is handled and
who is told (below). Do not use real user data, and do not test systems that are not yours.

## What you can expect

Hubtask is open source under Apache-2.0, provided as is, and maintained by one person on a
best-effort basis ([licensing-editions.md](docs/architecture/licensing-editions.md)). These are
**aims, not deadlines**:

| Step | Aim |
|---|---|
| Acknowledgement of receipt | within a few days |
| Initial assessment with CVSS | within a week or two |
| Fix for critical / high / medium | as soon as the owner can; critical first, an actively exploited one before anything else |
| Coordinated disclosure | after the fix; if no fix is in sight, we agree a date with you rather than leave the report open indefinitely |

## How a report is handled

The process follows ISO/IEC 29147 (disclosure) and ISO/IEC 30111 (handling); the project's side of
it, including what is reported to whom, is
[security.md §14](docs/architecture/security.md#14-handling-incidents).

1. **Intake** — the report is acknowledged in the private advisory (or by mail, then moved there).
2. **Triage** — reproduced, scored with CVSS, the affected versions named, and checked for being
   actively exploited.
3. **Embargo** — the report stays private until the fix is released or the agreed date arrives.
4. **Fix** — on every supported line (below), with a regression test; variants of the same flaw
   are looked for in the rest of the code.
5. **Advisory** — published as a GitHub Security Advisory with a CVE (GitHub is a CVE Numbering
   Authority), the affected and fixed versions, a workaround, and detection guidance.
6. **Credit** — as the finder, on request.

## Supported versions

Each release line has a **published support period**: the time it receives security fixes. It is
an intention, not a warranty — the project makes no maintenance commitment and may be archived —
and the rule behind the table is
[versioning-release.md §5](docs/architecture/versioning-release.md#5-support-and-upgrade-policy).

| Line | Security fixes | Support ends |
|---|---|---|
| `0.9.x` (current) | yes, in the latest `0.9` patch | when `1.0.0` is released |
| older `0.x` | no | ended with the release of the next minor |
| from `1.0.0` | the latest minor of the current major, and the previous major | for the previous major, 12 months after the next major is released |

Every release, its signed image, its SBOM and its advisories stay published; none is withdrawn when
its support ends. Every version is Apache-2.0, so anyone may carry a fix into an older one — and a
provider who sells Hubtask, or a distribution of it, declares a support period of its own
(security.md §14).

## What counts as a vulnerability

The threat model in `docs/architecture/security.md` §4 (T-01…T-28) is authoritative. Particularly
relevant: bypassing the tenant boundary, privilege escalation, SSRF through automation or backup
targets, tampering with the audit trail, and disclosure of secrets.

**Not** treated as vulnerabilities: missing hardening that the operator configures themselves
(reverse proxy, WAF, TLS termination), self-inflicted denial of service with valid credentials,
and deliberately documented limits — for instance that, without an external seal, an attacker with
permanent full database access can recompute the audit chain
(`docs/architecture/audit.md` §4).
