# Security

## Reporting a vulnerability

**Please do not open issues for security problems.** Report them confidentially through
[GitHub Security Advisories](https://github.com/Jersyfi/hubtask/security/advisories/new).

Alternatively by email to `security@hubtask.eu` — encrypted if you prefer.

Please include: the affected version, a description, reproduction steps, and the possible impact.
Do not use real user data, and do not test systems that are not yours.

## What you can expect

Until Licensing Start ([licensing-editions.md](docs/architecture/licensing-editions.md) §6) the
project is maintained by one person on a best-effort basis, and these are **aims, not deadlines**:

| Step | Aim |
|---|---|
| Acknowledgement of receipt | within a few days |
| Initial assessment with CVSS | within a week or two |
| Fix for critical / high / medium | as soon as the owner can; critical first |
| Coordinated disclosure | after the fix; if no fix is in sight, we agree a date with you rather than leave the report open indefinitely |

Once fixed, an advisory is published with the affected versions, a workaround, and detection
guidance. Credit as the finder on request.

From Licensing Start, a major that licences have been sold for carries a declared support period
and the vulnerability-handling obligations of the Cyber Resilience Act; the deadlines that come
with them are stated here from that day.

## Supported versions

Until Licensing Start, security fixes go into the current minor version, best effort; no version
is promised a fix, and no version is promised a support period. What every version keeps is its
Change Date: three years after publication it is Apache-2.0, whatever happens to the project.
From Licensing Start, the policy in `docs/architecture/versioning-release.md` §5 applies to majors
with sold licences.

## What counts as a vulnerability

The threat model in `docs/architecture/security.md` (T-01…T-20) is authoritative. Particularly
relevant: bypassing the tenant boundary, privilege escalation, SSRF through automation or backup
targets, tampering with the audit trail, and disclosure of secrets.

**Not** treated as vulnerabilities: missing hardening that the operator configures themselves
(reverse proxy, WAF, TLS termination), self-inflicted denial of service with valid credentials,
and deliberately documented limits — for instance that, without an external seal, an attacker with
permanent full database access can recompute the audit chain
(`docs/architecture/audit.md` §4).
