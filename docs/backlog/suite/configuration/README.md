# One configuration model

Accepted with the suite on 2026-10-28 ([`../accepted.md`](../accepted.md)) · Written 2026-10-28 ·
Part of the suite's running plan ([`../README.md`](../README.md)). Each section moves to its subject
document (`multi-tenancy.md`, `domain-model.md`, `design-system.md`, …) with the pull request that
builds it.

The owner's brief (2026-10-28, paraphrased): administration — for people at every level and for AI
agents — must be intuitive, clear about who configures what where, give one solution per feature,
work with templates that set a direction while customers shape their own world, and let industry
standards (ITIL for service, Scrum and agile, classic project management) be built on the base
features. Every existing screen changes where the result requires it.

## The files

| File | What it settles |
|---|---|
| [`today.md`](./today.md) | What Hubtask's administration is today, read from the code on `main` (b832f6ceb), and where it already splits the way the market does |
| [`model.md`](./model.md) | The model: levels and who configures them, the building blocks, the one-solution rule, the settings catalogue |
| [`templates.md`](./templates.md) | Moved to `docs/architecture/configuration.md` §6–9 |
| [`standards.md`](./standards.md) | ITIL 4, Scrum, Kanban, scaled agile, PRINCE2 / PMBOK, OKR and others mapped onto building blocks and templates |
| [`connected.md`](./connected.md) | The connected suite: one reference type, mention / card / live view, conversions, designed synergies (P-19) |
| [`config-document.md`](./config-document.md) | Moved to `docs/architecture/configuration.md` §1–5 |
| [`ui.md`](./ui.md) | The administration's information architecture and the patterns every settings screen follows; the inventory of existing screens that change |
| [`../decisions.md`](../decisions.md) | The owner's questions D-11 … D-18, as they were put — answered in [`../accepted.md`](../accepted.md) |
| [`../research/admin-2026-10.md`](../research/admin-2026-10.md) | The web research behind it (Atlassian, others, standards), with sources |

## In one paragraph

Hubtask already holds the core of the right answer in two places — the installation's settings
(value and lock, a kind declared in the domain, three doors reading one document) and the
structure settings (layers from installation to collection, locks with their origin, presets). The
model makes that the **only** way anything is configured: one catalogue of settings and building
blocks, each with exactly one home per level, its source and its lock always visible, every door —
screen, `hubctl`, REST, MCP, file — reading and writing the same document through the same use
cases; one building block per feature (a type's *form* is the only way an entry of that type is
created or edited, wherever it is created); templates that bundle building blocks and carry their
version so they can be upgraded; and a plan → apply cycle with a preview that a person or an agent
uses alike.
