// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The import, the parts that need no engine (design-system.md §11.8).
 *
 * **The kinds come from the contract.** `ImportKind` is generated from `api/openapi.yaml`, and the
 * list here is checked against it at compile time: a kind added to the contract and not to this
 * list is a type error, not a control that quietly never appears. The manifest declares no import
 * capability and nothing here adds one - a kind this build refuses answers by name, and the
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
export const IMPORT_KINDS = [
  'CSV', 'XLSX', 'TRELLO', 'GOOGLE_TASKS', 'MICROSOFT_TODO', 'JIRA', 'CONFLUENCE', 'MARKDOWN', 'HTML', 'DOCX',
] as const satisfies readonly ImportKind[];

/**
 * The sources as a person looks for them (user-experience.md §7.11, *Where from*): by what they
 * kept their work in, not by file format. Every kind in exactly one family.
 */
export const SOURCE_FAMILIES = [
  { family: 'spreadsheets', kinds: ['CSV', 'XLSX'] },
  { family: 'task_apps', kinds: ['TRELLO', 'GOOGLE_TASKS', 'MICROSOFT_TODO'] },
  { family: 'trackers', kinds: ['JIRA'] },
  { family: 'documents', kinds: ['CONFLUENCE', 'MARKDOWN', 'HTML', 'DOCX'] },
] as const satisfies readonly { family: string; kinds: readonly ImportKind[] }[];

const MARKS = {
  CSV: 'file-text', XLSX: 'file-text', TRELLO: 'layout-template', GOOGLE_TASKS: 'square-check',
  MICROSOFT_TODO: 'square-check', JIRA: 'flag', CONFLUENCE: 'book-open', MARKDOWN: 'file-text', HTML: 'globe', DOCX: 'file-text',
} as const satisfies Record<ImportKind, string>;

/** A source's mark on its card. */
export function markOf(kind: ImportKind): (typeof MARKS)[ImportKind] {
  return MARKS[kind];
}

/** The kinds that land pages in the hub: offered on a hub only, never into a collection. */
const LANDS_PAGES: ReadonlySet<ImportKind> = new Set<ImportKind>(['CONFLUENCE', 'MARKDOWN', 'HTML', 'DOCX']);

/** The kinds offered where the import starts: on a hub every one, in a collection those that land entries. */
export function kindsFor(intoCollection: boolean): readonly ImportKind[] {
  return intoCollection ? IMPORT_KINDS.filter((kind) => !LANDS_PAGES.has(kind)) : IMPORT_KINDS;
}

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
export function contentTypeFor(kind: ImportKind, fileName = ''): string {
  if (kind === 'CSV') return 'text/csv';
  const name = fileName.toLowerCase();
  // One document on its own is sent as what it is; a ZIP of them as a ZIP.
  if (kind === 'HTML' && (name.endsWith('.html') || name.endsWith('.htm'))) return 'text/html';
  // A wiki's export is a ZIP, and so is an issue tracker's answer with its attachments. An Office
  // document is one too, to the server's sniff, which refuses a sharper claim over it.
  if (LANDS_PAGES.has(kind) || kind === 'XLSX' || name.endsWith('.zip')) return 'application/zip';
  return 'application/json';
}

/** What the file dialog offers first for a kind. A hint to the platform, never a check. */
export function acceptFor(kind: ImportKind): string {
  if (kind === 'CSV') return '.csv,text/csv';
  if (kind === 'HTML') return '.html,.htm,.zip,text/html,application/zip';
  if (kind === 'DOCX') return '.docx,.zip,application/zip';
  if (kind === 'XLSX') return '.xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';
  if (LANDS_PAGES.has(kind)) return '.zip,application/zip';
  if (kind === 'JIRA') return '.json,.zip,application/json,application/zip';
  return '.json,application/json';
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

/**
 * The entities a person counts, in the order a plan names them, each with the word it is counted
 * by (`app.import.count_<word>`). Two archive entities can share a word - a page's comments are
 * comments. What is not here (a label on an entry, a sprint's membership) is part of the total
 * and not named, because nobody counts it.
 */
const COUNTED: readonly (readonly [entity: string, word: string])[] = [
  ['containers', 'collections'],
  ['buckets', 'columns'],
  ['labels', 'labels'],
  ['entry_types', 'types'],
  ['custom_field_definitions', 'fields'],
  ['workflows', 'workflows'],
  ['sprints', 'sprints'],
  ['work_items', 'entries'],
  ['pages', 'pages'],
  ['page_versions', 'versions'],
  ['comments', 'comments'],
  ['page_comments', 'comments'],
  ['media_objects', 'files'],
  ['saved_views', 'views'],
];

/** The withheld reasons an import says in its own words; any other is said generically. */
const WITHHELD = new Set(['unmapped_members', 'orphaned', 'media_missing']);

export type PlanTone = 'add' | 'keep' | 'remove' | 'refuse';

export interface PlanLine {
  readonly text: string;
  readonly tone: PlanTone;
}

export interface PlanWords {
  t(code: string, params?: Record<string, string>): string;
  /** *1 collection, 6 columns and 248 entries*: the locale's conjunction. */
  list(parts: readonly string[]): string;
}

/**
 * An import's report as a plan in words (user-experience.md §7.11): what it would create - or
 * created - by the things a person counts, what is left as it is because it is already here, the
 * rows that would not land and what the source carried that has no place here. Never a key: a
 * reason without words of its own is said generically. `isDry` chooses the tense.
 */
export function planOf(run: Pick<ImportRun, 'report' | 'refused'>, isDry: boolean, words: PlanWords): PlanLine[] {
  const report = run.report ?? {};
  const counts = new Map<string, number>();
  for (const [entity, word] of COUNTED) {
    const count = report.entities?.[entity] ?? 0;
    if (count > 0) counts.set(word, (counts.get(word) ?? 0) + count);
  }
  const fresh = report.new ?? 0;
  const kept = report.skipped ?? 0;
  const total = fresh + kept + (report.overwritten ?? 0) + (report.duplicated ?? 0);
  const things =
    counts.size > 0
      ? words.list([...counts].map(([word, count]) => words.t(`app.import.count_${word}`, { count: String(count) })))
      : words.t('app.import.count_things', { count: String(total) });
  const tense = isDry ? 'dry' : 'done';

  const lines: PlanLine[] = [];
  if (fresh > 0 && kept === 0) {
    lines.push({ tone: 'add', text: words.t(`app.import.plan_creates_${tense}`, { things }) });
  } else if (fresh === 0 && kept > 0) {
    lines.push({ tone: 'keep', text: words.t(`app.import.plan_all_kept_${tense}`, { count: String(kept) }) });
  } else if (fresh > 0) {
    lines.push({ tone: 'add', text: words.t(`app.import.plan_some_new_${tense}`, { things, count: String(fresh) }) });
    lines.push({ tone: 'keep', text: words.t(`app.import.plan_kept_${tense}`, { count: String(kept) }) });
  }
  const refused = run.refused?.length ?? 0;
  if (refused > 0) {
    lines.push({ tone: 'refuse', text: words.t(`app.import.plan_refused_${tense}`, { count: String(refused) }) });
  }
  for (const [reason, count] of Object.entries(report.withheld ?? {})) {
    if (count <= 0) continue;
    const code = WITHHELD.has(reason) ? `app.import.plan_withheld_${reason}` : 'app.import.plan_withheld_other';
    lines.push({ tone: 'remove', text: words.t(code, { count: String(count) }) });
  }
  return lines;
}
