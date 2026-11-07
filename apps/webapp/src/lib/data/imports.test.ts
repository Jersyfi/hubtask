// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { IMPORT_KINDS, SOURCE_FAMILIES, acceptFor, contentTypeFor, importIdOf, kindsFor, mappingFor, parseHeader, planOf, prefill } from './imports.ts';

test('the nine kinds are offered, CSV as text, a ZIP as a ZIP and the rest as JSON', () => {
  assert.deepEqual([...IMPORT_KINDS], ['CSV', 'XLSX', 'TRELLO', 'GOOGLE_TASKS', 'MICROSOFT_TODO', 'JIRA', 'CONFLUENCE', 'MARKDOWN', 'HTML', 'DOCX']);
  assert.equal(contentTypeFor('CSV'), 'text/csv');
  assert.equal(contentTypeFor('TRELLO'), 'application/json');
  assert.equal(contentTypeFor('JIRA', 'search.json'), 'application/json');
  assert.equal(contentTypeFor('JIRA', 'Projects.ZIP'), 'application/zip');
  assert.equal(contentTypeFor('CONFLUENCE', 'space.zip'), 'application/zip');
  assert.equal(contentTypeFor('MARKDOWN', 'notes.zip'), 'application/zip');
  assert.match(acceptFor('MARKDOWN'), /zip/);
  assert.equal(contentTypeFor('HTML', 'notes.zip'), 'application/zip');
  assert.equal(contentTypeFor('HTML', 'Page.HTML'), 'text/html');
  // An Office document is a ZIP to the server's sniff, and a sharper claim over it is refused as
  // media.type_mismatch: the claim is the ZIP the bytes are.
  assert.equal(contentTypeFor('DOCX', 'plan.docx'), 'application/zip');
  assert.match(acceptFor('DOCX'), /\.docx/);
  assert.equal(contentTypeFor('XLSX', 'assets.xlsx'), 'application/zip');
  assert.match(acceptFor('XLSX'), /\.xlsx/);
  assert.ok(kindsFor(true).includes('XLSX'), 'a workbook lands entries, so a collection offers it');
  assert.match(acceptFor('CSV'), /csv/);
  assert.match(acceptFor('GOOGLE_TASKS'), /json/);
  assert.match(acceptFor('JIRA'), /zip/);
});

// UC-IMP-05 check 4: a collection's Import… does not offer a kind that lands pages.
test('a wiki lands pages in the hub, so a collection does not offer it', () => {
  assert.ok(kindsFor(false).includes('CONFLUENCE'));
  assert.ok(!kindsFor(true).includes('CONFLUENCE'));
  assert.ok(kindsFor(false).includes('MARKDOWN') && !kindsFor(true).includes('MARKDOWN'));
  assert.ok(kindsFor(false).includes('DOCX') && !kindsFor(true).includes('HTML'));
  assert.ok(kindsFor(true).includes('JIRA'));
});

test('the header row: a byte order mark, a quoted cell holding the delimiter, and a semicolon file', () => {
  assert.deepEqual(parseHeader('\uFEFFTitle,Notes,"Due, by",Done\nrow'), ['Title', 'Notes', 'Due, by', 'Done']);
  assert.deepEqual(parseHeader('Titel;Notizen;Fällig\r\nrow;row;row'), ['Titel', 'Notizen', 'Fällig']);
  assert.deepEqual(parseHeader('a,"b ""quoted""",c'), ['a', 'b "quoted"', 'c']);
  assert.deepEqual(parseHeader('\n\nrow'), [], 'a blank first line offers nothing');
  assert.deepEqual(parseHeader('only'), ['only']);
});

test('a column named like a field is prefilled, as the converter would take it', () => {
  const chosen = prefill(['Name', 'Description', 'Due date', 'Tags', 'List', 'Owner']);
  assert.equal(chosen.title, 'Name');
  assert.equal(chosen.notes, 'Description');
  assert.equal(chosen.due, 'Due date');
  assert.equal(chosen.labels, 'Tags');
  assert.equal(chosen.bucket, 'List');
  assert.equal(chosen.completed, '');
  assert.equal(chosen.parent, '');
});

test('the mapping sent is only what differs from what the header already says', () => {
  const chosen = { ...prefill(['Name', 'Description', 'Owner', 'Finished']), completed: 'Finished' };
  assert.deepEqual(mappingFor(chosen), { completed: 'Finished' });
  assert.equal(mappingFor(prefill(['title', 'notes'])), undefined, 'nothing differs, no mapping');
});

test('the import a job points at is the last segment of result_url', () => {
  assert.equal(importIdOf('/imports/0192f000-0000-7000-8000-000000000001'), '0192f000-0000-7000-8000-000000000001');
  assert.equal(importIdOf('/imports/abc?x=1'), 'abc');
  assert.equal(importIdOf(null), undefined);
  assert.equal(importIdOf('/imports/'), undefined);
});

/** Words that show what was asked: the code, then its parameters in order. */
const words = {
  t: (code: string, params: Record<string, string> = {}) =>
    [code.replace('app.import.', ''), ...Object.values(params)].join(' '),
  list: (parts: readonly string[]) => parts.join(' + '),
};

test('a plan says what an import would create by the things a person counts, never a key', () => {
  const run = {
    report: {
      new: 260,
      entities: { containers: 1, buckets: 6, labels: 3, work_items: 248, item_labels: 2, comments: 1, page_comments: 1 },
    },
    refused: [{ row: 17, code: 'imports.row_title_missing' }],
  };
  assert.deepEqual(planOf(run, true, words), [
    { tone: 'add', text: 'plan_creates_dry count_collections 1 + count_columns 6 + count_labels 3 + count_entries 248 + count_comments 2' },
    { tone: 'refuse', text: 'plan_refused_dry 1' },
  ]);
  assert.equal(planOf(run, false, words)[0]?.text.startsWith('plan_creates_done'), true, 'after the run, in the past');
});

test('the same file again is left as it is, a mix says both, and a withheld reason is said in words', () => {
  assert.deepEqual(planOf({ report: { skipped: 251, entities: { work_items: 251 } } }, true, words), [
    { tone: 'keep', text: 'plan_all_kept_dry 251' },
  ]);
  const mixed = planOf({ report: { new: 4, skipped: 10, entities: { work_items: 14 }, withheld: { unmapped_members: 3, legal_hold_kept: 1 } } }, false, words);
  assert.deepEqual(mixed, [
    { tone: 'add', text: 'plan_some_new_done count_entries 14 4' },
    { tone: 'keep', text: 'plan_kept_done 10' },
    { tone: 'remove', text: 'plan_withheld_unmapped_members 3' },
    { tone: 'remove', text: 'plan_withheld_other 1' },
  ]);
  assert.deepEqual(planOf({ report: { new: 2, entities: { reminders: 2 } } }, true, words), [
    { tone: 'add', text: 'plan_creates_dry count_things 2' },
  ], 'an entity nobody counts is part of a total, not a name');
  assert.deepEqual(planOf({}, true, words), [], 'an empty file is an empty plan, said by the caller');
});

test('every source stands in exactly one family', () => {
  const listed = SOURCE_FAMILIES.flatMap((family) => [...family.kinds]);
  assert.deepEqual([...listed].sort(), [...IMPORT_KINDS].sort());
  assert.equal(new Set(listed).size, listed.length);
});
