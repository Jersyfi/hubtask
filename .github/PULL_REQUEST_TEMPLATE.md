## What and why

<!-- Keep it short. The why matters more than the what — the what is in the diff. -->

Closes #

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

## Impact

- **Breaking change:** yes / no — if yes: the migration path
- **Security:** the threats touched (T-xx), or "none"
- **Data protection:** new processing of personal data? Purpose and retention
- **Operations:** new configuration, new dependency, new alerts?
