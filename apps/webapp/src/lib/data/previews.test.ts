// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A preview writes nothing, so it reloads nothing. The engine reads a `mutate` without `invalidates`
// as "everything may have changed" and reloads every open read; a screen that previews again when
// its reads change then never comes to rest. Read from the source, so every store is held.

import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const dir = fileURLToPath(new URL('./', import.meta.url));

/** Every `engine.mutate(…)` call in the text, parentheses counted. */
function mutateCalls(text: string): string[] {
  const calls: string[] = [];
  for (const match of text.matchAll(/engine\.mutate\b[^(]*\(/gu)) {
    let depth = 0;
    for (let i = match.index + match[0].length - 1; i < text.length; i += 1) {
      if (text[i] === '(') depth += 1;
      else if (text[i] === ')' && (depth -= 1) === 0) {
        calls.push(text.slice(match.index, i + 1));
        break;
      }
    }
  }
  return calls;
}

test('every preview through the engine names the reads it invalidates', () => {
  const missing: string[] = [];
  let previews = 0;
  for (const name of readdirSync(dir).filter((each) => each.endsWith('.ts') && !each.endsWith('.test.ts'))) {
    for (const call of mutateCalls(readFileSync(`${dir}${name}`, 'utf8'))) {
      if (!call.includes(':preview')) continue;
      previews += 1;
      if (!/invalidates:\s*\[\s*\]/u.test(call)) missing.push(`${name}: ${call.split('\n')[0]}`);
    }
  }
  assert.ok(previews >= 4, `found only ${previews} previews; the scan no longer reads the stores`);
  assert.deepEqual(missing, []);
});
