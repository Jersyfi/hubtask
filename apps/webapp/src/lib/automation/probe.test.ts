// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Run, TestResult } from '../data/runs.svelte.ts';
import { newStep, type Step } from './model.ts';
import { framesOfRun, framesOfTest, healthOf } from './probe.ts';

const CHAIN: Step[] = [
  newStep('ADD_LABEL'),
  { kind: 'BRANCH', params: { condition: 'has(item.due_at)' }, then: [newStep('ADD_COMMENT')], else: [newStep('WAIT'), newStep('STOP')] },
  newStep('SEND_WEBHOOK'),
];

test('a dry run is drawn in the order the run would visit the cards, both arms of a branch included', () => {
  const result: TestResult = {
    matched: true,
    condition_results: [{ index: 0, matched: true }, { index: 1, matched: true }],
    actions: [
      { path: '0', kind: 'ADD_LABEL', would_run: true },
      { path: '1', kind: 'BRANCH', would_run: true, matched: false } as never,
      { path: '1/then/0', kind: 'ADD_COMMENT', would_run: false },
      { path: '1/else/0', kind: 'WAIT', would_run: true },
      { path: '1/else/1', kind: 'STOP', would_run: true },
      { path: '2', kind: 'SEND_WEBHOOK', would_run: false },
    ],
  };
  const { frames, outcome } = framesOfTest(CHAIN, result);
  assert.deepEqual(
    frames.map((frame) => `${frame.key}:${frame.verdict.state}:${frame.verdict.code.replace('app.flow.verdict_', '')}`),
    ['trigger:yes:fires', 'conditions/0:yes:held', 'conditions/1:yes:held', '0:yes:would_run', '1:no:otherwise', '1/then/0:skipped:skipped', '1/else/0:yes:parks', '1/else/1:yes:ends', '2:skipped:skipped'],
  );
  assert.deepEqual(outcome, { status: 'SUCCEEDED', code: 'app.flow.outcome_would_run', params: { count: 3 } });
});

test('a dry run whose gate did not hold stops at the gate', () => {
  const { frames, outcome } = framesOfTest(CHAIN, { matched: false, condition_results: [{ index: 0, matched: false }], actions: [] });
  assert.deepEqual(frames.map((frame) => `${frame.key}:${frame.verdict.state}`), ['trigger:yes', 'conditions/0:no']);
  assert.equal(outcome.status, 'SKIPPED');
});

test('a recorded run is drawn from its own log, with what it never reached marked', () => {
  const run: Run = {
    id: 'run-1', rule_id: 'r1', trigger: 'EVENT', status: 'FAILED', started_at: '2026-09-20T10:00:00Z', causation_depth: 1,
    condition_results: [{ index: 0, matched: true }],
    action_results: [
      { index: 0, kind: 'ADD_LABEL', path: '0', status: 'SUCCEEDED' },
      { index: 1, kind: 'BRANCH', path: '1', matched: true, status: 'SUCCEEDED' },
      { index: 2, kind: 'ADD_COMMENT', path: '1/then/0', status: 'FAILED', error_code: 'items.not_found' },
    ],
  };
  const { frames, outcome } = framesOfRun(CHAIN, run);
  assert.deepEqual(
    frames.map((frame) => `${frame.key}:${frame.verdict.state}`),
    ['trigger:yes', 'conditions/0:yes', '0:yes', '1:yes', '1/then/0:no', '1/else/0:skipped', '1/else/1:skipped', '2:skipped'],
  );
  assert.equal(outcome.code, 'app.runs.status_failed');
});

test('health is the arithmetic over the last runs, with findings and the switch before it', () => {
  const runs = (...statuses: string[]) => statuses.map((status) => ({ status }));
  assert.equal(healthOf({ enabled: true, findings: [], runs: runs('SUCCEEDED', 'SKIPPED', 'THROTTLED') }), 'works');
  assert.equal(healthOf({ enabled: true, findings: [], runs: runs('SUCCEEDED', 'FAILED', 'SUCCEEDED') }), 'sometimes');
  assert.equal(healthOf({ enabled: true, findings: [], runs: runs('FAILED', 'FAILED', 'SUCCEEDED') }), 'failing');
  assert.equal(healthOf({ enabled: true, findings: [], runs: runs('RUNNING', 'WAITING') }), 'unknown');
  assert.equal(healthOf({ enabled: false, findings: [], runs: runs('SUCCEEDED') }), 'off');
  assert.equal(healthOf({ enabled: true, findings: [{ level: 'ATTENTION' }], runs: runs('SUCCEEDED') }), 'attention');
  assert.equal(healthOf({ enabled: false, findings: [{ level: 'BROKEN' }], runs: [] }), 'broken');
});
