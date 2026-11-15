// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { keyCode, lineOf } from './queryline.ts';

const keys = [
  { key: 'type', value: 'kind', kinds: ['task'], examples: ['type: task'], values: ['task', 'ticket'] },
  { key: 'desk.sla', value: 'choice', kinds: ['ticket'], examples: ['desk.sla: breached'], values: [] },
];
const names: Record<string, string> = { 'app.qkey.type': 'Art' };
const t = (code: string) => names[code] ?? code;
const has = (code: string) => code in names;

// UC-WRK-43 check 1. One line: a typed key becomes a chip named in the reader's words, words stay text, and the
// line written back is the line typed (Q-1, Q-2).
test('the search line cuts keys into named chips and writes them back', () => {
  const { language, catalogue, known } = lineOf(keys, t, has);
  const { tokens, rest } = language.parse('passkey type: ticket ');
  assert.equal(rest, '');
  assert.deepEqual(tokens.map((token) => token.kind), ['text', 'chip']);
  const chip = tokens[1];
  assert.ok(chip?.kind === 'chip' && chip.fieldLabel === 'Art');
  assert.equal(tokens.map((token) => language.format(token)).join(' '), 'passkey type: ticket');
  assert.deepEqual(catalogue.fields.map((field) => field.label), ['Art', 'desk.sla']);
  assert.deepEqual(catalogue.values('type', '').map((value) => value.value), ['task', 'ticket']);
  assert.ok(known.has('desk.sla'));
  assert.equal(keyCode('desk.sla'), 'app.qkey.desk_sla');
});

test('the entry search reads the line with its keys written close', async () => {
  const { compactLine } = await import('./queryline.ts');
  assert.equal(compactLine('boiler is: open who: me'), 'boiler is:open who:me');
  assert.equal(compactLine('in: "Team Atlas" -label: x'), 'in:"Team Atlas" -label:x');
  assert.equal(compactLine('Re: Besprechung'), 'Re:Besprechung');
});
