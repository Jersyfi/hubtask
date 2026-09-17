# Licence and Edition Model

> This document describes what was decided and how it is implemented. The decision itself, with
> the options weighed against each other, is [ADR-0059](../adr/ADR-0059-licensing-phases-and-licensing-start.md),
> which supersedes [ADR-0013](../adr/ADR-0013-licensing.md) and keeps its reasoning for why BSL 1.1
> with a Change Date was chosen at all. It is not legal advice.

---

## 1. The two phases, and Licensing Start

Hubtask is published under the Business Source License 1.1, and the licence applies **separately
to each version**: the terms a version is published under are that version's terms, and they are
never tightened afterwards. What changes between versions is one parameter, the Additional Use
Grant, and it changes on a **date** rather than at a version number.

| Phase | When | The Additional Use Grant a release carries |
|---|---|---|
| **Pre-Licensing** | From the merge of ADR-0059 (2026-09-17) until Licensing Start | The interim grant of §2: any use, with the provider case as the one exception |
| **Licensing Start** ("Day X") | A date the owner announces at least 60 days in advance — in ADR-0059 (which then moves to `accepted`), the release notes, the README and the website | — |
| **Licensed** | The first release published after Licensing Start, and every release after it | The commercial grant of §4 |

Four things follow, and they are the whole point of the construction:

* **Until Licensing Start, nobody pays.** Not a company, not a freelancer, not a public body.
  The one thing the interim grant reserves is offering Hubtask itself to third parties (§2).
* **A version keeps its grant.** Every version published before Licensing Start stays under the
  interim grant until its own Change Date, whoever runs it and whatever later versions say. A
  company running `1.4.2` on the day licences go on sale may keep running it, in production,
  without a licence, until that version is Apache-2.0.
* **Licensing Start is not a version.** It may coincide with `2.0.0`; nothing requires it to. It
  waits on the prerequisites in §8, which are the owner's and have no date.
* **What is already published can only be loosened.** The `0.x` releases went out under
  ADR-0013's stricter grant; §3 grants them the interim terms in addition. The reverse — taking a
  right from a published version — is not possible, and this document will never claim it is.

The Change Date stays at **three years** per version, and the Change License stays
**Apache-2.0**. Those two are the guarantees, and they are not touched by any phase.

---

## 2. The licence and the interim grant

**Business Source License 1.1.** The full text with the parameters filled in is
[LICENSE](../../LICENSE); the body is MariaDB's verbatim, as its Covenants of Licensor require,
and only the parameters are the project's.

| Parameter | Value |
|---|---|
| Licensor | Jérôme Bastian Winkel |
| Licensed Work | Hubtask, in the version the `LICENSE` file is distributed with — a release by its tag, or otherwise the commit it is checked out from. Files that carry an `Apache-2.0` SPDX header, and subdirectories with a `LICENSE` file of their own, are not part of it (§9) |
| Additional Use Grant | **Interim (Pre-Licensing):** any use, including commercial production use by companies, organisations of every kind, and freelancers and sole traders in their professional practice — with one exception, the provider case below |
| Change Date | Three years after a given version is first publicly distributed |
| Change License | Apache-2.0 |

**The one exception.** The interim grant does not extend to making Hubtask, or a service whose
value derives substantially from it, available to third parties on a hosted, managed, embedded or
white-label basis, for a fee or as part of a commercial offering. That is the *provider* case:
offering access to Hubtask itself is what is reserved.

**What is explicitly not the provider case.** An organisation that uses Hubtask to deliver its own
products or services to its own clients — an agency running its projects in it, a practice
tracking its cases, a consultancy inviting its clients into its instance as users — is doing its
own work with Hubtask, not offering Hubtask. The grant says so in its own words.

### Who needs a licence today

| Who | Needs a licence? |
|---|---|
| A private person, for themselves or their household | No |
| A non-profit, a school, a university, a public body | No |
| A company evaluating, developing against, or contributing to it | No |
| A company running it for its own operations, or for its own clients | No |
| A freelancer or sole trader using it in their professional practice | No |
| Anyone offering Hubtask itself, or a service built substantially on it, to third parties for a fee or as part of a commercial offering | **Yes** — and until Licensing Start there is no licence to buy, so this use is reserved rather than sold. Write to licensing@hubtask.eu |

**The interim grant requires legal review before Licensing Start.** That is stated here and in
ADR-0059, deliberately not in the licence text: a note in `LICENSE` would read as a reservation
of the right to tighten what has been granted, and there is no such right. Review may sharpen the
wording of *future* releases; it cannot narrow what any published version already permits.

Two consequences worth stating plainly, unchanged since ADR-0013:

* **This is not "open source" in the OSI sense.** It is source available, and it converts to open
  source on a schedule. Describing it as open source in marketing would be false, and the project
  does not do it.
* **The Change Date applies per version.** Version 0.4.0 published in March 2027 is Apache-2.0 in
  March 2030, regardless of what happens to newer versions, to the project, or to the Licensor.
  That is the guarantee against abandonment and against a rug pull.

---

## 3. The declaration for the `0.x` releases

The `0.x` releases published before this document went out with the Additional Use Grant of
ADR-0013 in their `LICENSE` file, which permits production use only for non-commercial purposes
and treats a freelancer's practice as commercial.

**The Licensor hereby grants, for every version of Hubtask first publicly distributed with the
Additional Use Grant of ADR-0013 in its `LICENSE` file, the interim Additional Use Grant of §2 in
addition to the grant that version was distributed with.** Declared on 2026-09-17. Whoever holds
a copy of such a version may rely on either grant; the interim one is the wider, and it is the one
that applies in practice. Nothing in the original grant is withdrawn — a published licence can be
loosened, never tightened, and this is a loosening.

The declaration lives here rather than in the `LICENSE` file of those versions because those
files are what they were when they were published and stay that way; a release's `LICENSE` is not
rewritten after the fact. It is a statement by the Licensor, dated, in a public document under
version control, which is what makes it citable.

---

## 4. The commercial Additional Use Grant — a draft

> **Effective from the first release published after Licensing Start. Until then this text
> grants nothing.** It is published so that nobody is surprised by it, and it requires legal
> review before it is used. It is deliberately not in `LICENSE`: a grant in the licence file is
> a grant made.

Free of charge, with the full feature set and no limitation:

* private and household use;
* non-profit and public-benefit organisations, in furtherance of their purposes;
* teaching, study, and non-commercial research;
* evaluation, testing, development, demonstration and contribution — including by companies
  that would otherwise need a licence, as long as it is not part of serving their customers;
* **organisations with fewer than five persons**, professional and commercial use included —
  owners, employees and permanently engaged freelancers counted together, and counted together
  with affiliated organisations.

A licence is required for:

* every other organisation in production use;
* the provider case of §2 — offering Hubtask itself, or a service substantially based on it, to
  third parties — regardless of the organisation's size.

The same clarification applies as in §2: delivering one's own services to one's own clients,
including inviting them into the instance as users, is not the provider case.

The parameter text as it would appear in `LICENSE` is Appendix A of
[ADR-0059](../adr/ADR-0059-licensing-phases-and-licensing-start.md).

---

## 5. The model from Licensing Start

These are the terms the owner intends to attach to licences once they are sold. **They are not
commitments made today** — they are written down so that this document, the website and the
prerequisites in §8 describe the same thing, and so that nobody plans around a shape that was
never intended. Any of them may change before Licensing Start; what cannot change is the grant
a published version already carries.

| Offer | Shape |
|---|---|
| **Licence per major version** | One-time, priced by organisation size. Covers every release of that major, security updates included. No term, no lock-out, no technical enforcement — a licence key never restricts anything (§10) |
| **Maintenance and support contract** | Optional, annual |
| **Provider or platform licence** | Annual, for the case §2 reserves |
| **Hosted** (later) | Monthly, with a read-only archive mode and export or migration to self-hosting at any time |

**Fairness rules**, intended to accompany the licence per major:

* a paid major at most every 24 months;
* a licence bought within the twelve months before a new major makes the upgrade to that major
  free; otherwise the upgrade is offered at half the price of a new licence;
* the previous major receives security fixes for its declared support period under the Cyber
  Resilience Act.

**No prices anywhere in the repository.** Prices are published on the website when licences go
on sale, and not before.

---

## 6. No maintenance commitments before Licensing Start

Until Licensing Start, Hubtask is provided **as is**, in the sense the licence text gives those
words and in the plain one:

* **no support period** — there is no promise that any version, major or minor, receives fixes
  for any length of time;
* **no service levels and no response deadlines** — a security report is acknowledged and
  handled as soon as the owner can, and [SECURITY.md](../../SECURITY.md) states aims, not
  deadlines;
* **no roadmap commitment** — [roadmap.md](../roadmap.md) is a plan, and a plan may change or
  stop;
* **security fixes are best effort**;
* **the project may be archived at any time.**

Two guarantees stay, because they need no maintenance to keep:

1. **The Change Date.** Every published version becomes Apache-2.0 three years after its first
   public distribution. That is written into the licence of each version and needs nobody to do
   anything.
2. **A published version is never tightened.** Whatever terms a version went out with are its
   terms; this document, the website and the README will never claim otherwise.

**If the project is archived**, the code stays public where it is, and every version becomes
Apache-2.0 on its Change Date. The Licensor *may* bring that conversion forward, for some versions
or for all — that is a "may", written here so that the option is known, and not a promise.

**After Licensing Start** the picture changes for what has been sold. The Cyber Resilience Act's
manufacturer obligations attach to a major once a licence to it has been sold: a declared support
period, vulnerability handling and reporting, an SBOM. From that day, "archive at any time"
applies only to version lines without sold licences; a major with sold licences is maintained for
its declared support period because the law, not this document, says so.

---

## 7. Funding, and what happens if it does not work

Maintenance is currently funded by donations ([GitHub Sponsors](https://github.com/sponsors/Jersyfi))
and, from Licensing Start, by commercial licences.

The aim for private users is: **free, with the full feature set, for as long as the owner
maintains the project.** That is an aim with a condition attached, and the condition is stated
rather than hidden: nothing in §6 turns it into a commitment.

If donations and, later, commercial licences do not cover maintenance, the options, in order of
preference:

1. Widen the commercial boundary for **future versions** — for example, lower the threshold of the
   commercial grant in §4.
2. Introduce a paid tier for *new* private-use features, leaving the existing feature set free.
3. Move to a different licence model for **future versions**.
4. **Archive the project.** The code stays public, every version keeps its terms and its Change
   Date, and the Licensor may convert versions to Apache-2.0 early.

In every case, everything already published keeps its terms and its Change Date; no rights are
ever withdrawn retroactively. Any such change is published as an ADR **before** it takes effect,
not announced afterwards. That sequencing is the whole difference between a licence change and a
rug pull.

---

## 8. Prerequisites for Licensing Start

These are the owner's responsibilities. They are **not** backlog items, no issue tracks them, and
no milestone waits for them; Licensing Start waits for them. Each is listed so that anyone can see
what stands between today and a licence that can be bought.

| Prerequisite | What it entails |
|---|---|
| **Legal review of the grants** | The interim grant of §2 and the commercial grant of §4, by a qualified lawyer. The review may sharpen future releases' wording; it cannot narrow a published version |
| **Legal review of the commercial contract** | German law; the CISG excluded; a liability clause that holds under §§ 307 ff. BGB; the English text binding, with a German translation |
| **Pricing** | Per organisation size, per major; the fairness rules of §5 costed. Published on the website at Licensing Start, never in the repository |
| **Payment handling and VAT** | Reverse charge for business customers in the EU, the OSS scheme for consumers, possibly a merchant of record that handles both; invoices that a purchasing department accepts |
| **CRA manufacturer processes** | Vulnerability reporting through the ENISA single reporting platform; a coordinated vulnerability disclosure policy; a declared support period per licensed major; an SBOM per release (the pipeline already produces one) |
| **The trade mark** | Registration of the name and the logo as an EU trade mark at the EUIPO, so that [TRADEMARK.md](../../TRADEMARK.md) rests on a registration rather than on use alone |
| **Sponsorship tiers** | For as long as nothing is sold, the GitHub Sponsors tiers offer no consideration in return — no support, no priority, no feature — because a tier with consideration is a sale, with the obligations of one. The tiers live outside the repository and are the owner's to check |

---

## 9. Files in the repository

| File | Purpose | Status |
|---|---|---|
| [`LICENSE`](../../LICENSE) | BSL 1.1, parameters filled in, body verbatim; the Licensed Work per version, the interim grant | in place |
| [`LICENSE-APACHE`](../../LICENSE-APACHE) | The Change License text | in place |
| [`NOTICE`](../../NOTICE) | Copyright, trademarks, third-party pointer, the Apache-2.0 parts | in place |
| [`TRADEMARK.md`](../../TRADEMARK.md) | Name and logo: forks take the code, not the name | in place |
| [`CLA.md`](../../CLA.md) | Contributor License Agreement, bot-signed on first PR; the Licensor's commitment to the Change Date | in place |
| [`CONTRIBUTING.md`](../../CONTRIBUTING.md) | Contribution process, Conventional Commits, CLA | in place |
| [`SECURITY.md`](../../SECURITY.md) | Reporting path, aims, advisories | in place |
| [`CODE_OF_CONDUCT.md`](../../CODE_OF_CONDUCT.md) | Contributor Covenant | in place |
| [`THIRD-PARTY-LICENSES.md`](../../THIRD-PARTY-LICENSES.md) | Generated by `go-licenses` (`make licenses`), kept in the repository and regenerated for each release | in place |
| SPDX headers | `SPDX-License-Identifier: BUSL-1.1` in every hand-written source file of the Licensed Work; `Apache-2.0` in every file of the parts below, written by the generators for generated files | in place |

**The parts that are not the Licensed Work** are Apache-2.0, decided by ADR-0059 §6 for
[ADR-0057](../adr/ADR-0057-sdk-licence-and-extraction.md): each carries a `LICENSE` file of its
own and `Apache-2.0` headers, and `LICENSE` names the exclusion so that the two cannot be read
as contradicting each other.

| Part | Where | Note |
|---|---|---|
| The Go SDK | `sdk/go/` | Generated by `oapi-codegen`; the header comes from the template in `sdk/go/templates/` |
| The Python SDK | `sdk/python/` | Generated by `tools/sdkgen`; `pyproject.toml` carries the classifier |
| The TypeScript SDK | `sdk/typescript/` | Generated by `tools/sdkgen`; a workspace member that generates its own types from the contract and is an island on the workspace map, so nothing first-party sits in its import path. `packages/api-client` — the first-party types — stays `BUSL-1.1` throughout |
| The API contract and the event schemas | `api/` — `openapi.yaml`, `openapi.json`, `events/`, and the generator configuration beside them | What every SDK is generated from |
| The connector packages | `packages/n8n-nodes-hubtask/`, `packages/zapier-app/` | [ADR-0058](../adr/ADR-0058-connector-packages.md); same generator, same source, same publication logic — a connector a company may not ship is a connector nobody ships |

Package names and extraction into repositories of their own stay open (ADR-0057); nothing is
published from any of these directories yet.

**The CI licence gate** blocks GPL and AGPL dependencies. This is not ideology: a copyleft
dependency would make it impossible to relicense the result, which would break both the commercial
licence and the promised conversion to Apache-2.0.

It is `make gate-licenses`: `go-licenses check` with the forbidden and restricted types refused,
plus a comparison of `THIRD-PARTY-LICENSES.md` against what is actually linked - a list that has
gone stale is worse than none, because it reads as a check somebody did. `make gate-selftest`
proves the gate turns red on a disallowed type. The header test in `test/architecture` expects
`Apache-2.0` on exactly the paths above and `BUSL-1.1` everywhere else.

**Trademark protection matters more than licence text** for keeping the project's identity. The
licence permits forks; the trademark policy means a fork cannot call itself Hubtask. Official
container images come only from the project registry.

---

## 10. Editions

There is **one** code path and **one** image. The differences are legal and operational, never
functional:

| Edition | Audience | Technically |
|---|---|---|
| **Community (self-hosted)** | Anyone the grant in force covers — until Licensing Start, everyone but a provider | `HUBTASK_TENANCY_MODE=single`, every feature active, no licence key, no telemetry |
| **Commercial** (from Licensing Start) | Organisations the commercial grant does not cover; providers | Adds a licence key as legal evidence, optional metering, optional add-ons (support with a service level, managed instances, SCIM/advanced SSO, extended retention) |
| **Hosted** (later) | Customers who do not want to operate it | Multi-tenancy mode plus billing |

Implementation rules:

1. **No feature kill switch in the core.** `EditionPolicy` describes only limits that make
   operational sense (tenant count, rate limits) and is unbounded in community mode.
2. **Metering is opt-in** and off in self-hosting. `UsageRecord` (active users, items, automation
   runs, storage) exists as a port and a data model so that *any* billing model can be built later
   without touching the domain.
3. **No telemetry without consent.** An optional, documented, anonymous version ping, off by
   default.
4. **Optional commercial add-ons** — if they ever arrive — live in a separate `ee/` directory with
   its own licence file and attach through the same ports. The core stays fully functional and
   buildable without `ee/`.
5. **The licence key** (later) is a signed token with a validity period and a scope, verified
   offline, with no network requirement. An invalid key does **not** restrict any functionality;
   it produces a notice in the admin area. Technical enforcement would contradict the model and
   would cost more trust than it protects revenue.

---

## 11. Settled questions

These were open before the repository went public. They are closed, and the reasoning is in
ADR-0013 and ADR-0059:

* **Licensor** — Jérôme Bastian Winkel, as a natural person. A later transfer to a company is
  possible; the CLA already permits sublicensing, so it needs no contributor consent.
* **The boundary of the grant** — written out in the Additional Use Grant of each phase: the
  provider case is what is reserved; a freelancer's practice is not (§2), and from Licensing Start
  the line is organisation size (§4).
* **Change Date** — three years, chosen over four because the guarantee is what the community
  gets in exchange for a non-OSI licence.
* **CLA tooling** — CLA Assistant, triggered on the first pull request.
* **Legal review** — the Licensor's own responsibility, outside the project backlog (§8).
* **The SDKs, the contract and the connector packages** — Apache-2.0, in this repository (§9).

Still genuinely open, and the owner's rather than the project's:

* Everything in §8.
* The date of Licensing Start.
* Package names and extraction of the SDKs (ADR-0057).
