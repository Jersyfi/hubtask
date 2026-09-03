# apps/webapp — the to-do application

The product UI, in the browser. Not the website: `apps/website` is hubtask.eu, an information site
with no task management in it. The names matter because `apps/web` would not say which.

This bundle is embedded into the Go binary and served at `/` by `presentation/webui`
([ADR-0028](../../docs/adr/ADR-0028-embedded-web-ui.md)), so the API and the interface always come
from the same commit and cannot be a version apart.

## What must not happen here

* **The framework is decided, and it is not SvelteKit here.** This app is Svelte 5 (runes,
  TypeScript) as a plain Vite SPA
  ([ADR-0030](../../docs/adr/ADR-0030-svelte-frontend-framework.md)) — SvelteKit's inline
  bootstrap script cannot pass the CSP below. The two standing constraints are unchanged: the
  content security policy permits neither `'unsafe-inline'` nor `'unsafe-eval'` (the built
  bundle must contain no inline script or style — `pnpm build` runs `build/check-csp.js` and
  fails on one), and every value comes from the design system.
  This one codebase is also what the Tauri shells wrap
  ([ADR-0031](../../docs/adr/ADR-0031-tauri-app-shell.md)); platform-specific code lives only
  behind the `src/lib/platform/` seam
  ([ADR-0033](../../docs/adr/ADR-0033-shared-client-architecture.md)).
* **No colour, spacing, radius or duration written here.** They come from
  `@hubtask/design-system`; a value that does not exist is added to `tokens.json` or is not
  needed. `pnpm lint` fails on one ([ADR-0029](../../docs/adr/ADR-0029-design-system-tokens.md)).
  In CSS that means `var(--…)`. Where a value is needed **in script**, it comes through
  `tokens.ts` as a custom-property reference, never as a literal:

  ```ts
  import { tokens } from '@hubtask/design-system';
  element.style.color = tokens.text.primary; // 'var(--text-primary)' — resolves per theme
  ```

  `values.light`/`values.dark` (the resolved literals) are for surfaces a custom property
  cannot reach — a canvas, an exported image — and nothing else: a component that writes the
  light-mode colour is wrong in dark mode, and no type can catch it.
* **The theme is an attribute, set in one place.** `src/lib/theme.ts` puts `data-theme` on the
  document, following the system preference until the account preference exists; the generated
  stylesheet has no `:root` fallback on purpose, so a document without the attribute looks
  broken at once. Components never read or set the theme themselves.
* **No sentence in a component.** The server delivers a code and parameters, never display text
  (ADR-0011, [`i18n-l10n.md`](../../docs/architecture/i18n-l10n.md) §1), and the client is the half
  that turns the pair into words: `src/lib/i18n/` holds the ICU renderer, the locale resolution of
  §2 and the source-language fallback of §3. A component calls `t('code', params)` and writes no
  English of its own — including the application's own strings, which are codes under `app.*` in
  the same `locales/en.json` the binary embeds. **A second catalogue under `apps/` is not an
  option**; `src/lib/i18n/catalogue.ts` is the one place that reads the file, and the workspace lint
  carries that single exception (`project-structure.md` §2.1). Anything the renderer cannot render
  is refused by name and `catalogue.test.ts` parses every message, so a construct nobody
  implemented turns the build red rather than printing braces at a reader.
* **No hand-written API type.** They come from `@hubtask/api-client`, generated from
  `api/openapi.yaml`. If the type you need is not there, change the specification (ADR-0004).
* **No request to a foreign origin.** `connect-src 'self'`, and the fonts ship with the bundle. A
  self-hosted Hubtask contacts nobody on load (ADR-0018).
* **No dependency on `apps/website`.** The two clients share packages, never each other.
* **Routing is the in-house module `src/lib/router.ts`** — real paths over the History API,
  never `#/` (ADR-0028's `index.html` fallback exists so deep links survive a reload). Routes
  join its table in `App.svelte`; a router library would be a new dependency and therefore a
  proposal, not a commit (CLAUDE.md, the W-06 pull request records the reasoning).

* **The frame decides once, and views consume it.** `src/lib/frame/` holds the shell every view
  sits inside; beside it, four modules hold what the application knows about itself and may not
  answer twice:
  `lib/data/capabilities.svelte.ts` reads `/meta/capabilities` **once at boot** — nothing may
  hard-code what the manifest answers; `lib/data/account.svelte.ts` reads `GET /accounts/me` when
  there is a bearer, and its `locale` outranks the browser's (`i18n-l10n.md` §2);
  `lib/data/health.svelte.ts` reads `/meta/health` **only where the actor may read it** — no
  bearer means no request, a `401` or `403` is silence rather than a message, and there is no
  second unauthenticated health surface; and `lib/maturity.ts` carries the stage of
  [ADR-0035](../../docs/adr/ADR-0035-one-product-version.md) §2, which is what the banner reads and
  what convergence changes by changing one line. The language is applied in exactly one place, the
  frame, for the reason the theme is: an attribute two modules set is an attribute nobody owns.
* **The session is a token, it is temporary, and `0.6.0` is what replaces it.**
  `api/openapi.yaml` declares one security scheme — a bearer that may be an OIDC access token, an
  `hbt_pat_…` or a service account token — and no login route, no session endpoint, no redirect
  flow. So `lib/session.svelte.ts` asks for a token, verifies it by reading `GET /accounts/me` with
  it, and hands it to `platform.holdBearer`; `lib/platform/tokenStore.ts` keeps it in
  `sessionStorage`, which survives a reload and dies with the tab. **Do not check the token's
  shape**: `security.md`'s pattern describes one of the three credentials that scheme accepts, and
  a client enforcing it would refuse the other two. **The token is a secret**: never in a URL,
  never in a log, never written into a message, never in the DOM beyond the password field that
  accepts it. Sign-out calls `engine.reset()` as well as `releaseBearer()` — `offline-sync.md` §9.6
  applies from the first day there is anything to discard. A `401` from any request ends the
  session through the engine's one hook, remembering the path first.
* **A failure becomes a sentence in `lib/problem.ts`, never in a component.** A `TransportError`
  carries the whole problem document (ADR-0025); that module chooses between `code` and the more
  specific `detail_code`, puts each `field_errors[]` entry under its own `path`, and hands back the
  `request_id` where the sentence does not already carry it. A component that read `error.code`
  itself would be a second place where an English sentence could appear.

## Two things the API decides for you

* **Cursor pagination, never page numbers** — the API has none, so no component may imply them.
* **`/meta/capabilities` is what the client configures itself from**, including which fields may be
  filtered. A hard-coded list will be wrong on somebody's installation.

## How to check a change

```bash
pnpm --filter @hubtask/webapp build
pnpm --filter @hubtask/webapp lint       # no literal values
pnpm --filter @hubtask/webapp typecheck
pnpm --filter @hubtask/webapp test
```

Nothing here is importable from Go, and no `.go` file belongs in this directory.
