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
