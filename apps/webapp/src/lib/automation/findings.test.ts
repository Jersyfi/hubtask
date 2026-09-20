// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { cardOf, listHealth, marksOf } from './findings.ts';

test('a finding lands at the card its pointer names', () => {
  assert.equal(cardOf('/trigger/event_type'), 'trigger');
  assert.equal(cardOf('/run_as'), 'run_as');
  assert.equal(cardOf('/conditions/1/expr'), 'conditions/1');
  assert.equal(cardOf('/actions/2/params/then/0/kind'), '2/then/0');
  assert.equal(cardOf('/actions/0/params/label_id'), '0');
  assert.equal(cardOf('/throttle/max_runs_per_hour'), undefined);
});

test('the marks keep the first finding per card, in the catalogue\'s words', () => {
  const words = { t: (code: string, params?: Record<string, string | number>) => `${code}(${params?.kind ?? ''})`, has: (code: string) => code !== 'unknown.code' };
  const marks = marksOf(words, [
    { level: 'ATTENTION', path: '/actions/0/params/label_id', code: 'automation.finding.reference_gone', params: { kind: 'label', id: 'x' } },
    { level: 'ATTENTION', path: '/actions/0/params/bucket_id', code: 'automation.finding.reference_gone', params: { kind: 'bucket', id: 'y' } },
    { level: 'BROKEN', path: '/actions/1/kind', code: 'unknown.code' },
  ]);
  assert.deepEqual([...marks], [['0', 'automation.finding.reference_gone(label)'], ['1', 'unknown.code']]);
});

test('the list judges health from the findings and the switch alone', () => {
  assert.equal(listHealth({ enabled: true, findings: [], failure_count: 0 }), 'works');
  assert.equal(listHealth({ enabled: true, findings: [], failure_count: 2 }), 'sometimes');
  assert.equal(listHealth({ enabled: false, findings: [], failure_count: 0 }), 'off');
  assert.equal(listHealth({ enabled: true, findings: [{ level: 'ATTENTION', path: '/actions/0', code: 'c' }], failure_count: 0 }), 'attention');
  assert.equal(listHealth({ enabled: false, findings: [{ level: 'BROKEN', path: '/actions/0/kind', code: 'c' }], failure_count: 0 }), 'broken');
});
