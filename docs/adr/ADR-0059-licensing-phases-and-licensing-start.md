# ADR-0059 — Licensing phases and Licensing Start

**Status:** proposed · **Date:** 2026-09-17 · **Supersedes:** [ADR-0013](./ADR-0013-licensing.md)

> The decisions in this record are made and are in force for Phase 1 from the day it was merged:
> `LICENSE` carries the interim grant from §2. What the status waits on is one event — the
> owner's announcement of Licensing Start with its date, at least sixty days ahead. That
> announcement is what moves this record to `accepted`, together with a release note, the README
> and the website. Until then, nothing here is a commitment about maintenance, support or the
> project's continuation (§5).

## Context

[ADR-0013](./ADR-0013-licensing.md) chose BSL 1.1 with an Additional Use Grant that reserves
*all* commercial production use — companies, and deliberately also freelancers — and said it was
to be revisited before `1.0.0`. This is that revisit, and it starts from a fact the decision of
August could not know: **`1.0.0` will ship before a licence can be sold.** Selling one needs a
reviewed grant and a reviewed contract, a price, and a way to take money and account for the VAT
on it, and none of those is done. They are the Licensor's own work, not the project's
([licensing-editions.md](../architecture/licensing-editions.md) §8),
and they have no date.

A licence that reserves commercial use while nobody can buy the reserved right is a licence that
forbids; it does not sell. A company that would pay cannot, a freelancer who reads the grant
honestly stops, and both learn that the project asks for something it cannot deliver. The
opposite mistake is as bad: promising the model, the price, or the support that will accompany a
sold licence *before* any of it exists turns a roadmap into an obligation, and an obligation is
what a project that may still be archived must not take on.

Two properties of the licence make an open-ended buffer possible without giving anything away.
BSL 1.1 applies **separately to each version** — its own text says so, and the Change Date may
vary per version — so the terms a version is published under are that version's terms, whatever
later versions say. And a licence can always be **loosened** for what is already published,
never tightened: a later grant may add rights to `0.x`; nothing can take rights from anyone who
already holds a copy. So the terms can be generous now and reserved later, with the boundary
between the two drawn by a date rather than by a version number, and every version keeps the
terms it went out with until its Change Date.

Three things sat beside the revisit and are decided with it, because they are the same decision
seen from three sides: which parts of the repository are not the Licensed Work at all
([ADR-0057](./ADR-0057-sdk-licence-and-extraction.md), proposed since 2026-09-16); what the CLA
promises a contributor about the terms their work will ever be under; and the maintenance
commitments the documentation had accumulated — a support period, response deadlines, a
"for as long as" — that were written as if a company already paid for them.

## Decision

### 1. Two phases, bound to versions, separated by a date

1. **A version's licence terms are fixed when it is published and are never tightened
   afterwards.** `LICENSE` no longer names "Hubtask, all versions" as the Licensed Work; it names
   the version the file is distributed with. The Change Date stays at three years per version;
   the Change License stays Apache-2.0.
2. **Phase 1, "Pre-Licensing"**, runs from the merge of this record until Licensing Start. Every
   release published in it carries the interim Additional Use Grant of §2.
3. **Licensing Start ("Day X")** is a date the owner announces at least sixty days in advance.
   The announcement is the event: this record moves to `accepted`, and the release notes, the
   README and the website say the date. The first release published *after* that date, and every
   release after it, carries the commercial Additional Use Grant whose draft is Appendix A. Day X
   is not tied to a version number; it may coincide with `2.0.0`, and nothing requires it to.
4. **Every version published before Day X keeps its interim grant** until its own Change Date.
   A company running `1.4.2` on the day licences go on sale is running a version it may keep
   running, in production, without a licence, until that version is Apache-2.0.
5. **The `0.x` releases already published under ADR-0013's stricter grant** are additionally
   granted the interim terms by a declaration in `licensing-editions.md` §3 — a loosening, which
   is the one direction a published licence may move.

### 2. The interim Additional Use Grant — in force now, in `LICENSE`

Any use is permitted — including commercial production use by companies, by organisations of
every kind, and by freelancers and sole traders in their professional practice — with **one
exception**: making Hubtask, or a service substantially based on it, available to third parties
on a hosted, managed, embedded or white-label basis, for a fee or as part of a commercial
offering. The grant says in its own words that an organisation using Hubtask to deliver its own
services to its own clients — including inviting those clients into its instance as users — does
**not** make such an offering: what is reserved is offering *access to Hubtask*, not doing one's
work with it.

The BSL 1.1 body stays verbatim (its Covenants of Licensor require that); only the parameters
change. The grant **requires legal review before Licensing Start** — that is stated here and in
the documentation, and deliberately not in the licence text, where it would read as a
reservation of the right to tighten it.

### 3. The commercial Additional Use Grant — a draft, effective from Day X

Appendix A holds it. It is placed in this record and in `licensing-editions.md` §4, and
deliberately **not** in `LICENSE`: a grant in the licence file is a grant made, and this one is
not made until the first release after Day X carries it. Its shape: free of charge for private
and household use, non-profit and public-benefit organisations, teaching and non-commercial
research, evaluation, development and contribution including by companies, and — new against
ADR-0013 — **organisations with fewer than five persons**, professional use included. A licence is
required for every other organisation in production use and, regardless of size, for the
provider case of §2, with the same clarification about inviting one's own clients.

### 4. The commercial model from Day X — written as intent, not as a promise

These are the terms that will accompany licences once they are sold. **They are not
commitments made today**; they are what the owner intends to offer, recorded so that the
documentation, the website and the prerequisites in §8 of `licensing-editions.md` describe the
same thing.

* A **one-time licence per major version**, priced by organisation size, covering every release
  of that major, security updates included. No term, no lock-out, no technical enforcement: a
  licence key never restricts anything, as `licensing-editions.md` has said since the start.
* An **optional annual maintenance and support contract**.
* A **provider or platform licence**, annual, for the case §2 reserves.
* **Hosted**, later: monthly, with a read-only archive mode and export or migration to
  self-hosting at any time.
* **Fairness rules**: a paid major at most every 24 months; a licence bought within the twelve
  months before a new major makes the upgrade free, otherwise the upgrade is at half price; the
  previous major receives security fixes for its declared support period under the CRA.
* **No prices anywhere in the repository.**

### 5. No maintenance commitments before Day X

Until Licensing Start, Hubtask is provided **as is**: no support period, no service levels, no
response deadlines, no roadmap commitment, and the project may be archived at any time. Security
fixes are best effort. The documentation is rewritten to say so wherever it said otherwise; the
pull request that merged this record lists each statement it weakened.

Two guarantees stay, because they need no maintenance to keep: **the Change Date**, and **that a
published version is never tightened retroactively**. If the project is archived, the code stays
public and every version becomes Apache-2.0 on its Change Date. The Licensor *may* bring that
conversion forward for some or all versions — a "may", not a promise.

After Day X the picture changes for what has been sold: the Cyber Resilience Act's manufacturer
obligations apply to a major once a licence to it has been sold, and "archive at any time" then
applies only to version lines without sold licences.

### 6. The licence part of ADR-0057 — decided

**Apache-2.0** for the Go SDK (`sdk/go/`), the Python SDK (`sdk/python/`), the TypeScript SDK
(the generated client and its example in `packages/api-client`), the API contract
(`api/openapi.yaml`, with the `api/openapi.json` generated from it) and the event schemas
(`api/events/`), and the n8n community node package (`packages/n8n-nodes-hubtask`,
[ADR-0058](./ADR-0058-connector-packages.md)). **Not** for first-party types used only by the
project's own apps — the re-export in `packages/api-client/src/index.ts` and the generated
`dist/schema.d.ts` stay under the repository licence and first-party. The Zapier app is not
named by this decision and stays under the repository licence.

Where SDK code and first-party code share a package — `packages/api-client` — the boundary
follows ADR-0057's own consequence, "only the generated client class is covered": it is drawn per
file, and Appendix B documents it. **Package names and extraction into repositories of their own
stay open**: this record decides the licence and nothing else, and nothing is created for either.

Implemented as: an Apache-2.0 `LICENSE` in every affected directory or package; SPDX headers
`Apache-2.0`, written by the generators for generated files so that `make generate` and
`make sdk` stay diff-free; `license` in the package manifests and the classifier in
`pyproject.toml`; the header test expecting `Apache-2.0` on exactly these paths and `BUSL-1.1`
everywhere else; and `LICENSE` naming the excluded files, so that the root licence and the
per-directory ones cannot be read as contradicting each other.

### 7. The CLA

The CLA gains a binding commitment from the Licensor to every contributor: a contribution is
published under the project's terms; every version it is part of receives its Change Date with
Apache-2.0 as the Change License; and no contribution is ever placed retroactively under
stricter terms, or offered only under proprietary terms. The sublicensing rights the CLA already
grants stay as they are — they are what makes the Change Date and a commercial licence possible.

## What was not chosen

Recorded, not re-weighed:

| Alternative | Why not |
|---|---|
| Keep ADR-0013's grant and sell from `1.0.0` | Nothing can be sold at `1.0.0`; a reservation nobody can buy out of is a prohibition |
| Tie Licensing Start to a version number | The prerequisites in §8 of `licensing-editions.md` have no date, and a version does; binding one to the other makes either the release or the licence wait for the other |
| Rewrite ADR-0013 in place | Accepted records are immutable; the reasoning of August is worth keeping as it was |
| Apache-2.0 for everything now | Gives away the provider case permanently, which is the one case both grants reserve |
| Put the commercial grant into `LICENSE` now, "effective from Day X" | A grant in the licence file is a grant made; a conditional one is a legal question this record cannot answer |

## Consequences

* **What a reader of `LICENSE` sees from now:** any use is fine except offering Hubtask itself
  to third parties, and this version is licensed as this version. No freelancer, no company has
  to pay today; the documentation, the README and the website say so consistently.
* **What changes on Day X:** the parameters of `LICENSE` in the next release, this record's
  status, the release notes, the README and the website. Nothing changes for any version already
  published.
* **What the project gives up until Day X:** the right to ask anybody for money, and the
  language of commitments — support periods, deadlines, "we will". What it keeps is the two
  guarantees that cost nothing to keep.
* **`licensing-editions.md`** is restructured around the phases; §8 lists the owner's
  prerequisites for Licensing Start. They are responsibilities, not backlog items, and no issue
  tracks them.
* **The roadmap** loses its licence and commercial items from version-bound milestones; they
  sit under a "Licensing Start" horizon that has no version.
* **The SDKs, the contract and the connector packages** are Apache-2.0 from this merge. A third party
  may build on them today, in a product it sells, without touching the Licensed Work's terms.
  Publication of any package stays a separate owner action, as ADR-0057 says.
* **The `0.x` declaration** loosens what was published under ADR-0013; it withdraws nothing.

## Appendix A — the commercial Additional Use Grant (draft)

> Effective from the first release published after Licensing Start. Until then this text grants
> nothing; it is the shape of what is intended, published so that nobody is surprised by it. It
> requires legal review before it is used.

```text
Additional Use Grant: You may use, copy, modify, redistribute, and self-host the
                      Licensed Work in production, with its full functionality
                      and without any feature limitation, for any of the
                      following purposes:

                      (a) use by a natural person for personal, family, or
                          household purposes, including self-hosting an instance
                          for themselves and for other members of their
                          household;

                      (b) use by a charitable, non-profit, or public-benefit
                          organisation in direct furtherance of its charitable
                          or public-benefit purposes;

                      (c) use by an educational institution for teaching or
                          study, and use by any person or organisation for
                          non-commercial scientific research;

                      (d) evaluation, testing, development, demonstration, and
                          contribution to the Licensed Work, including by an
                          organisation whose production use would otherwise
                          require a separate commercial licence, provided that
                          such use does not form part of that organisation's
                          operations serving its own customers or business
                          processes;

                      (e) use, including professional and commercial use, by an
                          organisation with fewer than five persons, where
                          "persons" means its owners, its employees, and the
                          freelancers it engages on a permanent basis, counted
                          together with the persons of every organisation
                          affiliated with it.

                      Production use by any other organisation requires a
                      commercial licence from the Licensor.

                      This grant does not extend, regardless of the size of the
                      organisation, to making the Licensed Work, or a service
                      whose value derives substantially from the Licensed Work,
                      available to third parties on a hosted, managed, embedded,
                      or white-label basis, for a fee or as part of a commercial
                      offering. An organisation that uses the Licensed Work to
                      deliver its own products or services to its own clients,
                      including by inviting those clients into its instance as
                      users, does not thereby make such an offering.
```

## Appendix B — the boundary in `packages/api-client` (superseded by the amendment of 2026-09-17)

The package is one workspace member that holds both first-party output and an SDK, and
ADR-0057's consequence draws the line per file: "the TypeScript *types* stay `BUSL-1.1` and
first-party; only the generated client class is covered." Applied:

| File | Licence | Why |
|---|---|---|
| `src/client.gen.ts` | Apache-2.0 | The TypeScript SDK: the generated client class, written by `tools/sdkgen` with its header |
| `examples/quickstart.mjs` | Apache-2.0 | The SDK's example; the Go and Python examples sit under `sdk/` and are covered the same way |
| `src/index.ts` | BUSL-1.1 | The first-party re-export of the types the apps compile against |
| `dist/schema.d.ts` (uncommitted) | BUSL-1.1 | The generated types, first-party per ADR-0057; a third party regenerates them from the Apache-2.0 document with `openapi-typescript` |
| `dist/openapi.json`, `dist/events.json` (uncommitted) | Apache-2.0 | Copies of the contract and the event schemas, which are Apache-2.0 at their source |
| `scripts/`, `package.json`, `README.md`, `CLAUDE.md`, `tsconfig.json` | BUSL-1.1 | The package's own tooling and manifest — first-party |

The one place the two licences meet is unchanged from ADR-0057: the Apache-2.0 client imports
the first-party types, and an extraction regenerates the types beside it from the same
Apache-2.0 document rather than copying them. What this boundary does **not** settle is the
package's `license` field and its `LICENSE` file, because a manifest has one of each and this
package has two licences: that is the open point the pull request describes rather than decides,
and it is one the publication decision — a package that is cut for a registry is cut from the
Apache-2.0 files alone — makes moot.

## Amendment — 2026-09-17, the owner's answers to the three points the pull request left open

The pull request that merged this record (#734) described three things rather than deciding
them. The owner answered the same day:

1. **`packages/api-client` no longer holds two licences.** The TypeScript client and its example
   move to `sdk/typescript/`, a workspace member of its own beside `sdk/go` and `sdk/python`,
   Apache-2.0 throughout with a `LICENSE` of its own. It generates its **own** `operations`
   types from the Apache-2.0 contract with `openapi-typescript` — the "regeneration rather than
   a copy" ADR-0057 foresaw for an extraction, done in place — so nothing first-party sits in
   the client's import path, and it is an island on the workspace map: it depends on no other
   member and no member depends on it (`project-structure.md` §2.1, enforced by the map lint).
   `packages/api-client` returns to types only, first-party, `BUSL-1.1`; its manifest and its
   `LICENSE` question are moot. Appendix B below is superseded by this paragraph and kept as the
   record of what the question was.
2. **`api/` is Apache-2.0 as a directory**, the server generator's configuration beside the
   contract included. A twenty-line configuration has nothing to protect, and "everything under
   `api/` is the contract and its tooling" is a rule a reader keeps without looking it up.
3. **The Zapier app is Apache-2.0 too.** ADR-0058 treats the two connector packages as twins —
   same generator, same source, same publication logic — and the sentence that decided the n8n
   node ("a connector a company may not ship is a connector nobody ships") decides this one.

§6 above therefore reads, in full: Apache-2.0 for `sdk/go`, `sdk/python`, `sdk/typescript`,
`api/`, `packages/n8n-nodes-hubtask` and `packages/zapier-app`; `BUSL-1.1` everywhere else,
`packages/api-client` included. Package names and extraction into repositories of their own
stay open, as before; the workspace name `@hubtask/sdk-typescript` is not a publication name.

## Notes

Supersedes [ADR-0013](./ADR-0013-licensing.md), whose context and options remain the record of
why BSL 1.1 with a Change Date was chosen; only the Additional Use Grant, the "all versions"
parameter and the "revisit before `1.0.0`" are replaced. Accepts the licence part of
[ADR-0057](./ADR-0057-sdk-licence-and-extraction.md). Related:
[ADR-0058](./ADR-0058-connector-packages.md) (the connector packages),
[ADR-0035](./ADR-0035-one-product-version.md) (one version, which "per version" terms attach to),
[`licensing-editions.md`](../architecture/licensing-editions.md) (the model as implemented, the
`0.x` declaration, the prerequisites), [`CLA.md`](../../CLA.md),
[`data-protection.md`](../architecture/data-protection.md) §7 (the CRA row).
