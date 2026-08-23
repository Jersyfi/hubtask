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
 *   - Every type comes from `@hubtask/api-client`, generated from `api/openapi.yaml` (ADR-0004).
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

const root = document.querySelector<HTMLDivElement>('#app');
if (!root) throw new Error('#app is missing from index.html');

mount(App, { target: root });
