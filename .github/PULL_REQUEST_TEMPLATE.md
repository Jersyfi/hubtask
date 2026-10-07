<!-- The job "Pull request description" reads this (tools/checkpr, `make gate-pr BODY=<file>`):
     every section stays, in this order, and says n/a where it does not apply. Writing the
     description with `gh pr create --body` skips this template - start from a copy of it. -->

## What and why

<!-- Keep it short. The why matters more than the what — the what is in the diff.
     A pull request without a task and without a readiness record (documentation, a dependency
     bump, a fix from outside) says so on its own line: `Readiness: n/a — <why>`. -->

Closes #

<!-- Or, when there is no issue: `No issue: <why>` on a line of its own. -->

## Use cases

<!-- The checks this pull request makes true — the ones the task carries in its `**Use cases:**`
     line. One line per check: met or not, and how it was confirmed (a test, a walk). A check that
     cannot be met is reported here and in a `decision` or `finding` issue; it is never rewritten to
     match the code. If this pull request changes a use case's Goal, How to check or Where it ends,
     say per change: `correction` (a wrong reference or number) or `decision #<issue>`. Move
     `state:` and `checked_by:` in this same pull request. "n/a" only for a change no use case
     describes (tooling, CI, documents). -->

- UC-…: check n — met / not met — confirmed by …

## Affected areas

<!-- Tick what this touches, and apply the matching `area:` labels. -->

- [ ] `area:core` — the domain, the application layer, the ports
- [ ] `area:api` — the OpenAPI contract, REST, MCP, the generated client
- [ ] `area:webapp` — the to-do application in the browser
- [ ] `area:website` — the project website hubtask.eu
- [ ] `area:design-system` — tokens, the CSS layer, the visual reference
- [ ] `area:infra` — persistence, storage, mail, outbound adapters, deployment
- [ ] `area:ci` — workflows, gates, release
- [ ] `area:docs` — subject documents, ADRs, use cases, the backlog, the guides

## Does this need an ADR?

<!-- The rule lives in a subject document; an ADR records why. A new decision brings its ADR and
     its subject-document change in this same pull request. -->

- [ ] No — this implements a rule already decided. Where it lives: `<file>.md §n`, or ADR-….
- [ ] Yes, and it is in this pull request (with the subject-document change) or already merged: ADR-….
- [ ] It deviates from a rule, or introduces a third-party dependency, or renames or removes a
      field in `api/openapi.yaml`, or touches the licence, the security gates or the retention
      safeguards — **none of which is decided in a pull request** (AGENTS.md, "What you do not
      decide yourself"): a `decision` issue first.

## Definition of Done

<!-- Mark anything that does not apply with "n/a"; do not delete it. Each item is explained in
     docs/architecture/engineering-guidelines.md §3 under the same number. -->

- [ ] 1. Tests green at every relevant level, coverage held; `make verify-pr` green for the pushed `HEAD`
- [ ] 2. `api/openapi.yaml` changed first; no diff after `make generate`
- [ ] 3. Use case in the registry → REST, MCP and automation (parity test green)
- [ ] 4. Event schemas under `api/events/`
- [ ] 5. Migration present, safe for rolling updates, tested against the previous state
- [ ] 6. Message codes in `locales/en.json`
- [ ] 7. Permissions checked; a cross-tenant negative test for every new repository method
- [ ] 8. A metric and a trace span; errors classified; logs free of user content and secrets
- [ ] 9. Timeouts everywhere, concurrency only through `SafeGo`, external effects through the outbox or jobs, idempotent, failure of the touched dependency tested
- [ ] 10. Authorisation in the application layer, outbound calls through `GuardedClient`, the affected SG gates green
- [ ] 11. Auditable action registered; new personal data in the data catalogue with a deletion path
- [ ] 12. Retention kind, merge rule for every new field, archive format and import path adjusted
- [ ] 13. Subject documents updated; an ADR for a decision; no history in documents or comments
- [ ] 14. Conventional Commit title; a breaking change marked
- [ ] 15. Client impact settled: `packages/api-client` regenerated and the web app green
- [ ] 16. Every carried use case check met with evidence; `state:`, `checked_by:` and *Today* moved
- [ ] 17. No colour, spacing, radius or duration value outside `tokens.json`; `make tokens` produces no diff
- [ ] 18. `core/` learned nothing about a frontend; no `.go` file under `apps/` or `packages/`
- [ ] 19. Reviewed against the rules no gate checks (AGENTS.md); findings: …

## Impact

- **Breaking change:** yes / no — if yes: the migration path
- **Security:** the threats touched (T-xx), or "none"
- **Data protection:** new processing of personal data? Purpose and retention
- **Operations:** new configuration, new dependency, new alerts?
