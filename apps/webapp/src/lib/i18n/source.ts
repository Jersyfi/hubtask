// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The source catalogue, read synchronously - for tests only.
 *
 * The application loads `locales/en.json` as a chunk of its own, like every other catalogue
 * (`catalogue.ts`), so that a reader of another language does not download it before the first
 * screen. A test in plain Node has no chunks and wants the sentences at once; this is where it
 * gets them. Nothing outside a `*.test.ts` imports this module - `build/chunks.test.js` checks
 * the effect, an entry without the English sentences.
 */

import english from '../../../../../locales/en.json' with { type: 'json' };

import { type Catalogue, withoutMetadata } from './catalogue.ts';

/** `locales/en.json`, minus its notes to translators. */
export const SOURCE: Catalogue = withoutMetadata(english);
