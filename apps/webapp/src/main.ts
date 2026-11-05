// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The entry point of the to-do application: Svelte 5 (runes) mounted as a plain Vite
 * single-page application (ADR-0030). Deliberately no SvelteKit — its inline bootstrap script
 * cannot pass the CSP fixed in ADR-0028, so this file and the external module script in
 * `index.html` are the whole boot sequence.
 *
 * What constrains everything mounted from here:
 *
 *   - The content security policy of ADR-0028 permits neither `'unsafe-inline'` nor
 *     `'unsafe-eval'`; the built bundle contains no inline script or style, and CI proves it.
 *   - Every value comes from `@hubtask/design-system`; no colour, spacing or duration is written
 *     here (ADR-0029).
 *   - Every type comes from `@hubtask/sync-engine`, which re-exports what `api/openapi.yaml`
 *     generated (ADR-0004). The application does not depend on `@hubtask/api-client` directly:
 *     the engine is the seam, and a component that imported the contract would be a component
 *     that could reach past it (ADR-0033 §2).
 *   - Platform-specific behaviour lives behind `src/lib/platform/`, never in a component
 *     (ADR-0033).
 *
 * The bundle this produces is embedded into the server binary and served from `/`.
 */

import '@hubtask/design-system/fonts.css';
import '@hubtask/design-system/fonts-scripts.css';
import '@hubtask/design-system/tokens.css';
import './app.css';

import { mount } from 'svelte';
import App from './App.svelte';
import { manifest } from './lib/data/capabilities.svelte.ts';
import { startLocale } from './lib/i18n/i18n.svelte.ts';
import { device } from './lib/device.svelte.ts';
import { t } from './lib/i18n/i18n.svelte.ts';
import { toast } from './lib/frame/toasts.svelte.ts';
import { currentEntry } from './lib/frame/chunkfailure.ts';
import { crashDetails, isNoise, newReference } from './lib/frame/crash.ts';
import { holdFragmentCredentials } from './lib/signin/fragmentToken.ts';

// Before anything can send a request: a credential a mail linked in the fragment leaves the address
// (UC-ID-07 check 2), and its screen takes it from where it is held.
holdFragmentCredentials();

// Before the first paint: the stylesheet deliberately renders nothing sensible without
// `data-theme` (ADR-0029), and this is the call that sets it - from what this device kept, else
// the system's preference (ADR-0043) - and `data-motion` beside it.
device.start();

// …and the document has to say what language it is in and which way it runs. At boot the browser's
// own preference is all the client knows; the account's and the installation's supported
// locales replace it through `messages.adopt`.
startLocale();

// …and the one read the whole application configures itself from. Started here rather
// than in a component, because what it answers changes the language of the first paint. The
// *route* needs no token - the manifest is the one unauthenticated route in the contract - but the
// *request* carries whatever bearer this tab still holds, and a stale one is answered `401` rather
// than anonymously. So this is the first read and not the only one: the session reads it again on
// every change of actor.
manifest.start();

const root = document.querySelector<HTMLDivElement>('#app');
if (!root) throw new Error('#app is missing from index.html');

// A failure outside drawing - a handler that threw, a promise nobody awaited - is told, with a
// reference and the details to copy, rather than swallowed. A drawing failure is the boundaries'
// in App.svelte. One message at a time: a failure repeating in a loop must not bury the screen.
let lastTold = 0;
function tell(error: unknown, message?: string): void {
  if (isNoise(error, message)) return;
  const now = Date.now();
  if (now - lastTold < 5_000) return;
  lastTold = now;
  const reference = newReference();
  const details = crashDetails({
    reference, build: currentEntry(), path: location.pathname, userAgent: navigator.userAgent, at: new Date(now), error,
  });
  toast.show({
    text: t('app.crash_unhandled', { reference }),
    tone: 'danger',
    action: { label: t('app.crash_copy'), run: () => navigator.clipboard.writeText(details) },
  });
}
window.addEventListener('unhandledrejection', (event) => tell(event.reason));
window.addEventListener('error', (event) => tell(event.error, event.message));

mount(App, { target: root });
