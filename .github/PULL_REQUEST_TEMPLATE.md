## What and why

<!-- Keep it short. The why matters more than the what — the what is in the diff. -->

Closes #

## Use cases

<!-- The use cases and checks this pull request makes true — copy them from the task's
     `**Use cases:**` line (docs/usecases/README.md). One line per check: met or not, and how it was
     confirmed (a test, a walk, a screenshot). A check that cannot be met is reported here and in the
     issue; it is never rewritten to match the code. Move `state:` and `checked_by:` in the use case
     in this same pull request. "n/a" only for a change no use case describes (tooling, CI). -->

- UC-…: check n — met / not met — confirmed by …

## Affected areas

<!-- Tick what this touches, and apply the matching `area:` labels. This is a personal
     repository, so organisation-level issue fields are unavailable and the labels carry that
     dimension (ADR-0027). -->

- [ ] `area:core` — the domain, the application layer, the ports
- [ ] `area:api` — the OpenAPI contract, REST, MCP, the generated client
- [ ] `area:webapp` — the to-do application in the browser
- [ ] `area:website` — the project website hubtask.eu
- [ ] `area:design-system` — tokens, the CSS layer, the visual reference
- [ ] `area:infra` — persistence, storage, mail, outbound adapters, deployment
- [ ] `area:ci` — workflows, gates, release
- [ ] `area:docs` — arc42, ADRs, the backlog, the guides

## Does this need an ADR?

<!-- An ADR comes *before* the code, not with it. If a box below is ticked and no ADR exists,
     this pull request is premature: open an ADR issue instead (CONTRIBUTING.md). -->

- [ ] No — this implements a decision that is already recorded. Which one: ADR-….
- [ ] Yes, and it is in this pull request or already merged: ADR-….
- [ ] It deviates from an existing ADR, or introduces a third-party dependency, or renames or
      removes a field in `api/openapi.yaml`, or touches the licence model, the security gates or
      the retention safeguards — **none of which is decided in a pull request** (CLAUDE.md,
      "What you do not decide yourself").

## Definition of Done

<!-- Mark anything that does not apply with "n/a"; do not delete it. -->

- [ ] Tests at every relevant level green, coverage thresholds held
- [ ] `make verify` green locally, no diff after `make generate`
- [ ] `api/openapi.yaml` changed before the code was written (for API changes)
- [ ] Use case in the registry → REST, MCP, and automation (parity test green)
- [ ] Event schema added under `api/events/`
- [ ] Migration present, safe for rolling updates, tested against the previous state
- [ ] Permissions checked, a cross-tenant negative test for every new repository method
- [ ] Metric and trace span present; logs free of user content and secrets
- [ ] Timeouts everywhere, concurrency only through `SafeGo`
- [ ] Auditable action registered; new personal data fields in the data catalogue with a deletion path
- [ ] Merge rule defined for new fields (offline sync)
- [ ] Message codes in `locales/en.json`
- [ ] Documentation updated; an ADR for an architectural decision
- [ ] No colour, spacing, radius or duration value written outside `tokens.json`; `make tokens` produces no diff
- [ ] `core/` learned nothing about the frontend; no `.go` file committed under `apps/` or `packages/`

## Impact

- **Breaking change:** yes / no — if yes: the migration path
- **Security:** the threats touched (T-xx), or "none"
- **Data protection:** new processing of personal data? Purpose and retention
- **Operations:** new configuration, new dependency, new alerts?
