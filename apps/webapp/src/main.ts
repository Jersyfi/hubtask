// SPDX-License-Identifier: BUSL-1.1
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
import '@hubtask/design-system/tokens.css';
import './app.css';

import { mount } from 'svelte';
import App from './App.svelte';
import { manifest } from './lib/data/capabilities.svelte.ts';
import { startLocale } from './lib/i18n/i18n.svelte.ts';
import { followSystemTheme } from './lib/theme.ts';

// Before the first paint: the stylesheet deliberately renders nothing sensible without
// `data-theme` (ADR-0029), and this is the call that sets it.
followSystemTheme();

// …and the document has to say what language it is in and which way it runs. At boot the browser's
// own preference is all the client knows; the account's (F1-08) and the installation's supported
// locales (F1-10) replace it through `messages.adopt`.
startLocale();

// …and the one read the whole application configures itself from (F1-10). Started here rather
// than in a component, because it is read once for the lifetime of the page and because what it
// answers changes the language of the first paint. It needs no token: the manifest is the one
// unauthenticated route in the contract.
manifest.start();

const root = document.querySelector<HTMLDivElement>('#app');
if (!root) throw new Error('#app is missing from index.html');

mount(App, { target: root });
