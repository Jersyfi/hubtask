# The 1.0 Website — content map, design mechanisms, and the claim ledger

Status: **draft on the branch, not deployed** · Written 2026-09-09 · Companion to
[`market-analysis.md`](./market-analysis.md)

> [`roadmap.md`](../roadmap.md) puts the 1.0 website in the convergence milestone `0.9.5`, and the
> repository stands at `0.7.0`. This site is therefore written **as the 1.0 launch fassung, in the
> present tense, and is not published yet.** § 5 below is what makes that safe: every sentence on
> the site that is not true today is listed there with the milestone that makes it true. Nothing
> goes live until its row can be ticked.
>
> Until then, `main` keeps the pre-release page. The rule that page was built under — F1-12's "only
> what is already true" — is not being relaxed; it is being scheduled.

---

## 1. The site map

Ten content pages plus the two legal ones. The order is the priority order of the audiences in
[`market-analysis.md`](./market-analysis.md) § 3.1.

| Path | Purpose | Written for | The one thing it has to land |
|---|---|---|---|
| `/` | The whole proposition in one scroll | All four | “Built like infrastructure. Shaped like a to-do list.” |
| `/product/` | Every capability, at the level it lives at | 1, 2 | The capability matrix — five levels are one model, not five entities |
| `/use-cases/` | The four readers, each with their own anchor | 1, 2, 3, 4 | You are one of these people, and here is your paragraph |
| `/security/` | Isolation, the trail, data protection, the gates | 3, 2 | Every claim links to the mechanism |
| `/developers/` | REST, MCP, CLI, events, SDKs, sync | 4 | The API is the product, and the agent runs on your box |
| `/self-hosting/` | Requirements, support matrix, backup, operations | 1, 2 | PostgreSQL, and that is the list |
| `/licence/` | BSL, who pays, the Change Date, editions | 1, 2 | Free for you; the guarantee is per version and irrevocable |
| `/roadmap/` | The milestones, and the working method | 4, 3 | Why the guarantees elsewhere are checkable |
| `/download/` | Server, clients, CLI | 1, 2 | Nothing to sign up for |
| `/accessibility/` | The statement the EAA expects | 3 | Operable by keyboard, or it is a defect |
| `/impressum/`, `/datenschutz/` | § 5 DDG, Art. 13 GDPR | — | German, `noindex`, unchanged in substance |

Navigation carries six of them; `/roadmap/`, `/download/` and `/accessibility/` are reached from
the footer and from the pages that earn them. The masthead is deliberately not a mega-menu: there
is no JavaScript to open one with.

---

## 2. The design mechanisms — what makes it recognisable

The brief asked for recognition value from the design system. `tokens.json` gives colours and
spacing; it does not by itself give a *look*. Four mechanisms do, and they repeat on every page so
that a screenshot of any one of them is identifiably Hubtask.

**0 · The colour-mode glyph.** Not a brand mechanism as such, but the one piece of the site that
moves, and the first thing a returning visitor touches. One glyph shows the mode; pressing it
advances to the next. Its three faces were picked for their parts — `sun` is a disc plus eight
separate rays, `moon-star` a crescent plus two star strokes, `sun-moon` a crescent inside a sun —
so a change of mode is rays coming out or a star twinkling in after the moon, rather than one
picture swapping for another. Hovering lights a halo in that mode's own colour: ember behind the
sun, blue behind the moon.

**1 · The mark.** Three nested planes, the innermost in the signature bordeaux — the sketch that
lives in the workbench's `Foundations/Tokens` story. It is in the masthead, in the footer, and it is
the favicon. See § 6: it is still the placeholder, not a finished wordmark.

**2 · The ember, in exactly three places.** The signature colour is the scarcest thing on the site,
which is what makes it read as a signature rather than as a palette:

* an **ambient wash** behind a hero (`--ambient-signature`, a gradient, never a surface),
* the **mono uppercase kicker** above every section title,
* a short **rule under the page title**, and the rim on the cards of the section the product cares
  most about.

`design-system.md` is explicit that ambient tokens are washes and `accent.signature` is one accent.
A bordeaux button or a bordeaux panel would break the system, and the site has neither.

**3 · The level strip.** Hub / Collection / Task / Work package / Activity, always in the same five
label tokens, always in that order, as mono chips. It is the product's own colour vocabulary
explaining the product, and it is the motif a reader will remember.

**4 · The proof chip.** An ember-rimmed mono pill beside a claim, linking to the document, the
decision record or the CI gate behind it. Forty-seven of them across the site. This is the site's
argument made structural — see [`market-analysis.md`](./market-analysis.md) § 4.1 — and it is the
thing no competitor's homepage carries.

**5 · The product, drawn with the product — deferred.** The site currently shows no product
interface at all, and that is a known gap rather than an oversight: a specimen of a collection in
its list and board layouts was built from the real `ListRow`, `Checkbox`, `LabelChip`,
`BucketColumn` and `WorkItemCard`, and was taken out again to be added deliberately later. What it
proved is worth keeping for whoever rebuilds it: the components render and behave from markup and
`:has()` alone, so ticking an entry off, collapsing a work package and switching layout all work
with no script — and a screenshot would have gone stale the day a token moved, where this could
not. What it cannot do is drag, and § 6.8 is the decision that blocks it.

Underneath those: IBM Plex Sans Condensed for display and Plex Mono for kickers, data and code, which
is what gives the pages their engineered voice; and `design-system.md` §6 rule 1 applied literally —
**raised** cards stand alone, **sunken** blocks sit inside a section, and nothing is glass, because
only overlays blur and a brochure has none.

---

## 3. How it is built

| Property | How | Why it matters |
|---|---|---|
| **No JavaScript at all** | `csr = false`, everything prerendered; `build/check-static.js` fails the build on a `<script>`, a `<style>` element, a `style` attribute or an inline handler | The product's pitch is that your data stays yours. A site with a tracker on it would lose the argument in the first second |
| **No value outside `tokens.json`** | The design system's own lint runs over this app | One origin, and the site cannot drift from the product |
| **Light and dark, and a manual switch** | The document is `data-theme="dark"`, the body carries `data-theme="light"`, and `src/site.css` neutralises the light set unless the reader or their system asks for it. Three radio buttons, no script | Verified in all six combinations of system preference and reader choice |
| **The colour mode is one glyph that cycles** | `sun-moon`, `sun` and `moon-star`; the three faces are stacked and only the current one is opaque and clickable, and its `for` names the *next* radio | Chosen by the owner over a three-target control. § 6.7 records what that costs and what pays for it |
| **A guard for that construction** | `build/check-theme.test.js` fails when the stylesheet reads a semantic token either colour-mode block forgets | The one way the construction could rot, closed. It has been seen to fail |
| **Fluid, with no width breakpoint** | Every measure is a token or a `ch`; the type scales with `clamp()` | A media query cannot read a custom property, so the alternative would have been literals |
| **Verified** | No horizontal overflow and no over-long measure at 320, 375, 768, 1024 and 1440; one `h1` per page; every page has a title and a description | Measured, not eyeballed |
| **Findable** | `robots.txt`, `sitemap.xml`, per-page titles and descriptions, an SVG favicon, the legal pages `noindex` | It had none of these |

---

## 4. What the site deliberately does not do

No newsletter, no waiting list, no contact form, no analytics, no cookies, no embedded video, no
external font, no social widget. Each would need a data-catalogue entry with a legal basis and a
deletion path, and the first three would need consent. The reasoning is
[`market-analysis.md`](./market-analysis.md) § 6, and the cost is accepted there: there is no way to
reach a visitor who was not ready.

It also names **no price** and **no date**. See § 5 and the licence page.

---

## 5. The claim ledger

**The rule: a row that is not ticked is a sentence that must be cut or rewritten before the page it
sits on goes live.** Everything not listed here is true as of 2026-09-09 — milestones `0.1.0`
through `0.6.0` complete, `0.7.0` at fourteen of seventeen tasks, client milestones F1 to F3
complete.

### 5.1 Not true yet — the site is written as though they are

| Page | The claim | True from | Note |
|---|---|---|---|
| `/download/` | Signed installers for Windows, macOS and Linux, with an updater | **F6** | The desktop shell is not built. The whole “Desktop” row of the client table |
| `/download/` | iOS and Android applications, and the admin-capability affordance | **F6** | Same. Store listings are submitted at `0.9.5` |
| `/download/` | “Every release publishes a signature and a software bill of materials” | **1.0** | The pipeline does it; the only published release today is `v0.2.0-rc.1`. The mechanism is real, the plural is not |
| `/product/`, `/use-cases/` | Installed clients work fully offline; the browser app keeps a best-effort cache | **0.8.5 / F6** | Per-field merging is specified and unbuilt |
| `/developers/` | `hubctl sync-conformance` checks an implementation | **0.8.5** | The contract it checks is written; the command is not |
| `/developers/`, `/product/` | Client SDKs in TypeScript, Go and Python; official n8n and Zapier nodes | **0.9.0** | `packages/api-client` exists; the published SDKs do not |
| `/product/` | “any language” — catalogue maintenance, CLDR formats, localised e-mail | **0.8.0** | Message codes end to end are true today; the full localisation surface is not |
| `/accessibility/` | The entire statement, and WCAG 2.2 AA demonstrated | **F5 / 1.0** | Contrast measured in CI and the focus rules are true today. The *statement* is a 1.0 deliverable (prerequisite 16) |
| `/developers/` | “API v1 is stable from the first stable release” | **1.0** | Correctly written as a promise about 1.0. Verify the deprecation process has actually been exercised before publishing |
| `/product/` | AI summaries (comment thread, collection status, weekly review) | **verify** | `ai-first.md` §2 lists it; unlike the jumble suggestion and hybrid search it carries no “shipped in J-xx” note. Confirm against J-08 before publishing, or cut the word “summaries” |

### 5.2 Checked, and true today

Worth recording, because several read as though they could not be: the capability matrix and the
refusal behaviour; three-door parity with the CI check; the MCP server including resources, prompts
and the session stream (J-11 to J-13); hybrid semantic search with pgvector detected rather than
demanded (J-09, J-10); the four built backup targets and the CI restore drill; the audit hash chain,
its verification and the missing `UPDATE`/`DELETE` grant; retention preview, grace period and legal
hold; data subject requests with deadline tracking; row level security with a cross-tenant test per
repository method; the twelve security gates and the self-test that proves they bite; the eight
service level objectives; the support matrix rows; and every statement on `/licence/`.

### 5.3 Deliberately absent

No price, no date for a price, no load-test figures (they stay internal by decision), no claim of
“open source”, no comparison table naming a competitor, and no promise that free private use is
permanent without the condition `licensing-editions.md` §5 attaches to it.

---

## 6. Open, and needing the owner

1. **The wordmark.** `design-system.md` §9 still lists it as unfinished. The site uses the
   workbench's three-nested-planes placeholder in the masthead, the footer and the favicon. That is
   the right placeholder — it is the only drawn record of the idea — but a launch is the moment it
   stops being one.
2. **Which mailbox.** The site uses `info@hubtask.eu` everywhere, including for licence enquiries.
   [`licensing-editions.md`](../architecture/licensing-editions.md) §2 names `licensing@hubtask.eu`.
   Either create that mailbox and split the two, or amend the document.
3. **The colour mode does not persist across a navigation.** CSS cannot write storage, and the site
   loads no script. A reader who picks Light gets dark again on the next page unless their system
   preference agrees. The system preference — which is what most people actually have set — is
   honoured on every page, so this affects only the reader who overrides it. Persisting it costs
   about a dozen lines of JavaScript, and that is a deviation from F1-12's no-script promise and
   from `check-static.js`, so it needs a decision and probably an ADR rather than a commit.
4. **No social preview image.** A link shared to a chat or a timeline renders as bare text. An
   Open Graph image has to be a raster; the design system's `values.*` exist for exactly that kind
   of consumer, but generating it needs a tool this repository does not have yet.
5. **A German accessibility statement.** The BFSG may expect one in German for a German-operated
   service. The current page is English, as the product is.
6. **`200+` or the exact number.** The registry holds 211 registered descriptors today. The site
   rounds down, so the claim stays true as the number moves. Publishing the exact figure would be
   more impressive once and wrong later.
7. **The colour-mode control cycles, and here is what that costs.** A three-target segmented
   control was built first and the owner chose the single cycling glyph instead. The trade is worth
   having written down rather than rediscovered.

   *What it costs.* Three states render as two appearances — on a dark system `Auto` and `Dark` look
   identical — so exactly one press in three does not change the page, and no ordering of the cycle
   avoids that. Reaching a named mode takes up to two presses, and the two modes you are not in are
   not visible.

   *What pays for it.* The glyph itself is the feedback: it changes, and it changes with enough
   movement that the press is legible even when the canvas is not. That is why the animation is
   load-bearing here rather than decorative, and why it was worth the three icons chosen for their
   parts. The keyboard path is unaffected — `aria-label` on each input keeps a plain three-option
   radio group, so assistive technology and the arrow keys still reach any mode directly, and never
   see the cycle at all.

   *If it should be reverted*, the segmented control is in the branch's history at commit
   `2eec183`.
8. **The product surface, and what dragging would cost.** Showing the interface is deferred by
   decision, and two findings should travel with it rather than be rediscovered.

   *Only some components can appear.* `ListRow`, `Checkbox`, `LabelChip`, `BucketColumn`,
   `WorkItemCard` and `Icon` render their whole selves from markup and CSS. `ViewSwitcher`, `Menu`,
   `Tabs` and `Dialog` choose through an `onclick`, so on a page that loads no script their controls
   are furniture — a layout switcher has to be rebuilt from the same tokens, which is the one place
   a component's appearance would be written twice.

   *Dragging is a decision, not a task.* Reordering by pointer is JavaScript, and `csr = false` in
   `src/routes/+layout.ts`, F1-12's promise, and `build/check-static.js` are one decision expressed
   three times. The site's own argument leans on it: the page that says your data stays yours loads
   nothing and calls nobody. Allowing a script — scoped to that one component, with an ADR amending
   F1-12 and a narrower static check — is what would unlock both dragging and the four components
   above. Leaving it means the surface demonstrates the model, the layouts and completion, and
   dragging is shown in the product where it belongs.

9. **How much roadmap to show.** The `/roadmap/` page currently shows every milestone including the
   1.0 prerequisites. That is unusually candid for a product site and is, in this positioning, an
   asset — but it is a decision, and it is the page most likely to need trimming.

---

## 7. Publishing

`.github/workflows/website.yml` mirrors `dist/` to the IONOS webspace on every push to `main` that
touches `apps/website/**` or `packages/design-system/**`. So **merging this branch publishes it.**
That is the reason § 5 exists and the reason this is a draft: the merge is the launch.

```bash
pnpm --filter @hubtask/website build      # and build/check-static.js proves it is plain static
pnpm --filter @hubtask/website lint       # no value outside tokens.json
pnpm --filter @hubtask/website typecheck
pnpm --filter @hubtask/website test       # the static check and the colour-mode guard
```
