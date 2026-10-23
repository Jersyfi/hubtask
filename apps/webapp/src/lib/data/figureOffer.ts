// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the *What counts* control offers a collection (UC-WRK-29 check 1), from the structure in
 * force - so it offers nothing the server refuses (P-05):
 *
 * * **PROGRESS** only where more than one level is in force: a list never shows a percentage
 *   (check 2), and the server refuses progress typed for one level (`structure.figure_not_offered`).
 * * **DUE** only where some offered level carries a due date: no level, no date to be due.
 * * **FIELD_SUM** only where the top level carries custom fields and a number field is offered.
 * * **FLOW** always: it counts the board's columns and what is completed, which every collection
 *   has (UC-WRK-31 check 4).
 *
 * And what the control has to explain: a lock (the value shown, not editable), and a progress the
 * layers set that the levels clip to open entries.
 */

import type { CustomFieldDefinition, EffectiveStructure } from '@hubtask/sync-engine';

export type FigureKind = 'OPEN' | 'FIELD_SUM' | 'DUE' | 'RHYTHM' | 'PROGRESS' | 'FLOW' | 'NONE';

/** The order the control lists them in: the simplest first, nothing last. */
const ORDER: readonly FigureKind[] = ['OPEN', 'FIELD_SUM', 'DUE', 'RHYTHM', 'PROGRESS', 'FLOW', 'NONE'];

export interface FigureOffer {
  /** The figures this collection may choose, in order. */
  readonly figures: readonly FigureKind[];
  /** The number fields a sum may name: a key is what a person reads for a field (no label is stored). */
  readonly fields: readonly { readonly key: string; readonly name: string }[];
  /** Where the value comes from, when it is not this collection's own. */
  readonly source: string;
  /** The lock's origin, where what counts is locked; the control then edits nothing. */
  readonly lockedBy: string | undefined;
  /** What the layers set and the levels clip, to explain why open entries are shown. */
  readonly clippedFrom: FigureKind | undefined;
  /** What counts now, and the field a sum names. */
  readonly current: FigureKind;
  readonly currentField: string | undefined;
}

const capabilitiesOf = (structure: EffectiveStructure, type: string): readonly string[] =>
  structure.settings.fields?.[type]?.capabilities ?? [];

/** The offer, from the structure in force and the number fields the collection sees. */
export function figureOffer(structure: EffectiveStructure, definitions: readonly CustomFieldDefinition[]): FigureOffer {
  const levels = (structure.settings.levels ?? []).map((level) => level.type);
  const fields = capabilitiesOf(structure, 'TASK').includes('CUSTOM_FIELDS')
    ? definitions
        .filter((definition) => definition.kind === 'NUMBER' && definition.applies_to.includes('TASK'))
        .map((definition) => ({ key: definition.key, name: definition.key }))
    : [];
  const offered = new Set<FigureKind>(['OPEN', 'RHYTHM', 'FLOW', 'NONE']);
  if (levels.length > 1) offered.add('PROGRESS');
  if (levels.some((type) => capabilitiesOf(structure, type).includes('DUE_DATE'))) offered.add('DUE');
  if (fields.length > 0) offered.add('FIELD_SUM');

  const counts = structure.settings.counts;
  return {
    figures: ORDER.filter((kind) => offered.has(kind)),
    fields,
    source: structure.sources?.counts ?? 'DEFAULT',
    lockedBy: structure.locks?.counts?.origin,
    clippedFrom: (structure.counts_clipped?.from.figure as FigureKind | undefined) ?? undefined,
    current: (counts?.figure as FigureKind | undefined) ?? 'NONE',
    currentField: counts?.field_key ?? undefined,
  };
}

/** The settings document a choice writes: the figure, and the field a sum names. */
export function countsChange(figure: FigureKind, fieldKey: string | undefined): { figure: FigureKind; field_key?: string } {
  return figure === 'FIELD_SUM' && fieldKey ? { figure, field_key: fieldKey } : { figure };
}

/** Whether a choice can be saved as it stands: a sum needs its field. */
export function isComplete(figure: FigureKind, fieldKey: string | undefined): boolean {
  return figure !== 'FIELD_SUM' || Boolean(fieldKey);
}
