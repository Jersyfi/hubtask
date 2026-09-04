// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The acceptance sentence this file is: "the filter editor is built from `query_fields` and
// changes when the manifest does, proved by a test".
//
// So there are two manifests below and nothing in between. What the editor offers is a function of
// the document, and the test that matters is that the same code answers differently when the
// document does — because the failure it guards against is the one that looks fine on the
// installation the developer had.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities } from '@hubtask/sync-engine';

import {
  fieldNamed,
  filterOf,
  filterableFields,
  groupOf,
  groupableFields,
  isSendable,
  sortOf,
  sortableFields,
  takesList,
  takesValue,
  textLanguages,
} from './query.ts';

const field = (
  name: string,
  kind: string,
  operators: string[],
  extra: Record<string, unknown> = {},
) => ({ field: name, kind, operators, nullable: false, sortable: false, groupable: false, ...extra });

/** What an installation of this version reports. */
const today = {
  query_fields: [
    field('title', 'text', ['CONTAINS', 'STARTS_WITH', 'MATCHES'], { sortable: true }),
    field('is_completed', 'boolean', ['EQ']),
    field('bucket_id', 'id', ['EQ', 'IN'], { nullable: true, groupable: true }),
    field('order_key', 'string', [], { sortable: true }),
    field('label_ids', 'id_set', ['CONTAINS_ANY', 'CONTAINS_ALL']),
  ],
  text_languages: ['en', 'de'],
} as unknown as Capabilities;

/** The same installation a milestone later: one more field, one more operator, one more language. */
const tomorrow = {
  query_fields: [
    ...(today.query_fields ?? []),
    field('due_at', 'timestamp', ['LT', 'LTE', 'GT', 'GTE', 'BETWEEN', 'IS_NULL'], {
      nullable: true,
      sortable: true,
    }),
  ],
  text_languages: ['en', 'de', 'ja'],
} as unknown as Capabilities;

// --- the editor is the manifest's -------------------------------------------------------------

test('what the editor offers is what the installation reports', () => {
  assert.deepEqual(
    filterableFields(today).map((each) => each.field),
    ['title', 'is_completed', 'bucket_id', 'label_ids'],
  );
  // `order_key` declares no operator — "empty for a field that may only be ordered or grouped by" —
  // so it is offered for sorting and never as a condition nobody could complete.
  assert.deepEqual(sortableFields(today).map((each) => each.field), ['title', 'order_key']);
  assert.deepEqual(groupableFields(today).map((each) => each.field), ['bucket_id']);
});

test('and it changes when the manifest does, with nothing recompiled', () => {
  // The whole point. A hard-coded list would answer the same for both of these.
  assert.ok(!filterableFields(today).some((each) => each.field === 'due_at'));
  assert.ok(filterableFields(tomorrow).some((each) => each.field === 'due_at'));
  assert.deepEqual(
    sortableFields(tomorrow).map((each) => each.field),
    ['title', 'order_key', 'due_at'],
  );
  assert.deepEqual([...textLanguages(today)], ['en', 'de']);
  assert.deepEqual([...textLanguages(tomorrow)], ['en', 'de', 'ja']);
});

test('a manifest that has not arrived offers nothing rather than a default', () => {
  // Nothing is knowable before the manifest is read, and a guess in the permissive direction is
  // what the manifest exists to prevent.
  assert.deepEqual(filterableFields(undefined), []);
  assert.equal(fieldNamed(undefined, 'title'), undefined);
  assert.deepEqual(textLanguages(undefined), []);
});

// --- an unknown field is never sent -----------------------------------------------------------

test('a condition on a field the installation does not report is not sent', () => {
  assert.equal(isSendable(today, { field: 'due_at', op: 'LT', value: '@today' }), false);
  assert.equal(filterOf(today, [{ field: 'due_at', op: 'LT', value: '@today' }]), undefined);
  // The same condition against the manifest that reports it is sent.
  assert.deepEqual(filterOf(tomorrow, [{ field: 'due_at', op: 'LT', value: '@today' }]), {
    op: 'LT',
    field: 'due_at',
    value: '@today',
  });
});

test('an operator the field does not declare is not sent either', () => {
  // `title` has no `EQ` here. Sending it would be `query.operator_unsupported` for a row the editor
  // itself put on the screen.
  assert.equal(isSendable(today, { field: 'title', op: 'EQ', value: 'Milk' }), false);
  assert.equal(isSendable(today, { field: 'title', op: 'CONTAINS', value: 'Milk' }), true);
});

test('IS_NULL is only sent for a field that can be absent', () => {
  // "Whether the field can be absent, and IS_NULL therefore means something" — the contract's own
  // words on `nullable`.
  assert.equal(isSendable(tomorrow, { field: 'due_at', op: 'IS_NULL', value: '' }), true);
  assert.equal(isSendable(today, { field: 'title', op: 'IS_NULL', value: '' }), false);
});

test('a row nobody has finished typing is not a condition', () => {
  // `EQ ""` asks for the entries whose title is the empty string, which is not what an empty input
  // means. The unfinished row waits rather than narrowing the list to nothing.
  assert.equal(isSendable(today, { field: 'title', op: 'CONTAINS', value: '   ' }), false);
  assert.equal(filterOf(today, [{ field: 'title', op: 'CONTAINS', value: '' }]), undefined);
});

// --- the document ------------------------------------------------------------------------------

test('one condition is the leaf, and several are an AND of them', () => {
  // A combination of one is a node the grammar allows and a reader would never have asked for.
  assert.deepEqual(filterOf(today, [{ field: 'title', op: 'CONTAINS', value: 'milk' }]), {
    op: 'CONTAINS',
    field: 'title',
    value: 'milk',
  });
  assert.deepEqual(
    filterOf(today, [
      { field: 'title', op: 'CONTAINS', value: 'milk' },
      { field: 'is_completed', op: 'EQ', value: 'false' },
    ]),
    {
      op: 'AND',
      nodes: [
        { op: 'CONTAINS', field: 'title', value: 'milk' },
        { op: 'EQ', field: 'is_completed', value: false },
      ],
    },
  );
});

test('a boolean and an integer travel as themselves, not as text', () => {
  // The two kinds the contract types as something other than a string. A string for either is a
  // 422 rather than a match.
  assert.deepEqual(filterOf(today, [{ field: 'is_completed', op: 'EQ', value: 'true' }]), {
    op: 'EQ',
    field: 'is_completed',
    value: true,
  });
});

test('a list operator takes a list, split where a person would split it', () => {
  assert.deepEqual(
    filterOf(today, [{ field: 'label_ids', op: 'CONTAINS_ANY', value: 'a, b ,c' }]),
    { op: 'CONTAINS_ANY', field: 'label_ids', value: ['a', 'b', 'c'] },
  );
  assert.equal(takesList('IN'), true);
  assert.equal(takesList('EQ'), false);
});

test('IS_NULL carries no value at all', () => {
  const node = filterOf(tomorrow, [{ field: 'due_at', op: 'IS_NULL', value: '' }]);
  assert.deepEqual(node, { op: 'IS_NULL', field: 'due_at' });
  assert.ok(node && !('value' in node), 'IS_NULL must not carry a value');
  assert.equal(takesValue('IS_NULL'), false);
});

test('an unsendable condition is dropped and the sendable ones still go', () => {
  // The half-built row does not take the finished one with it — the reader sees the filter they
  // completed applied, and the one they have not is simply not part of it yet.
  assert.deepEqual(
    filterOf(today, [
      { field: 'invented', op: 'EQ', value: 'x' },
      { field: 'title', op: 'CONTAINS', value: 'milk' },
    ]),
    { op: 'CONTAINS', field: 'title', value: 'milk' },
  );
});

// --- sorting and grouping ----------------------------------------------------------------------

test('sorting and grouping come from the same place as the filter', () => {
  assert.deepEqual(sortOf(today, 'title', 'DESC'), [{ field: 'title', dir: 'DESC' }]);
  assert.equal(sortOf(today, 'is_completed', 'ASC'), undefined, 'not sortable, so not sent');
  assert.deepEqual(groupOf(today, 'bucket_id'), { field: 'bucket_id', limit_per_group: 50 });
  assert.equal(groupOf(today, 'title'), undefined, 'not groupable, so not sent');
});
