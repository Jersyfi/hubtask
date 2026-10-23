// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { admittedJurisdictions, emptyModel, modelBody, modelOf } from './model.ts';

test('a minimum leaves out the looser declarations, but keeps the stored one', () => {
  assert.deepEqual(admittedJurisdictions('ANY', ''), ['SELF_HOSTED', 'EEA', 'ADEQUACY', 'THIRD_COUNTRY']);
  assert.deepEqual(admittedJurisdictions('EEA', ''), ['SELF_HOSTED', 'EEA']);
  assert.deepEqual(admittedJurisdictions('EEA', 'THIRD_COUNTRY'), ['SELF_HOSTED', 'EEA', 'THIRD_COUNTRY']);
});

test('the one model form leaves out what was not typed, the key above all', () => {
  const body = modelBody({ ...emptyModel('OLLAMA', 'SELF_HOSTED'), baseUrl: ' http://ollama:11434 ', completionModel: 'small' });
  assert.deepEqual(body, { kind: 'OLLAMA', base_url: 'http://ollama:11434', completion_model: 'small', jurisdiction: 'SELF_HOSTED' });
  assert.equal(modelBody({ ...emptyModel('OLLAMA', 'EEA'), apiKey: 'k' }).api_key, 'k');
});

test('a stored model opens with an empty key', () => {
  const draft = modelOf({ kind: 'OPENAI_COMPATIBLE', base_url: 'https://x.example/v1', embedding_model: null, jurisdiction: 'EEA' });
  assert.equal(draft.apiKey, '');
  assert.equal(draft.embeddingModel, '');
  assert.equal(draft.baseUrl, 'https://x.example/v1');
});
