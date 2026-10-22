// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Following the words of a title being typed (UC-AI-04 check 7).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { followTitle, worthAsking, type Lookalike, type Pause } from './lookalikes.ts';

/** A pause that waits for the test to say so. */
function manualPause() {
  const pending = new Map<number, () => void>();
  let next = 0;
  const pause: Pause = {
    set: (run) => {
      next += 1;
      pending.set(next, run);
      return next;
    },
    clear: (handle) => pending.delete(handle as number),
  };
  return { pause, fire: () => [...pending.values()].forEach((run) => run()), waiting: () => pending.size };
}

const near = (title: string): Lookalike => ({ item_id: title, title, collection_id: 'c1', similarity: 0.9 });
const settle = () => new Promise((resolve) => setImmediate(resolve));

test('a title is asked about once the typing rests, and three at most are shown', async () => {
  const timer = manualPause();
  const asked: string[] = [];
  let shown: readonly Lookalike[] = [];
  const follow = followTitle(async (title) => {
    asked.push(title);
    return [near('a'), near('b'), near('c'), near('d')];
  }, (found) => (shown = found), timer.pause);

  follow.typed('Bo');
  follow.typed('Boo');
  follow.typed('Book the tiler ');
  assert.equal(timer.waiting(), 1, 'every keystroke restarts the pause');
  timer.fire();
  await settle();

  assert.deepEqual(asked, ['Book the tiler']);
  assert.equal(shown.length, 3);
});

test('more typing stops the question on its way, and its answer is never shown', async () => {
  const timer = manualPause();
  const signals: AbortSignal[] = [];
  let release: (found: Lookalike[]) => void = () => {};
  let shown: readonly Lookalike[] | undefined;
  const follow = followTitle((_title, signal) => {
    signals.push(signal);
    return new Promise((resolve) => (release = resolve));
  }, (found) => (shown = found), timer.pause);

  follow.typed('Book the tiler');
  timer.fire();
  follow.typed('Book the plumber');
  assert.equal(signals[0]?.aborted, true);
  release([near('stale')]);
  await settle();
  assert.equal(shown, undefined, 'a stopped question showed its answer');
});

test('too few characters ask nothing and clear what was shown; a failure shows nothing', async () => {
  const timer = manualPause();
  let calls = 0;
  let shown: readonly Lookalike[] = [near('old')];
  const follow = followTitle(async () => {
    calls += 1;
    throw new Error('unreachable');
  }, (found) => (shown = found), timer.pause);

  follow.typed('ab');
  assert.deepEqual(shown, []);
  assert.equal(timer.waiting(), 0);
  follow.typed('abc');
  timer.fire();
  await settle();
  assert.equal(calls, 1);
  assert.deepEqual(shown, []);
  assert.equal(worthAsking('  ab  '), false);
  assert.equal(worthAsking('日本語'), true);
});
