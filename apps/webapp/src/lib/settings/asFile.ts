// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A settings page's part of the configuration document (configuration/ui.md A10): the values the
 * page shows, in the shape the installation's document and `hubctl` already use - per area, per
 * name, `{ set, value, locked }` - so what a person reads here is what they would write there.
 *
 * Only the place's own decisions are `set`; a value that comes from a level above is `set: false`
 * and carries no value, because the document of this place does not decide it (the same rule as
 * `InstanceSetting`). Pure, and tested beside itself.
 */

import { areaOf, nameOf } from './catalogue.ts';

/** One value a page shows: its catalogue key, the value in force, where it comes from. */
export interface ShownValue {
  readonly key: string;
  readonly value: unknown;
  /** The level the value in force comes from - `DEFAULT`, `INSTANCE`, … */
  readonly source: string;
  /** The level that locked it, where one did. */
  readonly lockedBy?: string;
}

interface DocumentValue {
  set: boolean;
  value?: unknown;
  locked: boolean;
}

/** The document's part for the values at one place - `WORKSPACE`, `HUB`, … - as indented JSON. */
export function asFile(values: readonly ShownValue[], place: string): string {
  const document: Record<string, Record<string, DocumentValue>> = {};
  for (const shown of values) {
    const area = (document[areaOf(shown.key)] ??= {});
    const isSetHere = shown.source === place;
    area[nameOf(shown.key)] = isSetHere
      ? { set: true, value: shown.value, locked: shown.lockedBy === place }
      : { set: false, locked: shown.lockedBy === place };
  }
  return JSON.stringify(document, null, 2);
}

/**
 * The part of the configuration document a settings page writes (A10): whole sections, the
 * `settings` section's keys of the page's areas, and - on a collection's page - only the blocks of
 * the hub document's `collection_*` sections that belong to that collection (`<handle>.<key>`).
 */
export interface PagePart {
  readonly sections: readonly string[];
  /** Areas of the `settings` section - `sign_in`, `time` - whose keys the page writes. */
  readonly settings?: readonly string[];
  /** A collection's handle: its blocks in the hub document's collection sections. */
  readonly handle?: string;
}

/** The page's part of a document, as indented JSON: what `hubctl config export` would hold of it. */
export function pagePartFile(document: Readonly<Record<string, unknown>>, part: PagePart): string {
  const out: Record<string, unknown> = {};
  for (const name of part.sections) {
    const section = (document[name] ?? {}) as Record<string, unknown>;
    if (part.handle === undefined || !name.startsWith('collection_')) {
      out[name] = section;
      continue;
    }
    const prefix = `${part.handle}.`;
    out[name] = Object.fromEntries(Object.entries(section).filter(([key]) => key.startsWith(prefix)));
  }
  if (part.settings && part.settings.length > 0) {
    const settings = (document.settings ?? {}) as Record<string, unknown>;
    out.settings = Object.fromEntries(Object.entries(settings).filter(([key]) => part.settings?.includes(areaOf(key))));
  }
  return JSON.stringify(out, null, 2);
}

/**
 * Each workspace settings page's part of the document, by its address. *Changes* shows the whole
 * document and *Modules* its own values (`asFile`), so neither is listed here.
 */
export const PAGE_PARTS: Readonly<Record<string, PagePart>> = {
  '/administration/sign-in': { sections: [], settings: ['sign_in'] },
  '/administration/legal': { sections: [], settings: ['legal'] },
  '/administration/workspace': { sections: [], settings: ['localisation', 'knowledge', 'servicedesk'] },
  '/administration/ai': { sections: [], settings: ['ai'] },
  '/administration/presets': { sections: ['structure'] },
  '/administration/agents': { sections: [], settings: ['agents'] },
  '/administration/time': { sections: [], settings: ['time'] },
  '/administration/rules': { sections: ['rules'] },
};

/** A page's part by its address; a page not listed has none. */
export const pagePart = (path: string): PagePart => PAGE_PARTS[path] ?? { sections: [] };

/** A hub's or a collection's settings page's part of the hub's document. */
export const placePart = (isHub: boolean, handle?: string): PagePart =>
  isHub
    ? { sections: ['structure', 'scheme', 'views', 'rules'] }
    : { sections: ['collection_structure', 'collection_fields', 'collection_views', 'collection_rules', 'collection_tolerance'], handle };
