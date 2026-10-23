// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { CollectionFigure } from '@hubtask/sync-engine';

import {
  barShare,
  durationText,
  figureTargets,
  headMessage,
  isStale,
  overLimitMessage,
  rhythmMarks,
  rowMessage,
  shareOf,
} from './figure.ts';

const today = '2026-10-22';
const of = (figure: Omit<CollectionFigure, 'today'>): CollectionFigure => ({ today, ...figure });

const every: CollectionFigure[] = [
  of({ kind: 'OPEN', open: 5 }),
  of({ kind: 'FIELD_SUM', open: 5, sum: 38.1, summed: 5 }),
  of({ kind: 'DUE', due: 3 }),
  of({ kind: 'RHYTHM', days: ['2026-10-19', '2026-10-21'], week_start: '2026-10-19' }),
  of({ kind: 'PROGRESS', done: 5, total: 13 }),
  of({ kind: 'FLOW', completed: 12, average_seconds: 181_440, over_limit: 1 }),
];

const rendered = (message: { params: Readonly<Record<string, string | number>> } | undefined) =>
  JSON.stringify(message?.params ?? {});

test('UC-WRK-29 check 2: only progress carries a share, and only a bar for it', () => {
  for (const figure of every) {
    const head = rendered(headMessage(figure, 'en'));
    const row = rendered(rowMessage(figure, 'en'));
    const hasShare = head.includes('%') || row.includes('%');
    assert.equal(hasShare, figure.kind === 'PROGRESS', `${figure.kind}: ${head} ${row}`);
    assert.equal(barShare(figure) !== undefined, figure.kind === 'PROGRESS');
  }
  assert.equal(headMessage(every[4]!, 'en')?.params.share, '38%');
  assert.equal(headMessage(every[4]!, 'de')?.params.share, '38\u00a0%');
});

test('the share is rounded down: 100 % only when everything is done, none without leaves', () => {
  assert.equal(shareOf(199, 200), 99);
  assert.equal(shareOf(200, 200), 100);
  assert.equal(shareOf(0, 0), undefined);
  assert.equal(headMessage(of({ kind: 'PROGRESS', done: 0, total: 0 }), 'en'), undefined);
  assert.equal(rowMessage(of({ kind: 'PROGRESS', done: 0, total: 0 }), 'en'), undefined);
});

test('UC-WRK-29 check 3: a week without a completion shows no number and names no day', () => {
  const empty = of({ kind: 'RHYTHM', days: [], week_start: '2026-10-19' });
  const head = headMessage(empty, 'en');
  assert.equal(head?.code, 'app.figure.rhythm_new_week');
  assert.deepEqual(head?.params, {});
  assert.equal(rowMessage(empty, 'en'), undefined);
  assert.deepEqual(rhythmMarks(empty), []);
});

test('rhythm draws one mark per day with a completion, and none for any other', () => {
  const figure = of({ kind: 'RHYTHM', days: ['2026-10-21', '2026-10-19', '2026-10-21'], week_start: '2026-10-19' });
  assert.deepEqual(rhythmMarks(figure), ['2026-10-19', '2026-10-21']);
  assert.deepEqual(headMessage(figure, 'en'), { code: 'app.figure.rhythm', params: { count: 3 } });
});

test('a sum short of the open entries is said to be about', () => {
  assert.equal(headMessage(of({ kind: 'FIELD_SUM', open: 5, sum: 38.1, summed: 4 }), 'en')?.code, 'app.figure.field_sum_about');
  assert.deepEqual(headMessage(of({ kind: 'FIELD_SUM', open: 5, sum: 38.1, summed: 5 }), 'en'), {
    code: 'app.figure.field_sum',
    params: { count: 5, sum: '38.1' },
  });
  // No live number field: what is open, and no sum.
  assert.equal(headMessage(of({ kind: 'FIELD_SUM', open: 5, sum: null }), 'en')?.code, 'app.figure.open');
});

test('a figure that could not be counted says so in the head and stays out of the row (P-11)', () => {
  const failed = of({ kind: 'OPEN', unavailable: true });
  assert.equal(headMessage(failed, 'en')?.code, 'app.figure.unavailable');
  assert.equal(rowMessage(failed, 'en'), undefined);
  assert.equal(barShare(of({ kind: 'PROGRESS', unavailable: true })), undefined);
});

test('a figure counted for yesterday is stale at the reader’s midnight', () => {
  assert.equal(isStale(every[0], today), false);
  assert.equal(isStale(every[0], '2026-10-23'), true);
  assert.equal(isStale(undefined, today), false);
});

test('a batch of changed collections re-reads each and each hub once', () => {
  const parents: Record<string, string> = { a: 'hub-1', b: 'hub-1', c: 'hub-2' };
  const targets = figureTargets(['a', 'b', 'a', 'c', 'gone'], (id) => parents[id]);
  assert.deepEqual(targets.collections, ['a', 'b', 'c', 'gone']);
  assert.deepEqual(targets.hubs, ['hub-1', 'hub-2']);
});

test('UC-WRK-31 check 4: the flow says how many a week and how long on average', () => {
  const flow = of({ kind: 'FLOW', completed: 12, average_seconds: 181_440, over_limit: 0 });
  assert.deepEqual(headMessage(flow, 'en'), { code: 'app.figure.flow', params: { count: 12, duration: '2.1 days' } });
  assert.equal(headMessage(flow, 'de')?.params.duration, '2,1 Tage');
  assert.deepEqual(rowMessage(flow, 'en'), { code: 'app.figure.short.flow', params: { count: 12 } });
  // Nothing completed: no average is invented.
  assert.deepEqual(headMessage(of({ kind: 'FLOW', completed: 0, average_seconds: null, over_limit: 0 }), 'en'), {
    code: 'app.figure.flow_none',
    params: {},
  });
});

test('a duration under a day is told in hours, never as zero', () => {
  assert.equal(durationText(18_000, 'en'), '5 hours');
  assert.equal(durationText(60, 'en'), '1 hour');
  assert.equal(durationText(86_400, 'en'), '1 day');
});

test('UC-WRK-31 check 3: a column over its limit is marked in the head and the row, and only for a flow', () => {
  const full = of({ kind: 'FLOW', completed: 3, average_seconds: 3_600, over_limit: 2 });
  assert.deepEqual(overLimitMessage(full), { code: 'app.figure.flow_over_limit', params: { count: 2 } });
  assert.deepEqual(rowMessage(full, 'en'), { code: 'app.figure.short.flow_over_limit', params: { count: 2 } });
  assert.equal(overLimitMessage(of({ kind: 'FLOW', completed: 3, average_seconds: 3_600, over_limit: 0 })), undefined);
  assert.equal(overLimitMessage(of({ kind: 'OPEN', open: 3 })), undefined);
  assert.equal(overLimitMessage(of({ kind: 'FLOW', unavailable: true })), undefined);
});
