# packages/design-system — one origin for every value

`tokens/tokens.json` is the **only** place a colour, spacing, radius, duration or easing value is
defined in this product. Everything else here is generated from it
([ADR-0029](../../docs/adr/ADR-0029-design-system-tokens.md)). The specification this implements
is [`docs/design/design-system.md`](../../docs/design/design-system.md), and it is binding.

## What must not happen here

* **No second place for a value.** A design system drifts at exactly the point where the same
  number is written twice. `pnpm lint` fails on a colour written outside `tokens.json`, and on a
  bare length or duration in application code.
* **No component yet.** The framework is decided — Svelte 5
  ([ADR-0030](../../docs/adr/ADR-0030-svelte-frontend-framework.md)) — and there is now somewhere
  to look at one ([ADR-0037](../../docs/adr/ADR-0037-component-workbench.md)), but `src/` stays
  empty until the component-layer work package builds it deliberately, wave by wave per
  `design-system.md` §4. No component arrives here as a side effect of other work.
* **No component without a story.** A `<Name>.svelte` in `src/` needs a `<Name>.stories.ts` beside
  it and a place in one of `design-system.md` §4's waves; `pnpm test` fails otherwise. That is the
  design system's parity gate — the specification's inventory and the tree may not become two
  different lists.
* **No workbench in the product.** `workbench/` is a development tool. It is not part of
  `pnpm build`, `pnpm -r build` does not depend on it, and nothing of it may reach `apps/webapp`'s
  bundle or the binary that embeds it (ADR-0028).
* **No colour value in the Go output.** `LabelTokens.go` carries the ten *names* and nothing else,
  so the core keeps the vocabulary while staying colour-blind. A hex constant in `core/domain`
  would be display information in the backend.
* **No hand-edited `dist/`.** It is generated. So is
  `core/domain/model/shared/LabelTokens.go` — that one is committed, which makes it editable and
  therefore worth saying twice: **never edit it**, CI regenerates it and fails on a diff.
* **No font loaded from a foreign domain.** IBM Plex ships in the bundle. A self-hosted Hubtask
  must contact nobody on load.
* **No `:root` fallback for the semantic layer.** A document without `data-theme` should look
  broken at once rather than quietly pick a mode nobody chose.

## Adding a value

Edit `tokens.json`, run `make tokens`, commit the regenerated `LabelTokens.go` if the label set
changed. Never the other way round.

## How to check a change

```bash
make tokens                                       # or: pnpm --filter @hubtask/design-system build
pnpm --filter @hubtask/design-system lint         # no value outside tokens.json
pnpm --filter @hubtask/design-system test         # ADR-0029's properties, and the story gate
pnpm --filter @hubtask/design-system typecheck    # svelte-check over workbench/ and src/
make workbench                                    # look at it: :5174
git diff --exit-code core/domain/model/shared/LabelTokens.go   # must be empty after committing
```

Every axis of the workbench is in the query string. A pull request that says a component is right
in dark RTL at 200 % links to it — `?story=…&theme=dark&dir=rtl&zoom=200` — rather than asserting
it (ADR-0037).

`reference/foundations.html` is the **visual acceptance reference**. Open it after any token
change and check both modes. It imports the generated stylesheet and declares no value of its own —
if you find yourself adding one there, that is the bug.
