// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The job watcher's arithmetic, which is the half worth asserting: a loop that never stops and a
// loop that gives up too early both look the same in a browser for the first ten seconds.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TransportError } from '@hubtask/sync-engine';

import {
  isTerminal,
  mayCancel,
  pollDelay,
  worthRetrying,
  POLL_FIRST_MS,
  POLL_MAX_MS,
  type JobStatus,
} from './jobs.ts';

test('the three terminal states end the watch and the two live ones do not', () => {
  const terminal: JobStatus[] = ['SUCCEEDED', 'FAILED', 'CANCELLED'];
  const live: JobStatus[] = ['QUEUED', 'RUNNING'];
  for (const status of terminal) assert.ok(isTerminal(status), status);
  for (const status of live) assert.ok(!isTerminal(status), status);
});

test('only work that is still going can be cancelled', () => {
  // The server answers `409` for the rest, and a button offered for a finished job would be a
  // button that lets somebody believe they prevented something that had already happened.
  assert.ok(mayCancel('QUEUED'));
  assert.ok(mayCancel('RUNNING'));
  for (const status of ['SUCCEEDED', 'FAILED', 'CANCELLED'] as JobStatus[]) {
    assert.ok(!mayCancel(status), status);
  }
});

test('the wait grows and then stops growing', () => {
  assert.equal(pollDelay(0), POLL_FIRST_MS);
  let previous = pollDelay(0);
  for (let attempt = 1; attempt < 30; attempt++) {
    const delay = pollDelay(attempt);
    assert.ok(delay >= previous, `attempt ${attempt} waited less than the one before`);
    assert.ok(delay <= POLL_MAX_MS, `attempt ${attempt} exceeded the ceiling`);
    previous = delay;
  }
  assert.equal(pollDelay(100), POLL_MAX_MS, 'the ceiling is reached and held');
});

test('a negative attempt is the first wait rather than a shorter one', () => {
  // Defensive, and cheap: an off-by-one somewhere else must not turn into a request every
  // microsecond.
  assert.equal(pollDelay(-3), POLL_FIRST_MS);
});

test('a refusal ends the watch and a timeout does not', () => {
  const problem = (status: number) => new TransportError('problem', { status, code: 'x' });
  assert.ok(!worthRetrying(problem(403)), 'a refusal will not become an answer by being asked again');
  assert.ok(!worthRetrying(problem(404)));
  assert.ok(worthRetrying(problem(429)), 'too many requests means later, not no');
  assert.ok(worthRetrying(problem(408)));
  assert.ok(worthRetrying(problem(503)), 'the server is shedding load; the job is still running');
  assert.ok(worthRetrying(new TransportError('timeout')));
  assert.ok(worthRetrying(new TransportError('offline')), 'a dead network says nothing about the job');
});

test('something that is not a transport failure is not retried', () => {
  // A programming mistake inside the watcher must not become an infinite loop of requests.
  assert.ok(!worthRetrying(new Error('a bug')));
  assert.ok(!worthRetrying(undefined));
});
