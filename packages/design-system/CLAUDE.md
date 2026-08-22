# packages/design-system — one origin for every value

`tokens/tokens.json` is the **only** place a colour, spacing, radius, duration or easing value is
defined in this product. Everything else here is generated from it
([ADR-0029](../../docs/adr/ADR-0029-design-system-tokens.md)). The specification this implements
is [`docs/design/design-system.md`](../../docs/design/design-system.md), and it is binding.

## What must not happen here

* **No second place for a value.** A design system drifts at exactly the point where the same
  number is written twice. `pnpm lint` fails on a colour written outside `tokens.json`, and on a
  bare length or duration in application code.
* **No component.** `src/` stays empty until the frontend framework ADR exists. A component layer
  built before that decision is rebuilt at the first contradiction.
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
make tokens                                  # or: pnpm --filter @hubtask/design-system build
pnpm --filter @hubtask/design-system lint    # no value outside tokens.json
pnpm --filter @hubtask/design-system test    # the properties ADR-0029 rests on
git diff --exit-code core/domain/model/shared/LabelTokens.go   # must be empty after committing
```

`reference/foundations.html` is the **visual acceptance reference**. Open it after any token
change and check both modes. It imports the generated stylesheet and declares no value of its own —
if you find yourself adding one there, that is the bug.
