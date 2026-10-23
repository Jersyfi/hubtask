// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { WorkItem } from '@hubtask/sync-engine';

import type { JumbleEntry } from '../data/jumble.svelte.ts';

import { instantOf } from '../i18n/zone.ts';
import type { Reading } from './parse.ts';
import { MAX_CAPTURE_LINES, type CaptureRules } from '../entries/capture.ts';
import { bodyOf, createAll, readLinesOf, type CreateBody, type Writer } from './submit.ts';

/** A writer that records every call, and refuses what it is told to. */
function fakeWriter(options: { refuseAt?: number } = {}) {
  const calls: string[] = [];
  let made = 0;
  const writer: Writer = {
    async createItem(body: CreateBody, key: string) {
      if (options.refuseAt === made) {
        calls.push(`refused ${body.title}`);
        throw new Error('refused');
      }
      made += 1;
      calls.push(`create ${body.title} ${key}`);
      return { id: `item-${made}`, version: 1, title: body.title } as unknown as WorkItem;
    },
    async submitJumble(subject: string, key: string) {
      calls.push(`jumble ${subject} ${key}`);
      return { id: 'entry-1' } as unknown as JumbleEntry;
    },
  };
  return { writer, calls };
}

const reading = (title: string, chips: Reading['chips'] = []): Reading => ({ title, chips });

const UNITS = ['g', 'kg', 'l'];
const LIST: CaptureRules = { splitsLines: true, readsAmount: true, units: UNITS };
const TASKS: CaptureRules = { splitsLines: false, readsAmount: false, units: UNITS };
/** A stand-in for `readLine`: "tomorrow" is a date chip, the rest the title. */
const read = (line: string): Reading => {
  const at = line.indexOf('tomorrow');
  if (at < 0) return reading(line);
  const title = `${line.slice(0, at)}${line.slice(at + 'tomorrow'.length)}`.replace(/\s+/gu, ' ').trim();
  return reading(title, [{ kind: 'date', text: 'tomorrow', start: at, end: at + 8, date: '2026-10-22' }]);
};
const summary = (text: string, rules: CaptureRules) => {
  const { lines, rest } = readLinesOf(text, rules, read);
  return { lines: lines.map((each) => [each.reading.title, each.amount ?? null, each.reading.chips.length]), rest };
};

test('one line where lines do not split: a pasted list is one entry, its lines joined', () => {
  assert.deepEqual(summary('milk\neggs\n\nbread', TASKS), { lines: [['milk eggs bread', null, 0]], rest: '' });
  assert.deepEqual(summary('   ', TASKS), { lines: [], rest: '' });
  // Without AMOUNT a leading number is the title's.
  assert.deepEqual(summary('2 kg apples', TASKS).lines, [['2 kg apples', null, 0]]);
});

test('a list reads lines and amounts: one entry per line, the amount off what the chips leave (UC-WRK-26 check 3)', () => {
  assert.deepEqual(summary('- 2 milk\n\n tomorrow 1 kg apples \nbread tomorrow', LIST), {
    lines: [['milk', '2', 0], ['apples', '1 kg', 1], ['bread', null, 1]],
    rest: '',
  });
  // The line that goes back into the field is the line as typed, without its bullet.
  assert.deepEqual(readLinesOf('- 2 milk', LIST, read).lines.map((each) => each.line), ['2 milk']);
  // Taken back from the chip, the number stays in the title.
  assert.deepEqual(summary('2 milk', { ...LIST, readsAmount: false }).lines, [['2 milk', null, 0]]);
});

test('lines split up to the bound, the rest kept', () => {
  const many = Array.from({ length: MAX_CAPTURE_LINES + 2 }, (_, index) => `line ${index}`).join('\n');
  const split = readLinesOf(many, LIST, read);
  assert.equal(split.lines.length, MAX_CAPTURE_LINES);
  assert.equal(split.rest, `line ${MAX_CAPTURE_LINES}\nline ${MAX_CAPTURE_LINES + 1}`);
});

test('the amount is sent only where it was read', () => {
  const place = { collectionId: 'c', type: 'TASK' };
  const clock = { now: 0, zone: 'UTC' };
  assert.equal(bodyOf(reading('milk'), place, clock, '2').amount, '2');
  assert.equal('amount' in bodyOf(reading('milk'), place, clock), false);
});

test('the body carries exactly what the chips say, the due date in the person’s zone', () => {
  const zone = 'Pacific/Kiritimati';
  const now = Date.parse(instantOf('2026-10-21', '23:59', zone)!);
  const body = bodyOf(
    reading('Call the electrician', [
      { kind: 'date', text: 'tomorrow', start: 0, end: 0, date: '2026-10-22' },
      { kind: 'time', text: '11am', start: 0, end: 0, time: '11:00' },
      { kind: 'person', text: '+Mara', start: 0, end: 0, id: 'mara' },
      { kind: 'label', text: '!Urgent', start: 0, end: 0, id: 'urgent' },
    ]),
    { collectionId: 'kitchen', type: 'TASK' },
    { now, zone },
  );
  assert.deepEqual(body, {
    type: 'TASK',
    collection_id: 'kitchen',
    title: 'Call the electrician',
    due_at: instantOf('2026-10-22', '11:00', zone),
    due_date_only: false,
    due_time_zone: zone,
    assignee_id: 'mara',
    label_ids: ['urgent'],
  });
});

test('no chips, no fields: nothing is guessed', () => {
  assert.deepEqual(bodyOf(reading('Water'), { collectionId: 'kitchen', type: 'TASK' }, { now: 0, zone: 'UTC' }), {
    type: 'TASK',
    collection_id: 'kitchen',
    title: 'Water',
  });
});

test('entries are made in order, each with the key of its line', async () => {
  const { writer, calls } = fakeWriter();
  const bodies = ['a', 'b'].map((title) => ({ type: 'TASK', collection_id: 'k', title }));
  const outcome = await createAll(writer, bodies, ['k1', 'k2'], ['a', 'b'], '');
  assert.deepEqual(calls, ['create a k1', 'create b k2']);
  assert.equal(outcome.kind === 'items' ? outcome.created.length : 0, 2);
});

test('a refusal part-way keeps what was made and puts the rest back in the field', async () => {
  const { writer } = fakeWriter({ refuseAt: 1 });
  const bodies = ['a', 'b', 'c'].map((title) => ({ type: 'TASK', collection_id: 'k', title }));
  const outcome = await createAll(writer, bodies, ['k1', 'k2', 'k3'], ['a', 'b', 'c'], 'd');
  assert.equal(outcome.kind, 'items');
  if (outcome.kind !== 'items') return;
  assert.deepEqual(outcome.created.map((item) => item.title), ['a']);
  assert.ok(outcome.failure);
  assert.equal(outcome.rest, 'b\nc\nd');
});
