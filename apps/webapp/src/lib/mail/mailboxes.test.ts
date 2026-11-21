// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Mailbox } from '../data/mailintake.svelte.ts';

// The store's fields, and those of the modules it imports, are runes, which only the Svelte
// compiler knows. The order under test depends on no reactivity, so runes that hand their value
// through are enough to run the class; a derived value is then the one of its first reading.
const rune = Object.assign((value: unknown) => value, { raw: (value: unknown) => value });
Object.assign(globalThis, { $state: rune, $derived: Object.assign((value: unknown) => value, { by: (read: () => unknown) => read() }) });

const CONNECTED = { id: 'mb-1', address: 'familie@gmail.com' } as unknown as Mailbox;

test('a read answered before the completion and applied after it keeps the new mailbox', async () => {
  const { engine } = await import('../data/engine.ts');
  const { MailboxList } = await import('./mailboxes.svelte.ts');

  // The server answered the read before the mailbox existed; the answer arrives last.
  let answerRead: (read: { data: unknown }) => void = () => {};
  const readAnswered = new Promise<{ data: unknown }>((resolve) => (answerRead = resolve));
  const stood = { refresh: engine.refresh, mutate: engine.mutate };
  Object.assign(engine, { refresh: () => readAnswered, mutate: async () => CONNECTED });
  try {
    const list = new MailboxList();
    const reading = list.read();
    let isDone = false;
    const completing = list.complete({ state: 's-1', code: 'c-1' }, () => (isDone = true));
    // The completion's answer is in before the read's: let everything that can run, run.
    await new Promise((resolve) => setImmediate(resolve));
    answerRead({ data: { mailboxes: [], presets: [] } });
    await Promise.all([reading, completing]);

    assert.deepEqual(list.mailboxes, [CONNECTED]);
    assert.equal(list.oauthFailure, undefined);
    assert.equal(isDone, true);
  } finally {
    Object.assign(engine, stood);
  }
});
