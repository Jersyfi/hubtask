// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The import, the parts that need no engine (F6-09, decision 15).
 *
 * **The kinds come from the contract.** `ImportKind` is generated from `api/openapi.yaml`, and the
 * list here is checked against it at compile time: a kind added to the contract and not to this
 * list is a type error, not a control that quietly never appears. The manifest declares no import
 * capability and nothing here adds one - a kind this build refuses answers by name (P-08), and the
 * dialog shows that answer.
 *
 * **The header row is read in the browser** so that the mapping can offer the file's own columns.
 * The rules mirror what the converter does with the same bytes (`infrastructure/importer/Csv.go`):
 * a byte order mark is skipped, the delimiter is whichever of `;` and `,` the first line has more
 * of, and a quoted cell may hold the delimiter. What the converter takes as a field without being
 * told is prefilled here for the same names, so the mapping sent is what differs from what the
 * file already says.
 */

import type { ImportKind, ImportRequest, ImportRun } from '@hubtask/sync-engine';

export type { ImportKind, ImportRequest, ImportRun };

/** The kinds, in the order they are offered. Every value of the contract's enum, and nothing else. */
export const IMPORT_KINDS = ['CSV', 'TRELLO', 'GOOGLE_TASKS', 'MICROSOFT_TODO'] as const satisfies readonly ImportKind[];

// The other direction: a kind the contract has and this list lacks is a compile error here.
type Unlisted = Exclude<ImportKind, (typeof IMPORT_KINDS)[number]>;
const everyKindListed: Unlisted extends never ? true : never = true;
void everyKindListed;

/** The seven fields a CSV column can carry, in the order the mapping asks for them. */
export const CSV_FIELDS = ['title', 'notes', 'due', 'completed', 'labels', 'bucket', 'parent'] as const;

export type CsvField = (typeof CSV_FIELDS)[number];

/**
 * The type the confirmation accepts for a kind, sent as the claim rather than what the browser
 * guesses from the extension: a `.csv` from one platform is `application/vnd.ms-excel` and from
 * another nothing at all, and the server judges the bytes either way.
 */
export function contentTypeFor(kind: ImportKind): string {
  return kind === 'CSV' ? 'text/csv' : 'application/json';
}

/** What the file dialog offers first for a kind. A hint to the platform, never a check. */
export function acceptFor(kind: ImportKind): string {
  return kind === 'CSV' ? '.csv,text/csv' : '.json,application/json';
}

/**
 * The header names the converter takes as a field without a mapping, lower-cased. The same table
 * as `csvFields` in `infrastructure/importer/Csv.go`, kept in step by hand: a name added there and
 * not here only costs a prefilled select, and a name here the converter lacks is sent as a
 * mapping, which the converter accepts.
 */
const HEADER_ALIASES: Readonly<Record<string, CsvField>> = {
  title: 'title', name: 'title', summary: 'title', subject: 'title',
  notes: 'notes', description: 'notes', body: 'notes', details: 'notes',
  due: 'due', due_date: 'due', 'due date': 'due', deadline: 'due', due_at: 'due',
  completed: 'completed', done: 'completed', status: 'completed', is_completed: 'completed',
  labels: 'labels', tags: 'labels', label: 'labels',
  bucket: 'bucket', list: 'bucket', column: 'bucket', state: 'bucket',
  parent: 'parent',
};

/** The first line of the text, whichever line ending the file uses. */
function firstLine(text: string): string {
  const end = text.search(/\r?\n/);
  return end === -1 ? text : text.slice(0, end);
}

/**
 * The column names of a CSV's header row, from the start of the file as text. Empty for a file
 * whose first line is blank - the converter refuses that file, and the mapping has nothing to
 * offer for it.
 */
export function parseHeader(text: string): readonly string[] {
  const line = firstLine(text.replace(/^\uFEFF/, ''));
  if (line.trim() === '') return [];
  const delimiter = (line.match(/;/g)?.length ?? 0) > (line.match(/,/g)?.length ?? 0) ? ';' : ',';
  const cells: string[] = [];
  let cell = '';
  let quoted = false;
  for (let i = 0; i < line.length; i += 1) {
    const char = line[i];
    if (quoted) {
      if (char === '"') {
        if (line[i + 1] === '"') {
          cell += '"';
          i += 1;
        } else {
          quoted = false;
        }
      } else {
        cell += char;
      }
    } else if (char === '"' && cell.trim() === '') {
      quoted = true;
      cell = '';
    } else if (char === delimiter) {
      cells.push(cell.trim());
      cell = '';
    } else {
      cell += char;
    }
  }
  cells.push(cell.trim());
  return cells;
}

/** The mapping's starting values: each field's column where a header already names it, else empty. */
export function prefill(columns: readonly string[]): Readonly<Record<CsvField, string>> {
  const chosen = Object.fromEntries(CSV_FIELDS.map((field) => [field, ''])) as Record<CsvField, string>;
  for (const column of columns) {
    const field = HEADER_ALIASES[column.trim().toLowerCase()];
    if (field && chosen[field] === '') chosen[field] = column;
  }
  return chosen;
}

/**
 * What to send as `mapping`: only the fields whose chosen column is not one the converter would
 * take by its name anyway. An empty result means no `mapping` at all.
 */
export function mappingFor(chosen: Readonly<Record<CsvField, string>>): Record<string, string> | undefined {
  const mapping: Record<string, string> = {};
  for (const field of CSV_FIELDS) {
    const column = chosen[field];
    if (column === '') continue;
    if (HEADER_ALIASES[column.trim().toLowerCase()] === field) continue;
    mapping[field] = column;
  }
  return Object.keys(mapping).length > 0 ? mapping : undefined;
}

/** The import a `JobRef` points at: the last segment of `result_url`, which is `/imports/{id}`. */
export function importIdOf(resultUrl: string | null | undefined): string | undefined {
  if (!resultUrl) return undefined;
  const segment = resultUrl.split('?')[0]?.split('/').pop();
  return segment === '' ? undefined : segment;
}
