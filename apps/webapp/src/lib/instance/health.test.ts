// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { backlogTiles, featureNameCode, instancePageOfFeature, schemaState } from './health.ts';

test('the schema state is said for each migration status, and not at all where none was answered', () => {
  const cases = [
    ['ok', 'up_to_date'], ['behind', 'waiting'], ['ahead', 'newer'], ['too_old', 'too_old'],
  ] as const;
  for (const [status, said] of cases) {
    assert.equal(schemaState({ status: 'ok', version: '1', migration: { status } }), said);
  }
  assert.equal(schemaState({ status: 'ok', version: '1' }), undefined);
  assert.equal(schemaState(undefined), undefined);
});

test('backlogs are absent until counted, and only a dead letter or a late outbox warns', () => {
  assert.deepEqual(backlogTiles({ status: 'ok', version: '1' }), []);
  const quiet = backlogTiles({
    status: 'ok', version: '1',
    backlogs: { dead_letter_total: 0, outbox_lag_seconds: 60, job_queue_depth: 9000, webhook_retry_backlog: 400 },
  });
  assert.deepEqual(quiet.map((tile) => [tile.id, tile.isAbove]), [
    ['dead_letters', false], ['outbox_lag', false], ['job_queue', false], ['webhook_retries', false],
  ]);
  const loud = backlogTiles({
    status: 'degraded', version: '1', backlogs: { dead_letter_total: 2, outbox_lag_seconds: 61 },
  });
  assert.deepEqual(loud.map((tile) => [tile.id, tile.isAbove, tile.href]), [
    ['dead_letters', true, '/instance/dead-letters'], ['outbox_lag', true, undefined],
  ]);
});

test('a degraded feature links to the installation screen that changes it, where there is one', () => {
  assert.equal(instancePageOfFeature('semantic_search'), '/instance/ai');
  assert.equal(instancePageOfFeature('media'), undefined);
});

test('a feature the probes name is said in words; an unknown one keeps its key', () => {
  assert.equal(featureNameCode('media'), 'app.instance.feature.media');
  assert.equal(featureNameCode('something_new'), undefined);
});
