# ADR-0069 — A third-party brand mark is content, and the button stays ours

**Status:** proposed · **Date:** 2026-09-26

## Context

A workspace may sign in through its own identity provider (H-04), and the sign-in concept extends
that to several at once — an installation-wide Google beside a workspace's own Entra ID beside a
Keycloak somebody runs themselves. A column of buttons reading "Sign in with …" raises a question
this repository has an unusually strict answer to everywhere else: **where does the picture come
from, and what colour is it?**

Three rules collide:

1. **[ADR-0029](./ADR-0029-design-system-tokens.md):** no colour value exists outside
   `tokens.json`, and `pnpm lint` fails on one. A Google mark is four hex values.
2. **[ADR-0041](./ADR-0041-icon-set.md):** a mark in the icon set carries `currentColor` and `none`
   and nothing else, so that it takes the colour of the text it sits in. A brand mark recoloured
   to the button's text colour is no longer that brand's mark.
3. **[ADR-0018](./ADR-0018-privacy-by-design.md):** a self-hosted Hubtask contacts nobody on load,
   and `connect-src 'self'` enforces it. A logo fetched from a provider's CDN is out.

And a fourth thing that is not a rule of ours: Google, Microsoft and Apple each publish guidelines
for their sign-in button, each of which permits a **neutral form** — a plain surface, a hairline
border, the mark at the leading edge, the label beside it — and each of which forbids altering the
mark.

## Decision

### 1. The mark is theirs; the button is ours

The button is `Button` with `tone="secondary"`: `bg.surface`, `border.default`, the label centred,
and the provider's mark pinned to the start edge. That is our component in our tokens, and it is
simultaneously the neutral form all three guidelines permit.

**No brand colour becomes a surface.** No Google blue behind the label, no Apple black, no
Microsoft grey. Four providers in a column would otherwise be four brand colours arguing about
which matters most, on the one screen where the product should be arguing for itself.

### 2. A brand mark is content, not a token

`tokens.json` is the single source for every value *of ours*. A brand mark's colours are not ours:
they are data with an owner, like a picture somebody uploads. So they live in their own place, and
that place carries three obligations:

- **Each colour is written with the lint exemption and its reason** — the mechanism
  `lint-no-literals.js` already has for exactly this, one line, one justification.
- **Each mark is reproduced as its owner publishes it.** Never recoloured, never simplified to fit,
  never given `currentColor`. The one exception is a mark whose own guideline is monochrome —
  Apple's — which then takes the button's text colour *by its owner's rule*, not by ours.
- **Each mark has an entry in `THIRD-PARTY-LICENSES.md`** naming the source, the guideline and the
  permitted use, checked by the gate that already keeps that file honest.

### 3. Which marks ship is a legal question per mark, not a design question

Google, Microsoft and Apple publish guidelines that permit the sign-in button explicitly. Every
other mark — Okta, Auth0, Keycloak, Authentik, GitLab, Slack — is a separate reading of a separate
guideline, and a mark whose guideline does not clearly permit it **does not ship**. The fallback is
not a borrowed logo and not an empty space.

### 4. A provider without a brand gets a tile, and the tile is ours

A workspace's own Keycloak has no brand in this product. It carries a square tile with the first
character of the name it was given, in one of the ten label colours the core already validates —
the answer `Avatar` gives a person with no picture, and `LabelChip` gives a label. Square rather
than round, because it sits in a column with the brand marks and a round one would be the only
thing out of line. The colour is derived from the name, so the same provider always looks the same
and nobody has to choose one.

**No logo upload.** An SVG from a workspace owner is a script in the bundle; a PNG through `/media`
is a second path to the same place with its own cache and its own permissions. The tile is the
honest answer.

### 5. Where the mark lives: the application, until there is a second reader

`packages/design-system/src/brands/` was the concept's proposal. It is not where this lands, and
the reason is the project's own rule against speculative abstraction: the sign-in card and the
provider list in the administration are **one application**, and a design-system component needs a
story, a place in `design-system.md` §4's waves and a line in the inventory — three obligations for
a picture that one app draws. `apps/webapp/src/lib/signin/ProviderMark.svelte` is where it is, with
the exemptions and the reasoning in the file. It moves into the design system when a second client
draws one, which is `F7`'s shells or nothing.

## Options

1. **The neutral button, marks as content, a tile for the rest (chosen).**
2. **Each provider's own coloured button.** What most products do. It is four decisions about
   prominence made by four other companies, on our screen, and it makes the primary action the
   least prominent thing on the card.
3. **Our own icon for every provider** (a globe, a key). Honest about ownership, useless for
   recognition: the mark is the whole reason a person finds their own way in.
4. **Text only, no marks.** Defensible, and what the screen did before. It reads as unfinished
   beside every other sign-in screen a person uses, and recognition is the one thing this row of
   buttons is for.

## Consequences

**Positive.** One visual system on the card: our surface, our border, our type, their mark. The
lint rule keeps its teeth — every exception is a line with a reason rather than a directory nobody
checks. The licences file says what may be shown and why.

**Negative.** `THIRD-PARTY-LICENSES.md` grows rows that are about trademarks rather than about
code, which is a slightly different kind of entry in one file. A guideline can change, and nothing
tells us when; the marks are re-read when a milestone touches them, not continuously.

**What this does not decide.** Whether "Sign in with Apple" is *required* — App Store Review
Guideline 4.8 asks for it wherever an app offers another third-party sign-in, which is a question
for the shells in `F7` rather than for the web app, and the concept records it so the shells do not
discover it late.
