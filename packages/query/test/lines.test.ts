// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-WRK-43 check 1: the browser's twin reads every shared line into the server's tree and writes
// the server's canonical text - the same file core/domain/model/view holds the server to.

import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { formatQuery, parseQuery } from '../src/parse.ts';

const file = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'test', 'querylanguage', 'lines.json');
const { lines } = JSON.parse(fs.readFileSync(file, 'utf8')) as {
  lines: { line: string; query?: Record<string, unknown>; canonical?: string; error?: { code: string; at: number; suggestion?: string } }[];
};

for (const row of lines) {
  test(`the shared line ${JSON.stringify(row.line)}`, () => {
    const parsed = parseQuery(row.line);
    if (row.error) {
      assert.equal(parsed.ok, false);
      if (parsed.ok) return;
      assert.equal(parsed.error.code, row.error.code);
      assert.equal(parsed.error.at, row.error.at);
      assert.equal(parsed.error.params.suggestion, row.error.suggestion);
      return;
    }
    assert.ok(parsed.ok, parsed.ok ? '' : JSON.stringify(parsed.error));
    if (!parsed.ok) return;
    assert.deepEqual(JSON.parse(JSON.stringify(parsed.query)), row.query);
    const text = formatQuery(parsed.query);
    assert.equal(text, row.canonical);
    const again = parseQuery(text ?? '');
    assert.ok(again.ok);
    if (again.ok) assert.deepEqual(again.query, parsed.query);
  });
}
