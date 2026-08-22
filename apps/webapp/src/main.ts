// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The entry point of the to-do application.
 *
 * It is deliberately a placeholder. Which framework renders this is a decision that has not been
 * taken and must not be taken by whoever writes the second file here (ADR-0027, arc42 §2.2 C-14).
 * What is already settled and will constrain that decision:
 *
 *   - The content security policy of ADR-0028 permits neither `'unsafe-inline'` nor
 *     `'unsafe-eval'`. A framework that needs either does not qualify.
 *   - Every value comes from `@hubtask/design-system`; no colour, spacing or duration is written
 *     here (ADR-0029).
 *   - Every type comes from `@hubtask/api-client`, generated from `api/openapi.yaml` (ADR-0004).
 *
 * The bundle this produces is embedded into the server binary and served from `/`.
 */

import '@hubtask/design-system/fonts.css';
import '@hubtask/design-system/tokens.css';
import { tokens } from '@hubtask/design-system';

import './app.css';

const root = document.querySelector<HTMLDivElement>('#app');
if (!root) throw new Error('#app is missing from index.html');

const heading = document.createElement('h1');
heading.textContent = 'Hubtask';
heading.style.color = tokens.text.primary;

const note = document.createElement('p');
note.textContent =
  'The application shell is scaffolded. The frontend framework is a separate decision (ADR-0027).';

root.append(heading, note);
