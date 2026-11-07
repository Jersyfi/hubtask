// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { readLine, writeLine } from './queryline.ts';

const kinds: Record<string, string> = { depth: 'integer', is_completed: 'boolean', 'custom_fields.priority': 'custom' };
const kindOf = (field: string) => kinds[field];

// UC-WRK-17 check 12: the line reads into the document the server reads it into
// (core/domain/model/view/Line_test.go holds the same cases).
test('a line is read into the filter and the order', () => {
  assert.deepEqual(readLine('status = open AND custom_fields.priority >= 3 ORDER BY due_at DESC NULLS FIRST', kindOf), {
    ok: true,
    filter: { op: 'AND', nodes: [
      { field: 'status', op: 'EQ', value: 'open' },
      { field: 'custom_fields.priority', op: 'GTE', value: 3 },
    ] },
    sort: [{ field: 'due_at', dir: 'DESC', nulls: 'FIRST' }],
  });
  assert.deepEqual(readLine("(status != done OR is_completed = false) and not title ~ 'ord\\'er'", kindOf), {
    ok: true,
    filter: { op: 'AND', nodes: [
      { op: 'OR', nodes: [{ field: 'status', op: 'NEQ', value: 'done' }, { field: 'is_completed', op: 'EQ', value: false }] },
      { op: 'NOT', nodes: [{ field: 'title', op: 'CONTAINS', value: "ord'er" }] },
    ] },
    sort: [],
  });
  assert.deepEqual(readLine('due_at IS NOT EMPTY', kindOf), { ok: true, filter: { op: 'NOT', nodes: [{ field: 'due_at', op: 'IS_NULL' }] }, sort: [] });
  assert.deepEqual(readLine('depth BETWEEN 1 AND 2', kindOf), { ok: true, filter: { field: 'depth', op: 'BETWEEN', value: [1, 2] }, sort: [] });
  assert.deepEqual(readLine("assignee_id IN (@me, 'x')", kindOf), { ok: true, filter: { field: 'assignee_id', op: 'IN', value: ['@me', 'x'] }, sort: [] });
  assert.deepEqual(readLine(''), { ok: true, filter: undefined, sort: [] });
});

test('a line that is not one says the character it stops at', () => {
  assert.deepEqual(readLine('status ='), { ok: false, at: 9 });
  assert.deepEqual(readLine("status = 'open"), { ok: false, at: 10 });
  assert.deepEqual(readLine('ä = b c'), { ok: false, at: 7 });
  assert.deepEqual(readLine('status open'), { ok: false, at: 8 });
  assert.equal(readLine(`${'('.repeat(40)}a = b`).ok, false);
});

test('the conditions are written as a line that reads back to them', () => {
  for (const line of [
    "status = 'open' AND (custom_fields.priority >= 3 OR title CONTAINS 'it\\'s') ORDER BY due_at DESC",
    "NOT (status IN ('a', 'b') AND due_at IS EMPTY)",
    "depth BETWEEN 1 AND 2 AND labels CONTAINS ALL ('x')",
  ]) {
    const read = readLine(line, kindOf);
    assert.ok(read.ok, line);
    const written = writeLine(read.filter, read.sort);
    assert.ok(written !== undefined, line);
    assert.deepEqual(readLine(written, kindOf), read, written);
  }
  assert.equal(writeLine({ field: 'a b', op: 'EQ', value: 'x' }), undefined);
});
