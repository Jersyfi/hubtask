# ADR-0030 — Svelte 5 for every first-party client, and the webapp as a plain Vite SPA

**Status:** accepted · **Date:** 2026-08-23

## Context

C-14 (arc42 §2.2) has held the frontend open since the beginning. Three accepted decisions have
since fenced in what any framework must satisfy, and together they make the choice far narrower
than a general "React or Vue or Svelte" debate:

* [ADR-0028](./ADR-0028-embedded-web-ui.md): the webapp is a **static build artefact**, embedded
  into the Go binary and served by `presentation/webui`. There is no Node process at runtime, so
  server-side rendering, server routes and form actions are not merely unused — they are
  impossible. The same ADR fixes the content security policy **before** the framework choice:
  `script-src 'self'`, `style-src 'self'`, no `'unsafe-inline'`, no `'unsafe-eval'`. A framework
  that needs an inline script or `eval` to boot does not meet the bar.
* [ADR-0029](./ADR-0029-design-system-tokens.md): every value comes from `tokens.json`; the
  component layer in `packages/design-system/src/` is deliberately empty and blocked on exactly
  this decision.
* [ADR-0021](./ADR-0021-offline-sync.md) and [offline-sync.md](../architecture/offline-sync.md)
  §9: the client keeps a local copy and a mutation queue. That logic is deliberately **not** a
  framework concern — [ADR-0033](./ADR-0033-shared-client-architecture.md) puts it in a
  framework-agnostic package — but the framework must bind to an external store without friction.
* [ADR-0011](./ADR-0011-i18n-message-codes.md): the client renders all text from message codes,
  including RTL locales; `docs/design/design-system.md` §3 already forbids `left`/`right`
  alignment.
* `docs/design/design-system.md` §6 confines motion to `opacity` and `transform`, and the
  "Rewarding interactions" principle (its §7) needs first-class, cheap micro-animations.

One more constraint is cultural rather than technical. This repository is built AI-assisted and
maintained by one person; the backend picked Go and `net/http` over larger frameworks for exactly
that reason. The frontend equivalent is a framework with explicit conventions, little runtime
magic, and small surface — modern and lean rather than maximal.

The framework question and the *application frame* question are separate, and conflating them is
how projects end up with a Node server they never wanted. This ADR answers both: which framework,
and whether the webapp uses SvelteKit or Svelte with a lightweight router.

## Options

**A. React.** The largest ecosystem and talent pool. Rejected: a virtual-DOM runtime and
reconciler shipped to every client for an app that is mostly forms and lists; implicit reactivity
rules (hook order, dependency arrays) that are a standing source of subtle bugs — the opposite of
"little magic"; and an ecosystem that churns through state-management idioms faster than this
product will ship releases. The philosophy mismatch is the same one that ruled out a JVM framework
on the backend.

**B. Vue 3.** Technically capable, smaller than React, good reactivity. Not chosen: two component
authoring styles (Options/Composition) mean every example and every assistant suggestion comes in
two dialects, and its single-file-component tooling brings more build-time machinery than Svelte
for no gain this product can use.

**C. Solid, Preact, or a vanilla/Web-Components approach.** Solid is the closest technical
cousin (compiled, fine-grained reactivity) but has a far smaller ecosystem and no answer to
Svelte's built-in transition primitives. Vanilla/Web Components would make every list, dialog and
form a hand-rolled project of its own. Rejected.

**D. Svelte 5, with SvelteKit as the application frame for the webapp.** Rejected **for the
embedded webapp** — the framework survives, the frame does not. SvelteKit's value lives on the
server: SSR, server routes, form actions, per-request data loading. ADR-0028 removes the server at
runtime, so all of that machinery would idle. Worse, it would idle while violating the CSP: even
in static/SPA mode (`adapter-static` with a fallback page), SvelteKit boots through an **injected
inline `<script>`**, which `script-src 'self'` blocks. SvelteKit's own answer is nonces or hashes
(`kit.csp`) — a nonce requires a server that rewrites the document per request, which the Go
adapter deliberately is not, and a build-time hash would couple `presentation/rest/Security.go` to
every frontend build output. Bending the accepted CSP to fit a convenience frame would be the tail
wagging the dog.

**E. Svelte 5 with Vite and a lightweight client-side router (chosen for the webapp).**

## Decision

**Svelte 5 (runes) with TypeScript is the frontend framework for every first-party client**: the
webapp, the website, and the component layer in `packages/design-system/src/` that ADR-0029 has
been holding open.

What decides it, in the order that matters here:

1. **The compiler approach fits the delivery model.** Svelte compiles components to imperative DOM
   code at build time; there is no virtual-DOM runtime to ship or to trust. The output is exactly
   what ADR-0028 wants to embed: a small pile of static, content-hashed files. Component styles
   compile to an **external** stylesheet, so `style-src 'self'` holds without exceptions.
2. **Runes make reactivity explicit.** `$state`, `$derived`, `$effect` and `$props` are one-word,
   greppable annotations with none of the hook-ordering or dependency-array folklore. For an
   AI-assisted codebase, reactivity that is visible in the source is worth more than familiarity.
3. **Motion is built in.** Svelte's transition and animation primitives operate on `opacity` and
   `transform`, honour `prefers-reduced-motion` patterns cleanly, and cost nothing when unused —
   precisely what the design system's motion rules and celebration slots (design-system.md §6–§7)
   need without pulling in an animation library.
4. **Small bundles.** The to-do app must feel instant on a self-hosted Raspberry Pi serving it and
   on the phone loading it. Svelte consistently produces the smallest output of the mainstream
   frameworks, and the embedded bundle inflates the Go binary byte for byte (ADR-0028).

**The webapp is a plain Vite single-page application — Svelte without SvelteKit.** Vite's
`index.html` references only external module scripts, so the accepted CSP holds with no nonce, no
hash, and no exception. Routing is client-side over the History API — real paths, because
ADR-0028's fallback rule exists precisely so that deep links survive a reload; hash routing would
waste it. Whether the router is a small library or a minimal in-house module is a dependency
decision taken in the scaffold task under CLAUDE.md's dependency rule ("every dependency is a
supply chain decision"), not silently here. SvelteKit remains available to revisit if the inline
bootstrap becomes externalisable; the framework choice does not change either way.

**The website uses SvelteKit with `adapter-static`, fully prerendered.** The website is the one
place SvelteKit's frame pays for itself at build time — many mostly-static pages, filesystem
routing, prerendering for SEO — while still producing plain files with no runtime Node
(ADR-0028 already keeps it out of the binary; its CSP is its host's decision, not
`Security.go`'s). Same framework, different frame, each matching its delivery.

**Svelte 5 is pinned as the major.** Version currency at the time of writing: Svelte 5.5x /
SvelteKit 2.5x (see References). Upgrades within the major are routine maintenance; a future
Svelte 6 migration is ordinary dependency work, not a reopening of this decision.

## Consequences

* The component layer in `packages/design-system/src/` is unblocked; components are Svelte
  components consuming tokens only. `reference/foundations.html` is eventually replaced by a
  component workbench, as ADR-0029 foresaw — choosing that tool is part of the component-layer
  work, not this ADR.
* The webapp scaffold is deliberately thin: Vite, Svelte 5, TypeScript, the design-system
  stylesheet, `@hubtask/api-client`, a router. CI must prove the CSP promise mechanically — a
  gate that fails on any inline `<script>` or `<style>` in the built bundle, so the constraint
  ADR-0028 placed before the framework stays enforced after it.
* Svelte's ecosystem and hiring pool are smaller than React's. Accepted knowingly: this is the
  same trade the backend made with Go, and the mitigation is the same — boring, explicit code
  over ecosystem breadth. One maintainer plus an assistant is the staffing reality this optimises
  for.
* Two application frames exist (Vite SPA for the webapp, SvelteKit prerender for the website).
  That is two configs, not two frameworks; the alternative — one frame forced onto both delivery
  models — was rejected above for the webapp and would be equally wrong inverted.
* `apps/webapp/CLAUDE.md` and `apps/website/CLAUDE.md` currently say "no framework decision";
  both are updated when this ADR is accepted, as is arc42 §2.2 C-14 (the constraint narrows: the
  *framework* is decided, the feature set follows [ADR-0032](./ADR-0032-client-capability-matrix.md)).

### Backlog impact

| Work package | Target |
|---|---|
| Webapp scaffold: Vite + Svelte 5 + TypeScript + router, CSP conformance check in CI | current workspace milestone |
| Design-system token wiring in Svelte (stylesheet import, lint already enforces the rest) | current workspace milestone |
| Embedded build pipeline: `ui` Docker stage produces the real bundle into `presentation/webui/dist` (ADR-0028) | current workspace milestone |
| Design-system component layer, wave 1, plus the component workbench decision | frontend track (roadmap phase 5) |
| Website scaffold: SvelteKit + `adapter-static`, prerendered | frontend track (roadmap phase 5) |

## References

* Svelte 5 runes — https://svelte.dev/docs/svelte/what-are-runes
* SvelteKit single-page apps / `adapter-static` — https://svelte.dev/docs/kit/single-page-apps
* SvelteKit CSP configuration (nonce/hash mechanism) — https://svelte.dev/docs/kit/configuration#csp
* CSP blocks SvelteKit's injected inline script — https://github.com/sveltejs/kit/issues/15019
* Version currency (Svelte 5.55 / SvelteKit 2.57, 2026) — https://stacknotice.com/blog/sveltekit-complete-guide-2026

## Notes

Related: [ADR-0028](./ADR-0028-embedded-web-ui.md) (the delivery model and the CSP this choice had
to fit), [ADR-0029](./ADR-0029-design-system-tokens.md) (the component layer this unblocks),
[ADR-0027](./ADR-0027-monorepo-structure.md) (where the code lives),
[ADR-0031](./ADR-0031-tauri-app-shell.md) (the shells that wrap the same codebase),
[ADR-0033](./ADR-0033-shared-client-architecture.md) (how one codebase serves three targets),
arc42 §2.2 C-14 (the constraint this closes half of).
