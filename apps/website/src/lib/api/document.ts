// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The contract, read once at build time.
 *
 * `@hubtask/api-client` is the one workspace member the site may take the document from
 * (project-structure.md §2.1, P-01): `make api-client` copies `api/openapi.json` into it, and the
 * site imports the copy. This is a build-time read - every page is prerendered and nothing here
 * runs in a browser - which is what keeps the site's own rule intact: no API call, ever.
 */

import events from '@hubtask/api-client/events.json' with { type: 'json' };
import document from '@hubtask/api-client/openapi.json' with { type: 'json' };

import { readReference, type Document, type Reference } from './reference.ts';

export const reference: Reference = readReference(document as unknown as Document);

export interface EventSchema {
  readonly title?: string;
  readonly description?: string;
  readonly properties?: Readonly<Record<string, { readonly description?: string; readonly type?: string | readonly string[]; readonly const?: unknown; readonly format?: string; readonly properties?: Readonly<Record<string, unknown>> }>>;
  readonly required?: readonly string[];
}

/** The CloudEvents schemas under `api/events/`, keyed by type, in file order. */
export const eventSchemas: readonly (readonly [string, EventSchema])[] = Object.entries(events as Record<string, EventSchema>);

/** The message codes an operation may answer, as `locales/en.json` spells them - for the link. */
export const catalogueUrl = 'https://github.com/Jersyfi/hubtask/blob/main/locales/en.json';
export const contractUrl = 'https://github.com/Jersyfi/hubtask/blob/main/api/openapi.yaml';
