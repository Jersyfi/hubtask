// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-INS-21 check 14: what switching Workflows off strands, summed from the scheme and the previews
// of removing each row - the places, the open entries in a status, those that could then not even
// be completed, and the column each status keeps.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { readFileSync } from 'node:fs';

import type { WorkflowList, WorkflowPreview } from '@hubtask/sync-engine';
import { format } from '../i18n/format.ts';
import { needsChoice, rowsInReach, strandedBy } from './workflowsoff.ts';

const offers = {
  key: 'offer', published: {
    initial: 'draft',
    statuses: [
      { key: 'draft', name: 'Draft', category: 'TODO' },
      { key: 'check', name: 'In review', category: 'IN_PROGRESS' },
      { key: 'released', name: 'Released', category: 'IN_PROGRESS' },
      { key: 'sent', name: 'Sent', category: 'DONE' },
    ],
    steps: [
      { key: 'submit', from: ['draft'], to: 'check' },
      { key: 'release', from: ['check'], to: 'released' },
      { key: 'send', from: ['released'], to: 'sent' },
    ],
  },
} as unknown as WorkflowList['data'][number];

const move = (collection_id: string, from: string, count: number, completed = false) =>
  ({ collection_id, entry_type: 'task', from, to: null, completed, count });

test('at the workspace every row is in reach; at a hub its own row and its collections\' rows', () => {
  const scheme = [
    { container_id: 'hub-a', entry_type: null, workflow: 'offer' },
    { container_id: 'col-a1', entry_type: 'task', workflow: 'offer' },
    { container_id: 'col-b1', entry_type: null, workflow: 'offer' },
  ];
  assert.equal(rowsInReach(scheme, undefined, []).length, 3);
  assert.deepEqual(rowsInReach(scheme, 'hub-a', ['col-a1']).map((row) => row.container_id), ['hub-a', 'col-a1']);
});

test('the open entries by status, the stuck ones, and the column each status keeps', () => {
  const preview = {
    moves: [move('col', 'draft', 6), move('col', 'check', 7), move('col', 'released', 4), move('col', 'sent', 23, true)],
    entries: 40, removed: [], columns_before: [], columns_after: [],
    boards: [{ collection_id: 'col' }],
  } as unknown as WorkflowPreview;
  const columns = [
    { id: 'open', statuses: ['draft'], is_done_bucket: false },
    { id: 'doing', statuses: ['check', 'released'], is_done_bucket: false },
    { id: 'done', statuses: ['sent'], is_done_bucket: true },
  ];
  const stranded = strandedBy([{ row: { container_id: 'col', entry_type: null, workflow: 'offer' }, preview }], [offers], () => columns);
  assert.deepEqual(stranded.places, ['col']);
  assert.deepEqual(stranded.collections, ['col']);
  assert.equal(stranded.open, 17, 'the done ones are not counted: they stay done');
  // Only Released has a step straight into done: Draft and In review would be stuck while paused.
  assert.equal(stranded.stuck, 13);
  assert.deepEqual(stranded.lines.map((line) => [line.status, line.count, line.columnId, line.isStuck]), [
    ['draft', 6, 'open', true], ['check', 7, 'doing', true], ['released', 4, 'doing', false],
  ]);
  assert.equal(needsChoice(stranded), true);
});

test('a status no column shows, or a list without columns, has no column of its own; nothing open needs no choice', () => {
  const preview = { moves: [move('list', 'draft', 2), move('list', 'sent', 1, true)], entries: 3, removed: [], columns_before: [], columns_after: [] } as unknown as WorkflowPreview;
  const stranded = strandedBy([{ row: { container_id: 'list', entry_type: null, workflow: 'offer' }, preview }], [offers], () => []);
  assert.equal(stranded.lines[0]?.columnId, undefined);
  const quiet = strandedBy([{ row: { container_id: 'list', entry_type: null, workflow: 'offer' }, preview: { ...preview, moves: [move('list', 'sent', 1, true)] } }], [offers], () => []);
  assert.equal(needsChoice(quiet), false);
  assert.equal(needsChoice(undefined), false);
});

// The pause card and the stop line count in both languages: one entry is not "the 1 entries", none
// stuck is not "0 of them", and a stop before the first place says so instead of "0 of 1 places".
test('the pause card and the stop line say one, many and none in English and German', () => {
  const catalogue = (language: string) => JSON.parse(readFileSync(new URL(`../../../../../locales/${language}.json`, import.meta.url), 'utf8')) as Record<string, string>;
  const en = catalogue('en');
  const de = catalogue('de');
  const pause = (messages: Record<string, string>, locale: string, open: number, stuck: number) => format(messages['app.modules.off_wf_pause_hint'] ?? '', { open, stuck }, locale);
  assert.match(pause(en, 'en', 1, 0), /Until then the entry cannot move on, and the cards/);
  assert.match(pause(en, 'en', 1, 1), /Until then the entry cannot move on or be completed, and/);
  assert.match(pause(en, 'en', 17, 13), /Until then the 17 entries cannot move on, 13 of them cannot be completed, and/);
  assert.match(pause(en, 'en', 17, 0), /Until then the 17 entries cannot move on, and/);
  assert.match(pause(de, 'de', 1, 1), /Bis dahin lässt sich der Eintrag nicht weiterbewegen und nicht abhaken, und/);
  assert.match(pause(de, 'de', 17, 0), /Bis dahin lassen sich die 17 Einträge nicht weiterbewegen, und/);
  const stopped = (messages: Record<string, string>, locale: string, done: number, count: number) => format(messages['app.modules.off_wf_stopped'] ?? '', { done, count }, locale);
  assert.match(stopped(en, 'en', 0, 1), /^Converting stopped: the place does not run on columns yet\./);
  assert.match(stopped(en, 'en', 0, 3), /none of the 3 places runs on columns yet/);
  assert.match(stopped(en, 'en', 1, 2), /1 of 2 places runs on columns now/);
  assert.match(stopped(en, 'en', 2, 3), /2 of 3 places run on columns now/);
  assert.match(stopped(de, 'de', 0, 1), /Der Ort läuft noch nicht mit Spalten/);
  assert.match(stopped(de, 'de', 1, 2), /1 von 2 Orten läuft jetzt mit Spalten/);
  for (const messages of [en, de]) {
    assert.ok(!/ …/.test(messages['app.modules.off_wf_converting_title'] ?? '') && !/ …/.test(messages['app.modules.off_wf_reading'] ?? ''), 'an ellipsis after a space (voice-and-tone.md §2.4)');
  }
});
