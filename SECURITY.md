# Security

## Reporting a vulnerability

**Please do not open issues for security problems.** Report them confidentially through
[GitHub Security Advisories](https://github.com/Jersyfi/hubtask/security/advisories/new).

Alternatively by email to `security@hubtask.eu` — encrypted if you prefer.

Please include: the affected version, a description, reproduction steps, and the possible impact.
Do not use real user data, and do not test systems that are not yours.

## What you can expect

Hubtask is open source under Apache-2.0, provided as is, and maintained by one person on a
best-effort basis ([licensing-editions.md](docs/architecture/licensing-editions.md)). These are
**aims, not deadlines**:

| Step | Aim |
|---|---|
| Acknowledgement of receipt | within a few days |
| Initial assessment with CVSS | within a week or two |
| Fix for critical / high / medium | as soon as the owner can; critical first |
| Coordinated disclosure | after the fix; if no fix is in sight, we agree a date with you rather than leave the report open indefinitely |

Once fixed, an advisory is published with the affected versions, a workaround, and detection
guidance. Credit as the finder on request.

## Supported versions

Security fixes go into the current minor version, best effort; no version is promised a fix, and
no version is promised a support period. Every version is Apache-2.0, so anyone may carry a fix
into an older one.

## What counts as a vulnerability

The threat model in `docs/architecture/security.md` (T-01…T-20) is authoritative. Particularly
relevant: bypassing the tenant boundary, privilege escalation, SSRF through automation or backup
targets, tampering with the audit trail, and disclosure of secrets.

**Not** treated as vulnerabilities: missing hardening that the operator configures themselves
(reverse proxy, WAF, TLS termination), self-inflicted denial of service with valid credentials,
and deliberately documented limits — for instance that, without an external seal, an attacker with
permanent full database access can recompute the audit chain
(`docs/architecture/audit.md` §4).
