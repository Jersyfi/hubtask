# ADR-0029 — The design system is code, and `tokens.json` is its only origin

**Status:** accepted · **Date:** 2026-08-22

## Context

[ADR-0027](./ADR-0027-monorepo-structure.md) gives the design system a home in
`packages/design-system`. What is not yet decided is where its *values* come from — the colours,
spacings, radii, durations and easings that two clients will both consume — and that is the
decision that determines whether the design system still means anything in a year.

Design systems fail in one specific way: the same value ends up written in two places, the two
drift, and after a while nobody can say which bordeaux is the bordeaux. It is not a discipline
problem. It is a structural one, and it has a structural answer — a value that exists in exactly one
file cannot disagree with itself.

The usual answer, a design tool as the master with an export step into code, does not work for this
project for a reason particular to it: the frontend is built AI-assisted. An assistant works from
what is in the repository. A specification that lives in a design tool is, to everything that reads
this repository, not a specification at all — it is a rumour, recoverable only through a manual,
lossy, unreviewable sync step that nobody can see in a diff. A specification next to the code it
governs is read, diffed, and reviewed like the code.

There is also a backend consequence, which is what makes this more than a frontend concern.
`domain-model.md` §4 stores a `colorToken` on `Label` and on `cover` — a name, deliberately not a
hex value, so that theming stays possible. The backend therefore has to validate that a submitted
token is one of the ten that exist. A hand-maintained list of ten names in Go, next to a
hand-maintained list of ten names in JSON, is precisely the two-places-one-value shape described
above, and it will drift the first time an eleventh colour is added.

## Options

**A. A design tool as master, exported to code.** Rejected: the sync is manual, so it is skipped;
it is not reviewable, so a wrong export is invisible; and the specification is unreadable to
everything that reads the repository, including the assistant doing most of the work.

**B. Tokens duplicated per application.** `apps/webapp` and `apps/website` each carry their own
values. Rejected: it drifts immediately and by construction — two clients of one product, showing
two different bordeaux.

**C. Hand-maintained CSS custom properties with no source format.** The values live in a
`tokens.css` written by hand. Rejected: it has no types for a TypeScript consumer, it cannot
produce a second or third target, and there is nothing to compare a committed artefact against, so
drift cannot be detected — only noticed.

**D. One source file, several generated targets (chosen).**

## Decision

**`packages/design-system/tokens/tokens.json` is the single origin for every colour, spacing,
radius, duration and easing value in this product.** It is W3C DTCG (`$type`/`$value`), so it is a
standard format rather than a private one, and it is read natively by Style Dictionary v4, which
does the generation.

Three artefacts are generated from it, and nothing else defines a value:

| Target | What it is |
|---|---|
| `dist/tokens.css` | CSS custom properties. Primitives on `:root`; the semantic layer scoped to `[data-theme="light"]` and `[data-theme="dark"]`, matching the two modes the source declares |
| `dist/tokens.ts` | Typed constants, nested, camelCase, for TypeScript consumers |
| a Go constant list | **Only the names of the ten label tokens**, plus a validation helper |

From which follows the rule that makes it worth anything: **no hex value, no pixel count and no
duration is written anywhere else.** Somebody who needs a value that does not exist adds it to
`tokens.json` — or does not need it. A CI check regenerates all three targets and fails on any
difference from what is committed.

**Why a Go artefact at all, and why only names.** The core has to validate a `colorToken`; the
frontend has to render it. If both are to mean the same thing, both must come from the same file.
But the core must stay colour-blind — a hex value in `core/domain` would be display information in
the backend, which rule 8 of CLAUDE.md forbids for text and which is no better as a colour. So the
generator emits the **vocabulary and never the values**: ten constants and a membership check, no
`#8A2438` anywhere in it. Backend and frontend then share one list of names while the backend
remains unable to say what any of them looks like — which is exactly the split `domain-model.md`
asked for when it chose to store a token instead of a colour.

**Where the Go file goes.** `project-structure.md` §1 already names
`core/domain/model/shared/ColorToken.go` as the home of colour-token handling, and validating a
label colour is domain validation, so that is where it belongs — not in an adapter, and not under
`packages/`, which ADR-0027 rule 4 keeps out of the Go build entirely. The generated file is
therefore written **into the core** by the design system's build and **committed** there. It is one
of exactly two generated files this project commits on purpose; the other is ADR-0028's UI
placeholder. Both are committed for the same reason: `go build ./...` must succeed in a checkout
where Node has never been installed.

Concretely it is `core/domain/model/shared/LabelTokens.go`, written there by Style Dictionary
directly rather than produced in `dist/` and copied. A copy step would put the same content in two
places, which is the failure mode this whole ADR exists to prevent, and it would leave a `.go` file
under `packages/` that `go build ./...` compiles while ADR-0027 rule 4 says nothing there is
importable — a contradiction the file system would have to be trusted to keep. `dist/` therefore
holds only `tokens.css` and `tokens.ts` and is ignored in its entirety. The name follows this
project's PascalCase convention rather than the lower-case spelling `design-system.md` §1 uses for
the Style Dictionary target: the convention of the tree a file lands in wins over the convention of
the tree it came from.

Committing it makes it hand-editable, so two safeguards stand in for the compiler. It carries the
`// Code generated … DO NOT EDIT.` line Go tooling recognises, and CI regenerates it and fails on
any difference. Generation is a separate `make` target from `make generate`, which must keep
working for a contributor who has no Node.js — a `go:generate` directive here would put a
JavaScript toolchain in the path of `make gate-quick`.

**Typography is self-hosted.** IBM Plex Sans, Sans Condensed and Mono (OFL-1.1) ship as files in the
repository and are recorded in `THIRD-PARTY-LICENSES.md`. Not for convenience: a self-hosted
Hubtask must not contact a foreign domain when it loads, which
[ADR-0018](./ADR-0018-privacy-by-design.md) requires of every other request and which
[ADR-0028](./ADR-0028-embedded-web-ui.md)'s `font-src 'self'` enforces at the browser.

**The component layer stays empty.** `packages/design-system/src/` contains a README and nothing
else until the frontend framework is decided in its own ADR. Tokens and the CSS layer are
framework-independent and can exist now; components cannot, and a component layer built before the
framework decision is rebuilt at the first contradiction. `reference/foundations.html` is the
visual acceptance reference in the meantime — it imports the generated `tokens.css` and declares no
values of its own, because a style guide with its own copy of the colours is itself a source of
drift. Storybook replaces it once there is a framework.

## Consequences

* Adding or changing a value is an edit to `tokens.json` and a regeneration — never an edit to a
  consumer. A pull request that changes a colour shows one line in a diff.
* The core gains one generated file. It imports nothing, so rule 1 of CLAUDE.md is untouched: it is
  a list of strings and a function over them.
* `project-structure.md` §3 lists three locations for generated code; this adds a fourth, and it is
  the first one inside `core/`. The rule that generated code is never hand-edited applies to it
  unchanged, and here it is enforced by a gate rather than by review.
* A drift between the design system and the domain becomes a failing check instead of a rendering
  bug found by a user.
* An eleventh label colour is a change in one file with a regenerated consequence in the backend —
  which is the point, and which also means adding one is a visible API-affecting change rather than
  a quiet frontend tweak.
* Style Dictionary becomes a build dependency of the design system package. It is a build-time
  dependency only; nothing it produces carries a runtime dependency into either client.

## Notes

Related: [ADR-0027](./ADR-0027-monorepo-structure.md) (where `packages/design-system` lives and why
nothing under it is importable from Go), [ADR-0028](./ADR-0028-embedded-web-ui.md) (the CSP that
requires self-hosted fonts, and the other deliberately committed generated file),
[ADR-0018](./ADR-0018-privacy-by-design.md) (no foreign origin on load),
[ADR-0011](./ADR-0011-i18n-message-codes.md) (the same argument for text that this makes for
colour: the backend holds a code, never a rendering),
[domain-model.md](../architecture/domain-model.md) §4 (`colorToken` on `Label` and `cover`),
[project-structure.md](../architecture/project-structure.md) §1 and §3,
`docs/design/design-system.md` (the specification this ADR makes binding).
The frontend framework is **not** decided here and needs its own ADR; it blocks the component layer
and nothing else.
