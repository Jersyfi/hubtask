// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The create act of a screen is its PageHeader's primary (user-experience.md §5.2): a create among
// the secondary acts while the head has no primary is the screen hiding its main door. Read from
// the source, because the shape is what a new screen copies from its neighbour.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const VIEWS = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src', 'views');

/** Each `<PageHeader …>` tag's own source, attributes and all, up to its closing `/>`. */
function headsOf(source) {
  const heads = [];
  let from = 0;
  for (;;) {
    const start = source.indexOf('<PageHeader', from);
    if (start < 0) return heads;
    let depth = 0;
    let end = start;
    for (; end < source.length; end++) {
      const char = source[end];
      if (char === '{') depth++;
      else if (char === '}') depth--;
      else if (depth === 0 && source.startsWith('/>', end)) break;
    }
    heads.push(source.slice(start, end));
    from = end;
  }
}

test('no screen puts its create act among the secondary acts of a head without a primary', () => {
  const found = [];
  for (const name of fs.readdirSync(VIEWS).filter((file) => file.endsWith('.svelte'))) {
    const source = fs.readFileSync(path.join(VIEWS, name), 'utf8');
    for (const head of headsOf(source)) {
      const hasPrimary = /\bprimary=|\{primary\}/.test(head);
      const secondary = /\bsecondary=\{([\s\S]*)/.exec(head)?.[1] ?? '';
      if (!hasPrimary && /icon: 'plus'/.test(secondary)) found.push(name);
    }
  }
  assert.deepEqual(found, [], 'make the create the head\'s primary');
});
