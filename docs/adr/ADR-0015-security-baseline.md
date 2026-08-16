# ADR-0015: Security as an enforced baseline rather than a review practice

* **Status:** accepted
* **Date:** 2026-08-14
* **Concerns:** security, CI, process
* **Related:** [ADR-0005](./ADR-0005-authn-authz.md), [ADR-0010](./ADR-0010-multi-tenancy.md), [security.md](../architecture/security.md)

## Context

Hubtask is open, multi-tenant, and operated by third parties themselves. Three circumstances make
the situation sharper than for an ordinary to-do application:

1. The source is public — attackers know the structure, and "security by obscurity" is off the table.
2. The automation engine triggers outbound HTTP calls; the application is therefore a potential SSRF and amplification tool.
3. In provider operation, other tenants live in the same database; a single missing `WHERE tenant_id` filter would be a reportable data protection incident.

Security requirements that exist only as prose in a policy document erode across releases, in
experience — especially with changing contributors in an open project.

## Decision

Security is implemented as an **enforced baseline**:

1. Every protective rule exists at two levels at least (defence in depth); the tenant boundary is additionally enforced by PostgreSQL RLS with a role that lacks `BYPASSRLS`.
2. Every rule has an automated proof. The twelve gates SG-1 … SG-12 from [security.md](../architecture/security.md) §13 fail the build; exceptions are not possible by comment, only through a new ADR.
3. The unconfigured state is the safe one (secure by default): AI off, registration closed, CORS empty, outbound targets only by allowlist, no default for a secret.
4. Missing context leads to rejection, never to passage (fail closed).
5. Outbound HTTP calls run exclusively through a `GuardedClient` with SSRF protection; using `http.Client` directly outside the adapter is a lint error.
6. A documented STRIDE threat model (T-01 … T-20) is part of the architecture and is extended with every new bounded context.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: an enforced baseline with CI gates** | The highest implementation effort up front, but the only variant that holds over years and changing contributors. |
| Policies plus code review | Common practice, but it does not scale with external contributions; reviewers miss exactly the cases that occur rarely (cross-tenant, SSRF). |
| An external audit instead of internal controls | An audit is a snapshot; without gates the result decays with the next feature. It is carried out in addition (S-1), but it does not replace the gates. |
| Isolation per tenant through separate databases | The strongest isolation, but incompatible with the operating goal of "one Compose file for private users" and expensive in provider operation; rejected in [ADR-0010](./ADR-0010-multi-tenancy.md). RLS is the compromise with an enforced boundary. |

## Consequences

**Positive**

* A forgotten tenant filter leads to empty results rather than to another tenant's data.
* Security commitments are verifiable — relevant to provider customers and to trust in an open project.
* New contributors learn the rules through red builds rather than through reviewer comments.

**Negative / countermeasures**

* *A slower pipeline and a higher barrier to entry.* → Gates staggered by runtime: fast checks on the PR, fuzzing and scans overnight or in the release; `make verify` reproduces the PR gates locally.
* *False positives block work.* → The baseline is frozen, with a documented route to suppressing individual findings with a justification in the code.
* *RLS costs performance and complicates some queries.* → Every index begins with `tenant_id`; a load test against 10⁶ items per tenant as the counter-check.
* *The completeness check for the cross-tenant tests (SG-3) is itself code that has to be maintained.* → Deliberately accepted; reconciling methods against tests is a few dozen lines and has the highest protective value in the project.
