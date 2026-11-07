// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { aiAnsweringOf, aiColumnFigure } from './aianswering.ts';

const own = (processing_allowed: boolean, kind = 'OPENAI_COMPATIBLE') => ({ kind, processing_allowed });

test('which AI answers, in one word', () => {
  assert.equal(aiAnsweringOf({ source: 'OFFERED', offered_model_id: 'm1' }, undefined), 'offered');
  assert.equal(aiAnsweringOf({ source: 'OFFERED' }, own(true)), 'withdrawn', 'an offered source without its model was withdrawn');
  assert.equal(aiAnsweringOf({ source: 'OWN' }, own(true)), 'own');
  assert.equal(aiAnsweringOf({ source: 'OWN' }, own(false)), 'own_not_allowed', 'configured is not consented');
  assert.equal(aiAnsweringOf({ source: 'OWN' }, undefined), 'own_missing');
  assert.equal(aiAnsweringOf({ source: 'OWN' }, own(true, 'NOOP')), 'own_missing', 'the no-op provider answers nothing');
  assert.equal(aiAnsweringOf({ source: 'NONE' }, own(true)), 'none');
  assert.equal(aiAnsweringOf(undefined, own(true)), 'own', 'before the source arrives, the own model');
});

test('the column counts only a source that stopped answering', () => {
  const t = (code: string) => code;
  assert.equal(aiColumnFigure('withdrawn', t), 'app.ai.column_withdrawn');
  for (const quiet of ['offered', 'own', 'own_not_allowed', 'own_missing', 'none'] as const) assert.equal(aiColumnFigure(quiet, t), undefined, quiet);
});

// The frame reads which AI answers and not the provider: the column's figure must not need it.
test('the column figure is the same with or without the provider', () => {
  const t = (code: string) => code;
  const sources = [{ source: 'OWN' }, { source: 'NONE' }, { source: 'OFFERED' }, { source: 'OFFERED', offered_model_id: 'm' }];
  for (const source of sources) {
    for (const provider of [undefined, own(true), own(false)]) {
      assert.equal(aiColumnFigure(aiAnsweringOf(source, provider), t), aiColumnFigure(aiAnsweringOf(source, undefined), t), JSON.stringify(source));
    }
  }
});
