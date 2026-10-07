# apps/webapp — the to-do application

The product UI in the browser, embedded into the binary and wrapped by the shells. Not the website
(`apps/website`). How it is built is in [`README.md`](./README.md); what each screen draws is
[`design-system.md`](../../docs/design/design-system.md) §11.

## What must not happen here

* **No SvelteKit, and nothing the content security policy refuses.** Svelte 5 as a plain Vite
  single-page application. The policy permits neither `'unsafe-inline'` nor `'unsafe-eval'`, so the
  built bundle holds no inline script or style; `pnpm build` runs `build/check-csp.js` and fails
  on one.
* **No platform-specific code outside `src/lib/platform/`**, and no runtime sniffing: a shell's
  build selects its implementation.
* **No colour, spacing, radius or duration written here** (rule 15). In CSS it is `var(--…)`; in
  script it is `tokens` from `@hubtask/design-system`, which yields a custom-property reference.
  `values.light`/`values.dark` are for what a custom property cannot reach — a canvas, an exported
  image — because a literal colour is wrong in the other theme.
* **No second owner of a document attribute.** The theme (`lib/theme.ts`), reduced motion
  (`lib/motion.ts`) and the language (`App.svelte`) are each applied in exactly one place. A
  component never reads or sets them; a second module that set one would be a second answer to
  "which theme" or "which locale".
* **No device choice in the account, and no account choice in the device.** The theme, reduced
  motion and how a screen is laid out (a sheet's size) belong to this browser and live in
  `localStorage` — never in the replica, which is the account's copy and is deleted at sign-out.
  Language, time zone, week start, celebrations and the tour's completion are the account's.
* **No sentence in a component.** A component calls `t('code', params)`; the application's own
  strings are `app.*` codes in `locales/en.json`. No second catalogue: `src/lib/i18n/catalogue.ts`
  is the one reader of that file. A failure becomes words in `lib/problem.ts` only — a component
  that read `error.code` itself would be a second place a sentence could appear.
* **No `fetch`, no IndexedDB, no `EventSource`.** Every request goes through
  `@hubtask/sync-engine` (see its `AGENTS.md`); this app supplies paths, not transport.
* **No hand-written API type** — they come from `@hubtask/api-client`; change the contract (rule
  11).
* **Nothing hard-coded that `/meta/capabilities` answers.** It is read at boot and on every change
  of actor, never once per page. A type or role the manifest does not declare is refused, never
  permitted, and before the manifest arrives nothing is shown as available.
* **No control drawn for a capability that is refused outright.** It is absent, not disabled. A
  refusal the reader might otherwise have had — a permission, a setting, a degraded feature — is
  drawn through `CapabilityGate` with its reason, never hidden silently.
* **No page numbers.** The API answers a page and an opaque cursor; a longer list arrives through
  `LoadMore`. No client-side pager either, not even over a list that arrived whole — a long list is
  sorted or narrowed.
* **No credential out of its place** ([`identity.md`](../../docs/architecture/identity.md) §14.4).
  Never in a URL, a log, a message or the DOM beyond its field; one that arrives in the address is
  removed from history before the first request leaves. No shape check of a token — the security
  scheme accepts three kinds and a pattern would refuse two. The refresh exchange happens in
  `lib/data/engine.ts` only, never in a store: two exchanges at once sign somebody out.
* **No request to a foreign origin** (`connect-src 'self'`); fonts ship in the bundle.
* **No router library** — `src/lib/router.ts`, real paths, never `#/`. Any new dependency is a
  proposal, not a commit.
* **No import from `apps/website`**, and **no `.go` file** (rule 14).

## How to check a change

```bash
pnpm --filter @hubtask/webapp build       # includes the CSP check
pnpm --filter @hubtask/webapp lint        # no literal values, no physical direction
pnpm --filter @hubtask/webapp typecheck
pnpm --filter @hubtask/webapp test
```
