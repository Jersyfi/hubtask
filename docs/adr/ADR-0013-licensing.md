# ADR-0013 — BSL 1.1 with a conversion to Apache-2.0

**Status:** accepted · **Date:** 2026-08-16 · **Supersedes:** the proposed draft of 2026-08-14

## Context

Four requirements shape the licence, and they pull in different directions:

1. **The source is public and the community contributes.** People need to read it, fork it, run
   it, and send patches without asking anyone.
2. **Private individuals use it free of charge, with the full feature set.** No crippled community
   edition, no feature gate, no licence key that switches things off.
3. **Commercial use is sold, and sold only to companies.** This is the funding model. If
   companies can use it for free, there is nothing to sell.
4. **The model must stay changeable.** Today maintenance is covered by donations. If donations
   stop covering it, moving to a different model has to remain possible without hunting down
   every past contributor for permission.

A licence can be loosened later, but practically never tightened: everything already published
stays published under the terms it went out with. So this decision had to be made before the
repository went public, and the repository is already public — which is why it is being closed
now rather than left open.

## Decision

**Business Source License 1.1**, with:

* **Additional Use Grant** — free use, including production self-hosting with the complete
  feature set, for any non-commercial purpose. The grant enumerates what that covers: private and
  household use, non-profit and public-benefit organisations, teaching and non-commercial
  research, and evaluation/development/contribution even by companies that would otherwise need a
  paid licence.
* **Change Date** — three years after a given version is first publicly distributed.
* **Change License** — Apache-2.0.

Supporting pieces, all now in place: a [CLA](../../CLA.md) so the conversion and commercial
licensing are actually possible, a [trademark policy](../../TRADEMARK.md) covering the name and
logo, `NOTICE`, `LICENSE-APACHE`, SPDX headers (`BUSL-1.1`) in the source, and a CI check that
blocks GPL/AGPL dependencies — those would make relicensing impossible. Product-side details are
in [licensing-editions.md](../architecture/licensing-editions.md).

## Options considered

| Option | Verdict |
|---|---|
| **BSL 1.1 + Apache-2.0 (chosen)** | The only option that reserves *all* commercial production use while giving private users everything, and the Change Date gives the community a hard, dated guarantee. |
| AGPL-3.0 + CLA | Genuinely OSI open source and the best community optics. But it permits commercial use outright — a company that complies with the copyleft owes nothing, which guts requirement 3. Works as a *lever* (comply or buy an exception), not as a reservation. |
| FSL 1.1 / Fair Core | The modern, well-drafted successors to BSL, and shorter to read. They forbid only competing offerings and explicitly allow internal commercial use — exactly the use we intend to sell. Fails requirement 3 by design. |
| Elastic License 2.0 | Same shape as FSL, same mismatch, and no conversion to open source at all. |
| PolyForm Noncommercial 1.0.0 | Very cleanly drafted non-commercial terms, and its definitions informed our Additional Use Grant. But it has no Change Date — nothing ever becomes open source, so the community gets no long-term guarantee. |
| MIT / Apache-2.0 | Gives away every monetisation option permanently. |
| Open core | Contradicts requirement 2: private users would not get the full feature set. |

## Consequences

**What this buys.** Requirements 1–4 are all met. Monetisation stays open in every form —
subscription, usage-based, hosted — because nothing has been given away. Each version becomes
real open source on a fixed date, which is a credible answer to "what if you abandon this or sell
out". And no technical enforcement is needed: there is no kill switch to build or to be resented
for.

**What this costs.** BSL is not OSI-approved, so this is "source available", not "open source",
and saying otherwise would be dishonest. Some of the community will refuse to engage on
principle. Linux distributions will not package it. Some corporate policies and public funding
programmes exclude non-OSI licences outright. The CLA is a second barrier on top, and some
contributors' employers will not sign one.

**Living with it.** The three-year Change Date is deliberately shorter than MariaDB's or
HashiCorp's four, because the guarantee is the main thing the community gets in return. The
boundary of "commercial" is written out in the licence rather than left to intuition — including
the deliberate, and debatable, decision that a freelancer using Hubtask in their practice needs a
paid licence. No-cost commercial licences for non-profits, schools, and public bodies are granted
on request as a matter of policy, which is a lever that can be pulled without amending the
licence. This ADR is to be revisited before 1.0.0.

**Legal review.** The construct is deliberate and internally consistent, and it is not a
substitute for advice from a qualified lawyer. Review is the Licensor's own responsibility and is
not tracked as a project task.
