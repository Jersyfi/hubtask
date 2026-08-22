# Components live here, once there is a framework

This directory is empty on purpose.

Tokens and the CSS layer are framework-independent, so they exist now. Components are not, and a
component layer built before the framework decision is a component layer rebuilt at the first
contradiction ([ADR-0029](../../../docs/adr/ADR-0029-design-system-tokens.md),
`design-system.md` §1).

The decision needs its own ADR. Two constraints are already fixed and will apply to whatever is
chosen:

* The content security policy of [ADR-0028](../../../docs/adr/ADR-0028-embedded-web-ui.md) allows
  neither `'unsafe-inline'` nor `'unsafe-eval'`. A framework that needs either does not qualify.
* The component inventory is already written down — `design-system.md` §4 lists four waves, and
  the Hubtask-specific ones are derived from `domain-model.md` rather than from a generic list.

Until then: no component here, and no colour, spacing or duration written anywhere but in
`tokens/tokens.json`.
