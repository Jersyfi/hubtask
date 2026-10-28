// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import test from 'node:test';

import { canonical, securityTxt } from '../src/lib/securityTxt.ts';

const fields = (text) => text.trim().split('\n').map((line) => line.split(': ', 2));

test('the file carries the fields RFC 9116 requires, each well formed', () => {
  const lines = fields(securityTxt(new Date('2026-10-28T12:00:00Z')));
  const names = lines.map(([name]) => name);
  assert.ok(names.includes('Contact'), 'Contact is required');
  assert.equal(names.filter((name) => name === 'Expires').length, 1, 'Expires appears exactly once');
  for (const [name, value] of lines.filter(([name]) => name === 'Contact')) {
    assert.match(value, /^(https:|mailto:)/, `${name} is a URI`);
  }
  assert.deepEqual(lines.find(([name]) => name === 'Canonical'), ['Canonical', canonical]);
});

test('Expires lies less than a year after the build', () => {
  const builtAt = new Date('2026-10-28T12:00:00Z');
  const [, value] = fields(securityTxt(builtAt)).find(([name]) => name === 'Expires');
  const expires = new Date(value);
  assert.ok(expires > builtAt, 'not already expired');
  assert.ok(expires.getTime() - builtAt.getTime() < 365 * 24 * 60 * 60 * 1000, 'under a year ahead');
  assert.match(value, /^\d{4}-\d{2}-\d{2}T00:00:00Z$/);
});

test('the contacts are the ones SECURITY.md names', async () => {
  const { readFile } = await import('node:fs/promises');
  const policy = await readFile(new URL('../../../SECURITY.md', import.meta.url), 'utf8');
  for (const [, value] of fields(securityTxt(new Date())).filter(([name]) => name === 'Contact')) {
    assert.ok(policy.includes(value.replace(/^mailto:/, '')), `${value} is not in SECURITY.md`);
  }
});
