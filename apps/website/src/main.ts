// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The entry point of the project website (hubtask.eu).
 *
 * This is the information site — what Hubtask is, how it is licensed, and the way into the
 * documentation. It does no task management and holds no API client: it is not a client of the
 * server at all, which is why it is not embedded into the binary (ADR-0028).
 *
 * A placeholder, like the webapp's: the framework is a separate decision. What is settled is that
 * every value comes from `@hubtask/design-system`.
 */

import '@hubtask/design-system/fonts.css';
import '@hubtask/design-system/tokens.css';

import './site.css';

const root = document.querySelector<HTMLElement>('#site');
if (!root) throw new Error('#site is missing from index.html');

const heading = document.createElement('h1');
heading.textContent = 'Hubtask';

const note = document.createElement('p');
note.textContent = 'Task management in five levels. Self-hostable, offline-capable, source available.';

root.append(heading, note);
