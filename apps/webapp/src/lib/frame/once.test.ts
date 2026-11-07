// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { once } from './once.ts';

test('two callers at once share one import', async () => {
  let imports = 0;
  const load = once(async () => {
    imports++;
    await Promise.resolve();
    return { default: 'screen' };
  });
  const [a, b] = await Promise.all([load(), load()]);
  assert.equal(imports, 1);
  assert.equal(a, b);
});

test('a failed import is asked again', async () => {
  let imports = 0;
  const load = once(async () => {
    imports++;
    if (imports === 1) throw new Error('offline');
    return 'screen';
  });
  await assert.rejects(load());
  assert.equal(await load(), 'screen');
  assert.equal(imports, 2);
});
