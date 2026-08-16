# Security

## Reporting a vulnerability

**Please do not open issues for security problems.** Report them confidentially through
[GitHub Security Advisories](https://github.com/Jersyfi/hubtask/security/advisories/new).

Alternatively by email to `security@hubtask.eu` — encrypted if you prefer.

Please include: the affected version, a description, reproduction steps, and the possible impact.
Do not use real user data, and do not test systems that are not yours.

## What you can expect

| Step | Deadline |
|---|---|
| Acknowledgement of receipt | 72 hours |
| Initial assessment with CVSS | 7 days |
| Fix for critical / high / medium | 7 / 30 / 90 days |
| Coordinated disclosure | after the fix, at the latest 90 days |

Once fixed, an advisory is published with the affected versions, a workaround, and detection
guidance. Credit as the finder on request.

## Supported versions

Before `1.0.0`: only the current minor version. After that, the policy in
`docs/architecture/versioning-release.md` applies.

## What counts as a vulnerability

The threat model in `docs/architecture/security.md` (T-01…T-20) is authoritative. Particularly
relevant: bypassing the tenant boundary, privilege escalation, SSRF through automation or backup
targets, tampering with the audit trail, and disclosure of secrets.

**Not** treated as vulnerabilities: missing hardening that the operator configures themselves
(reverse proxy, WAF, TLS termination), self-inflicted denial of service with valid credentials,
and deliberately documented limits — for instance that, without an external seal, an attacker with
permanent full database access can recompute the audit chain
(`docs/architecture/audit.md` §4).
