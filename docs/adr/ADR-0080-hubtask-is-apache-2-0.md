# ADR-0080 — Hubtask is Apache-2.0

**Status:** accepted · **Date:** 2026-10-07 · **Decided:** 2026-10-06, by the owner ·
**Supersedes:** [ADR-0059](./ADR-0059-licensing-phases-and-licensing-start.md) and
[ADR-0013](./ADR-0013-licensing.md)

**Rule lives in:** [licensing-editions.md](../architecture/licensing-editions.md) §1–§5

## Context

Hubtask has been published under the Business Source License 1.1 since its first public commit:
first with an Additional Use Grant that reserved all commercial production use
([ADR-0013](./ADR-0013-licensing.md)), then with an interim grant that permitted any use except
the provider case until a "Licensing Start" the owner would announce
([ADR-0059](./ADR-0059-licensing-phases-and-licensing-start.md)). Every version was to convert to
Apache-2.0 three years after its release.

That construction existed to sell a commercial licence later. It cost something every day it
stood: a licence that is not OSI open source keeps the project out of distributions and away from
contributors, it needed a CLA, a per-version reading of the grant, a list of prerequisites for a
sale nobody was preparing, and a body of documentation explaining all of it. The owner has
decided that the sale is not the point: **Hubtask itself shall be freely usable by everyone.**

## Decision

1. **The whole repository is licensed under the Apache License, Version 2.0.** `LICENSE` is the
   verbatim Apache-2.0 text; every hand-written source file carries
   `SPDX-License-Identifier: Apache-2.0`. There is no Licensed Work, no Additional Use Grant, no
   Change Date and no Licensing Start. The parts ADR-0057 and ADR-0059 had already put under
   Apache-2.0 — the SDKs, the API contract, the connector packages — keep their own `LICENSE`
   files; they are no longer an exception, only separately extractable.
2. **What was published before is Apache-2.0 too.** The sole copyright holder, Jérôme Bastian
   Winkel, makes every previously published state of the repository — the public history of
   `main`, every tag including `v0.2.0-rc.1`, every published image — available under Apache-2.0
   as well, in addition to the terms it was published under. Nobody's rights shrink; a licence
   can always be loosened.
3. **Contributions are inbound = outbound.** A contribution is made under Apache-2.0, as its §5
   says. `CLA.md` is deleted; there is no CLA and no DCO requirement.
4. **One edition.** One code path, one image, no licence key, no `ee/` directory, no commercial
   edition, no metering for billing. The operational quotas and plans — `usage_record`, the quota
   guard, `HUBTASK_TENANCY_MODE` — stay: they are about operating an installation, not about
   licensing it.
5. **The name stays protected.** [TRADEMARK.md](../../TRADEMARK.md) stays in force and now rests
   on Apache-2.0 §6, which grants no rights in the name or the logo. A fork takes the code, not the
   name.
6. **Funding is by donation only** ([`.github/FUNDING.yml`](../../.github/FUNDING.yml)). A
   donation buys nothing in return — no support, no priority, no feature.
7. **The copyleft deny stays.** `make gate-licenses` keeps refusing GPL and AGPL dependencies: a
   copyleft dependency would change the terms under which an Apache-2.0 binary can be passed on.

## Consequences

* **Superseded:** [ADR-0059](./ADR-0059-licensing-phases-and-licensing-start.md) (never accepted;
  its Licensing Start will not come) and [ADR-0013](./ADR-0013-licensing.md). Their bodies stay
  as written, as the record of why the project once chose otherwise.
* **[ADR-0057](./ADR-0057-sdk-licence-and-extraction.md)** stays accepted, but its licence
  motivation — an SDK under BSL could not be linked into a product — is moot. Whether the SDKs
  move into repositories of their own is now a question of release cadence only.
* **Removed:** `CLA.md`, `LICENSE-APACHE` (its text is now `LICENSE`), the phase model, the
  commercial grant draft, the editions table and the prerequisites list in
  [licensing-editions.md](../architecture/licensing-editions.md), which is rewritten as a short
  statement of the model. arc42's constraint C-11 is rewritten and its risk R-02 resolved.
* **Changed:** about 2,350 SPDX headers, the two generators that write them
  (`packages/design-system/build/formats.js` and `icons.js`), the licence header test in
  `test/architecture`, every `package.json` `license` field, `NOTICE` (a short Apache notice),
  `THIRD-PARTY-LICENSES.md` and its template, the Dockerfile (which now ships `NOTICE` beside
  `LICENSE`), the README, `SECURITY.md`, the website's licence page and every sentence in the
  documentation and on hubtask.eu that said "source-available" or "not open source".
* **Not changed:** the migrations — they are immutable, and none carries a licence header.
* **The Cyber Resilience Act** treats open source supplied outside a commercial activity as out of
  scope. Hubtask is not monetised; donations without consideration do not change that. Should a
  paid offering ever appear, that assessment is made again before it does.
* **Making the licence stricter again** for future versions would need a new ADR and, with
  outside contributions under §5, could no longer be done by the owner alone. That is accepted:
  it is the point of the decision.
