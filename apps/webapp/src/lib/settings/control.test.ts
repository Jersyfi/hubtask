// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One setting's control drawn from the catalogue (configuration/ui.md A1, A2): the editor by kind,
// the words of every choice, and why a value cannot be changed here.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { SettingDefinition } from '@hubtask/sync-engine';

import { isNamed } from './catalogue.ts';
import { boundsWords, choiceWords, countFrom, editorOf, lockedWords, shownValue, toggled } from './control.ts';

const writers: SettingDefinition['writers'] = [{ level: 'WORKSPACE', permission: 'APPROVE_TIME' }];
const holdsAll = () => true;

test('each kind is drawn with its own editor, and a part of another control with none', () => {
  assert.equal(editorOf({ kind: 'COUNT' }), 'count');
  assert.equal(editorOf({ kind: 'DATE' }), 'date');
  assert.equal(editorOf({ kind: 'CHOICES' }), 'choices');
  assert.equal(editorOf({ kind: 'TIME_ZONE' }), 'zone');
  assert.equal(editorOf({ kind: 'STRUCTURE' }), 'none');
});

test('a value a level above locked says which level, before any permission is asked', () => {
  assert.deepEqual(lockedWords({ writers }, { value: 'FULL', locked_by: 'PLAN' }, 'WORKSPACE', holdsAll), { code: 'app.source.locked_sentence_plan' });
  assert.deepEqual(lockedWords({ writers }, { value: 'FULL', locked_by: 'INSTANCE' }, 'WORKSPACE', holdsAll), { code: 'app.source.locked_sentence_instance' });
});

test('a reader without the writer\'s permission at the level sees the value and may not change it', () => {
  assert.deepEqual(lockedWords({ writers }, { value: 'FULL' }, 'WORKSPACE', () => false), { code: 'app.setting.not_yours' });
  assert.deepEqual(lockedWords({ writers }, { value: 'FULL' }, 'HUB', holdsAll), { code: 'app.setting.not_yours' });
  assert.equal(lockedWords({ writers }, { value: 'FULL', source: 'WORKSPACE' }, 'WORKSPACE', (p) => p === 'APPROVE_TIME'), undefined);
});

test('a count is read as a whole number, empty as undecided, anything else as typed', () => {
  assert.equal(countFrom(' 12 '), 12);
  assert.equal(countFrom(''), undefined);
  assert.equal(countFrom('1.5'), '1.5');
});

test('a count\'s bounds are said as its hint', () => {
  assert.deepEqual(boundsWords({ minimum: 8, maximum: 128 }), { code: 'app.setting.between', params: { minimum: '8', maximum: '128' } });
  assert.deepEqual(boundsWords({ minimum: 0 }), { code: 'app.setting.at_least', params: { minimum: '0' } });
  assert.equal(boundsWords({}), undefined);
});

test('a list of choices keeps the catalogue\'s order when one is switched on', () => {
  assert.deepEqual(toggled(['PASSWORD', 'OIDC', 'PASSKEY'], ['PASSKEY'], 'PASSWORD', true), ['PASSWORD', 'PASSKEY']);
  assert.deepEqual(toggled(['PASSWORD', 'OIDC', 'PASSKEY'], ['PASSWORD', 'PASSKEY'], 'PASSKEY', false), ['PASSWORD']);
});

test('the value in force is said in the words of its choices', () => {
  const t = (code: string, params?: Record<string, string>) => (params ? `${code}(${Object.values(params).join(',')})` : code);
  assert.equal(shownValue({ key: 'time.visibility', kind: 'CHOICE' }, 'FULL', t), 'app.time_settings.visibility_FULL');
  assert.equal(shownValue({ key: 'config.agent_apply', kind: 'CHOICE' }, 'NEVER', t), 'app.changes.agent_apply_option(NEVER)');
  assert.equal(shownValue({ key: 'sign_in.methods', kind: 'CHOICES' }, ['PASSWORD'], t), 'app.instance.choice.sign_in.methods.PASSWORD');
  assert.equal(shownValue({ key: 'time.locked_through', kind: 'DATE' }, null, t), 'app.setting.no_value');
});

// The server's own catalogue, as `make generate` writes it: a choice the server declares without
// words in either language would be drawn as its code.
test('every choice of every setting the server declares has words in every language', async () => {
  const { default: generated } = await import('./catalogue.gen.json', { with: { type: 'json' } });
  const { default: english } = await import('../../../../../locales/en.json', { with: { type: 'json' } });
  const { default: german } = await import('../../../../../locales/de.json', { with: { type: 'json' } });
  const missing: string[] = [];
  for (const definition of generated.settings as unknown as SettingDefinition[]) {
    if (!isNamed(definition)) continue;
    for (const option of definition.options ?? []) {
      const { code } = choiceWords(definition.key, option);
      if (!(code in english)) missing.push(`en ${code}`);
      if (!(code in german)) missing.push(`de ${code}`);
    }
  }
  assert.deepEqual(missing, []);
});
