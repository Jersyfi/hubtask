// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chipCodes } from './source.ts';

const member = { operates: false };
const operator = { operates: true };

test('each source has its own words', () => {
  for (const [source, level] of [
    ['WORKSPACE', 'workspace'],
    ['DEFAULT', 'default'],
    ['INSTANCE', 'instance'],
    ['PLAN', 'plan'],
    ['EXCEPTION', 'exception'],
  ]) {
    const chip = chipCodes({ source, lock: null }, member);
    assert.equal(chip?.label, `app.source.label_${level}`);
    assert.equal(chip?.source, `app.source.sentence_${level}`);
    assert.equal(chip?.isLocked, false);
  }
});

test('a lock names the level that holds it, whatever the stored source', () => {
  assert.deepEqual(chipCodes({ source: 'WORKSPACE', lock: 'INSTANCE' }, member), {
    label: 'app.source.locked_instance',
    isLocked: true,
    source: 'app.source.locked_sentence_instance',
    whomToAsk: 'app.source.ask_operator',
  });
  assert.equal(chipCodes({ source: 'INSTANCE', lock: 'PLAN' }, member)?.label, 'app.source.locked_plan');
});

test('whom to ask only where the value cannot be changed here', () => {
  assert.equal(chipCodes({ source: 'INSTANCE', lock: null }, member)?.whomToAsk, undefined);
  assert.equal(chipCodes({ source: 'WORKSPACE', lock: null }, member)?.whomToAsk, undefined);
  assert.equal(chipCodes({ source: 'PLAN', lock: 'PLAN' }, member)?.whomToAsk, 'app.source.ask_provider');
  assert.equal(chipCodes({ source: 'EXCEPTION' }, member)?.whomToAsk, 'app.source.ask_operator');
});

test('somebody who runs the installation is not sent to ask themselves', () => {
  assert.equal(chipCodes({ source: 'INSTANCE', lock: 'INSTANCE' }, operator)?.whomToAsk, 'app.source.ask_yourself');
  assert.equal(chipCodes({ source: 'PLAN', lock: 'PLAN' }, operator)?.whomToAsk, 'app.source.ask_yourself_plan');
});

test('no source, or one this client does not know, draws no chip', () => {
  assert.equal(chipCodes({}, member), undefined);
  assert.equal(chipCodes({ source: null, lock: null }, member), undefined);
  assert.equal(chipCodes({ source: 'SOMETHING_NEW' }, member), undefined);
});
