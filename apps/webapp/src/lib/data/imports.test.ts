// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { IMPORT_KINDS, acceptFor, contentTypeFor, importIdOf, kindsFor, mappingFor, parseHeader, prefill } from './imports.ts';

test('the nine kinds are offered, CSV as text, a ZIP as a ZIP and the rest as JSON', () => {
  assert.deepEqual([...IMPORT_KINDS], ['CSV', 'TRELLO', 'GOOGLE_TASKS', 'MICROSOFT_TODO', 'JIRA', 'CONFLUENCE', 'MARKDOWN', 'HTML', 'DOCX']);
  assert.equal(contentTypeFor('CSV'), 'text/csv');
  assert.equal(contentTypeFor('TRELLO'), 'application/json');
  assert.equal(contentTypeFor('JIRA', 'search.json'), 'application/json');
  assert.equal(contentTypeFor('JIRA', 'Projects.ZIP'), 'application/zip');
  assert.equal(contentTypeFor('CONFLUENCE', 'space.zip'), 'application/zip');
  assert.equal(contentTypeFor('MARKDOWN', 'notes.zip'), 'application/zip');
  assert.match(acceptFor('MARKDOWN'), /zip/);
  assert.equal(contentTypeFor('HTML', 'notes.zip'), 'application/zip');
  assert.equal(contentTypeFor('HTML', 'Page.HTML'), 'text/html');
  assert.match(contentTypeFor('DOCX', 'plan.docx'), /wordprocessingml/);
  assert.match(acceptFor('DOCX'), /\.docx/);
  assert.match(acceptFor('CSV'), /csv/);
  assert.match(acceptFor('GOOGLE_TASKS'), /json/);
  assert.match(acceptFor('JIRA'), /zip/);
});

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
