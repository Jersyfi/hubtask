// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { parseTicketKey, ticketKeyOf } from './ticket.ts';

test('a key is the prefix and the number, the handle in capitals where there is no prefix', () => {
  assert.equal(ticketKeyOf({ handle: 'incidents', key_prefix: 'INC' }, 42), 'INC-42');
  assert.equal(ticketKeyOf({ handle: 'einkauf', key_prefix: null }, 42), 'EINKAUF-42');
  assert.equal(ticketKeyOf({ handle: 'einkauf' }, 7), 'EINKAUF-7');
  assert.equal(ticketKeyOf(undefined, 42), undefined);
  assert.equal(ticketKeyOf({ handle: null }, 42), undefined);
  assert.equal(ticketKeyOf({ handle: 'einkauf' }, null), undefined);
});

test('a key is read back in any case and in brackets', () => {
  assert.deepEqual(parseTicketKey('EINKAUF-42'), { handle: 'einkauf', number: 42 });
  assert.deepEqual(parseTicketKey(' [einkauf-7] '), { handle: 'einkauf', number: 7 });
  assert.equal(parseTicketKey('Milch kaufen'), undefined);
  assert.equal(parseTicketKey('42-EINKAUF'), undefined);
});
