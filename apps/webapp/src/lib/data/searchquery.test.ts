// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import {
  QUICK,
  asksSomething,
  clear,
  compile,
  content,
  has,
  isQuick,
  joinLine,
  narrowingOf,
  parse,
  values,
  wordsOf,
  structural,
  toggle,
  write,
  type Context,
} from './searchquery.ts';

/** Everything the catalogue serves, which is what a complete installation reports. */
const ALL = new Set([
  'type', 'parent_id', 'collection_id', 'bucket_id', 'is_completed', 'title', 'notes', 'depth',
  'order_key', 'created_by', 'created_at', 'updated_at', 'completed_at', 'start_at', 'due_at',
  'archived_at', 'labels', 'assignee_id', 'members', 'text',
]);

const KITCHEN = '01a00000-0000-7000-8000-000000000001';
const SHOPPING = '01a00000-0000-7000-8000-000000000002';

function context(overrides: Partial<Context> = {}): Context {
  return {
    fields: ALL,
    collection: (name) => ({ kitchen: KITCHEN, garden: SHOPPING })[name.toLowerCase()],
    label: (name) => ({ urgent: 'lab-1', shopping: 'lab-2' })[name.toLowerCase()],
    hasCollection: true,
    ...overrides,
  };
}

test('a line is words plus tokens, and an unknown key stays in the words', () => {
  const parsed = parse('milk is:open due:week https://example.invalid/x');

  assert.equal(parsed.words, 'milk https://example.invalid/x');
  assert.deepEqual(
    parsed.tokens.map((token) => ({ key: token.key, value: token.value, negated: token.negated })),
    [
      { key: 'is', value: 'open', negated: false },
      { key: 'due', value: 'week', negated: false },
    ],
  );
});

test('a quoted phrase stays in the words, and a quoted value loses its quotes', () => {
  const parsed = parse('"spring cleaning" in:"kitchen" -label:"urgent"');

  assert.equal(parsed.words, '"spring cleaning"');
  assert.deepEqual(parsed.tokens.map(write), ['in:kitchen', '-label:urgent']);
});

test('several questions narrow, several answers of one question widen', () => {
  const compiled = compile(parse('type:task is:open'), context());

  assert.deepEqual(compiled.filter, {
    op: 'AND',
    nodes: [
      { op: 'IN', field: 'type', value: ['TASK'] },
      { op: 'EQ', field: 'is_completed', value: false },
    ],
  });

  const two = compile(parse('due:overdue due:none'), context());
  assert.deepEqual(two.filter, {
    op: 'OR',
    nodes: [
      { op: 'LT', field: 'due_at', value: '@today' },
      { op: 'IS_NULL', field: 'due_at' },
    ],
  });
});

test('`me` is the server placeholder, never an identifier this client resolved', () => {
  const compiled = compile(parse('who:me with:me by:me'), context());

  assert.deepEqual(compiled.filter, {
    op: 'AND',
    nodes: [
      { op: 'EQ', field: 'assignee_id', value: '@me' },
      { op: 'CONTAINS', field: 'members', value: '@me' },
      { op: 'EQ', field: 'created_by', value: '@me' },
    ],
  });

  // The one narrowing the chips cannot express at all today: nobody on it, anywhere.
  assert.deepEqual(compile(parse('who:none'), context()).filter, {
    op: 'IS_NULL',
    field: 'assignee_id',
  });
});

test('no date is computed here: an anchor travels unresolved', () => {
  assert.deepEqual(compile(parse('due:week'), context()).filter, {
    op: 'LTE', field: 'due_at', value: '@end_of_week',
  });
  assert.deepEqual(compile(parse('due:>@today+P7D'), context()).filter, {
    op: 'GT', field: 'due_at', value: '@today+P7D',
  });
  assert.deepEqual(compile(parse('updated:2026-10-01..2026-10-31'), context()).filter, {
    op: 'BETWEEN',
    field: 'updated_at',
    value: ['2026-10-01T00:00:00Z', '2026-10-31T00:00:00Z'],
  });
  assert.deepEqual(compile(parse('created:<2026-01-01'), context()).filter, {
    op: 'LT', field: 'created_at', value: '2026-01-01T00:00:00Z',
  });
});

test('a leading minus wraps the question in NOT, whatever the question is', () => {
  assert.deepEqual(compile(parse('-type:activity'), context()).filter, {
    op: 'NOT',
    nodes: [{ op: 'IN', field: 'type', value: ['ACTIVITY'] }],
  });
  assert.deepEqual(compile(parse('-due:none'), context()).filter, {
    op: 'NOT',
    nodes: [{ op: 'IS_NULL', field: 'due_at' }],
  });
});

test('the archive and the trash are flags, and the archive is also a question', () => {
  const compiled = compile(parse('is:archived'), context());

  assert.equal(compiled.includeArchived, true);
  assert.equal(compiled.includeTrashed, false);
  assert.deepEqual(compiled.filter, { op: 'NOT', nodes: [{ op: 'IS_NULL', field: 'archived_at' }] });

  // Deleted is not something the grammar may ask about, so this widens and narrows nothing.
  const trashed = compile(parse('is:trashed milk'), context());
  assert.equal(trashed.includeTrashed, true);
  assert.equal(trashed.filter, undefined);
  assert.equal(trashed.q, 'milk');
});

test('a sort is only meaningful without words, and the line says so before the server does', () => {
  const alone = compile(parse('who:me is:open sort:-due'), context());
  assert.deepEqual(alone.sort, [{ field: 'due_at', direction: 'DESC' }]);
  assert.deepEqual(alone.problems, []);

  const beside = compile(parse('milk sort:due'), context());
  assert.equal(beside.sort, undefined);
  assert.deepEqual(beside.problems, [{ code: 'app.searchline.sort_with_words' }]);
});

test('a field the installation does not report is refused by name, never sent', () => {
  const thin = compile(parse('who:me is:open'), context({ fields: new Set(['is_completed']) }));

  assert.deepEqual(thin.filter, { op: 'EQ', field: 'is_completed', value: false });
  assert.deepEqual(thin.problems, [
    { code: 'app.searchline.field_absent', params: { key: 'who' } },
  ]);
});

test('a name that stands for nothing is a problem, not a filter that matches nothing', () => {
  const place = compile(parse('in:cellar'), context());
  assert.equal(place.filter, undefined);
  assert.deepEqual(place.problems, [
    { code: 'app.searchline.place_unknown', params: { value: 'cellar' } },
  ]);

  // A label belongs to a collection (I-W3), so until one is named there is nothing to resolve
  // against - and the line says which, where the chip simply disappears.
  const loose = compile(parse('label:urgent'), context({ hasCollection: false }));
  assert.deepEqual(loose.problems, [{ code: 'app.searchline.label_needs_place' }]);
});

test('names resolve to identifiers, and a comma is several answers to one question', () => {
  assert.deepEqual(compile(parse('in:kitchen,garden'), context()).filter, {
    op: 'IN', field: 'collection_id', value: [KITCHEN, SHOPPING],
  });
  assert.deepEqual(compile(parse('label:urgent,shopping'), context()).filter, {
    op: 'CONTAINS_ANY', field: 'labels', value: ['lab-1', 'lab-2'],
  });
});

test('the address may carry the structure and never the content', () => {
  const parsed = parse('milk in:kitchen title:"birthday card" is:open');

  assert.equal(structural(parsed), 'in:kitchen is:open');
  assert.equal(content(parsed), 'milk title:"birthday card"');
});

test('a chip press adds a token, and the same press takes it off again', () => {
  let line = 'milk';
  line = toggle(line, 'is', 'open');
  assert.equal(line, 'is:open milk');

  line = toggle(line, 'type', 'task');
  assert.equal(line, 'is:open type:task milk');

  line = toggle(line, 'is', 'open');
  assert.equal(line, 'type:task milk');
  assert.equal(has(parse(line), 'type', 'task'), true);

  // A question with one answer replaces it rather than gathering a second.
  const ordered = toggle(toggle('who:me', 'sort', 'due'), 'sort', 'updated');
  assert.equal(ordered, 'who:me sort:updated');
});

test('neither words nor a narrowing is not a search', () => {
  assert.equal(asksSomething(compile(parse(''), context())), false);
  assert.equal(asksSomething(compile(parse('   '), context())), false);
  assert.equal(asksSomething(compile(parse('who:me'), context())), true);
  assert.equal(asksSomething(compile(parse('milk'), context())), true);
});

test('a value that is missing or unknown is said, not guessed', () => {
  assert.deepEqual(compile(parse('is:'), context()).problems, [
    { code: 'app.searchline.value_missing', params: { key: 'is' } },
  ]);
  assert.deepEqual(compile(parse('type:epic'), context()).problems, [
    {
      code: 'app.searchline.value_unknown',
      params: { key: 'type', value: 'epic', expected: 'task, work_package, activity' },
    },
  ]);
});

test('a half-typed date is refused rather than compared against as text', () => {
  // On the way to `due:week`, and the reason this matters: `LTE due_at "wee"` is a filter the
  // server accepts, answers with nothing, and gives the reader no reason for.
  const half = compile(parse('due:wee'), context());
  assert.equal(half.filter, undefined);
  assert.equal(half.problems[0]?.code, 'app.searchline.value_unknown');

  assert.notEqual(compile(parse('due:2026-10-01'), context()).filter, undefined);
  assert.notEqual(compile(parse('due:>@today+P7D'), context()).filter, undefined);
  assert.equal(compile(parse('updated:soon..later'), context()).filter, undefined);
});

// ---------------------------------------------------------------------------------------------
// The two surfaces over one string
// ---------------------------------------------------------------------------------------------

test('the editing split hands each control exactly its own half', () => {
  const line = 'milk who:me is:open in:kitchen bread';

  // The field for the words holds the words; the chips hold the tokens. Put back together they
  // are the string that was there - which is what makes the two surfaces impossible to desync.
  assert.equal(wordsOf(line), 'milk bread');
  assert.equal(narrowingOf(line), 'who:me is:open in:kitchen');
  assert.equal(joinLine(narrowingOf(line), wordsOf(line)), 'who:me is:open in:kitchen milk bread');
});

test('a chip says how many answers it holds, and empties only its own question', () => {
  const line = 'type:task type:activity is:open milk';

  assert.equal(values(parse(line), 'type').length, 2);
  assert.equal(values(parse(line), 'is').length, 1);
  assert.equal(clear(line, 'type'), 'is:open milk');
  // Emptying a question nobody answered changes nothing rather than rewriting the line.
  assert.equal(clear('is:open milk', 'label'), 'is:open milk');
});

test('a quick narrowing is recognised as itself, and stops being one the moment it is edited', () => {
  const mine = QUICK[0];
  assert.ok(mine);
  assert.equal(isQuick(mine.line, mine), true);
  // The order of the tokens is not part of it: the same question written the other way round is
  // the same question, and a chip that lit up only for one spelling would be a lie half the time.
  assert.equal(isQuick('is:open who:me', mine), true);
  assert.equal(isQuick(`${mine.line} due:week`, mine), false);
  assert.equal(isQuick(`${mine.line} milk`, mine), false);
});

test('every quick narrowing compiles, and names a field the catalogue has', () => {
  for (const quick of QUICK) {
    const compiled = compile(parse(quick.line), context());
    assert.deepEqual(compiled.problems, [], quick.code);
    assert.ok(asksSomething(compiled), quick.code);
    assert.ok(ALL.has(quick.field), quick.field);
  }
});

test('a person is looked up by name, and one nobody knows is refused rather than sent', () => {
  const known = compile(parse('who:anna'), context({ person: (name) => (name === 'anna' ? 'acc-1' : undefined) }));
  assert.deepEqual(known.filter, { op: 'EQ', field: 'assignee_id', value: 'acc-1' });

  // Sent as itself is the quiet failure: `EQ assignee_id 'bruno'` is answered with an empty page
  // rather than a complaint, and the reader reads "nothing matches" about a question nobody asked.
  const unknown = compile(parse('who:bruno'), context({ person: () => undefined }));
  assert.equal(unknown.filter, undefined);
  assert.deepEqual(unknown.problems.map((problem) => problem.code), ['app.searchline.person_unknown']);
});
