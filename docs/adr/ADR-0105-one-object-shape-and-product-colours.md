# ADR-0105 — One object shape, and product colours that identify

**Status:** accepted · **Date:** 2026-11-15 · **Decided:** 2026-11-12, by the owner (the concept
[work-surface](../backlog/work-surface/README.md) §4, §5, released as proposed)

**Rule lives in:** [user-experience.md](../architecture/user-experience.md) §1 (UX-9…UX-16), §5.6;
[design-system.md](../design/design-system.md) wave 7 (`ObjectHead`, `PropertyRail`,
`StatusButton`, `ProductBand`) and the work surface's tokens (`product.<id>`,
`chart.categorical.1–7`, `layout.rail`, `layout.reading`, `density.<mode>.row.height`).

## Context

An entry, a ticket, an inventory object, a goal, a release, an agent and a portfolio each drew a
head of their own; lists were stacks of cards; a product's colour was picked per component, and
charts took whatever colours lay at hand. A person moving between products had to learn each
screen again.

## Decision

* **One object, one shape.** Every object page starts with one head - key with the product's tile,
  type, title, the status as a button with where it can go, the next step as the one primary - and
  keeps everything but description, parts and conversation in a property rail beside it, folded
  when empty.
* **A product's colour identifies and means nothing.** Components name the product through alias
  tokens (`product.<id>.surface`, `.ink`) of the navigation's label colours; the colour never
  marks a state, a button or a chart series. Charts take one fixed categorical palette of seven,
  each at least 3:1 against every surface, with a table beside them.
* **Lists are calm tables** with a density the person chooses; every device keeps the functions,
  only the arrangement changes.

## Alternatives

* *A shape per product*, as before: each product free, every person relearning.
* *Product colours as the chart palette*: a chart of desk and plan figures would read as a statement
  about the products, and two of the colours (time, goals) are close.

## Consequences

The blocks are built once in the design system (wave 7) before any screen uses them, and the
product screens draw them instead of heads and lists of their own. Tokens are only added; no
existing value changes.
