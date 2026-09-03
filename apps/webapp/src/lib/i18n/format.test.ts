// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The renderer, checked in the languages that break the English assumptions.
//
// English has two plural forms and no case, which makes it the worst possible language to test a
// plural implementation in: `one`/`other` passes with a hard-coded `n === 1`. Polish and Arabic are
// here because they are where that shortcut shows - `few` for 2-4 and `many` for 5 and up, and a
// `zero` category that English does not have at all.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MessageSyntaxError, format, parse } from './format.ts';

test('a code with parameters renders in English', () => {
  assert.equal(format('Hello {name}', { name: 'Ada' }), 'Hello Ada');
  assert.equal(
    format('{actor} completed {item_title}', { actor: 'Anna', item_title: 'Review the quote' }),
    'Anna completed Review the quote',
  );
});

test('a missing parameter leaves its placeholder standing', () => {
  // The same choice infrastructure/i18n made on the Go side: `{request_id}` on the screen says a
  // value went missing; an empty gap says nothing at all.
  assert.equal(
    format('Something went wrong on our side. Reference: {request_id}'),
    'Something went wrong on our side. Reference: {request_id}',
  );
});

test('the plural categories are the locale’s, not English’s', () => {
  const polish = '{n, plural, one{# zadanie} few{# zadania} many{# zadań} other{# zadania}}';
  assert.equal(format(polish, { n: 1 }, 'pl'), '1 zadanie');
  assert.equal(format(polish, { n: 3 }, 'pl'), '3 zadania');
  assert.equal(format(polish, { n: 5 }, 'pl'), '5 zadań');
  assert.equal(format(polish, { n: 22 }, 'pl'), '22 zadania');

  // Arabic has all six, and `zero` is a category rather than the number nought - which is the
  // distinction a hand-rolled `n === 0` check gets wrong.
  const arabic = '{n, plural, zero{صفر} one{واحد} two{اثنان} few{قليل} many{كثير} other{آخر}}';
  assert.equal(format(arabic, { n: 0 }, 'ar'), 'صفر');
  assert.equal(format(arabic, { n: 2 }, 'ar'), 'اثنان');
  assert.equal(format(arabic, { n: 11 }, 'ar'), 'كثير');
});

test('an exact match wins over its category, and an offset moves the number', () => {
  const pattern = '{n, plural, =0{Nobody has read it} one{# person has read it} other{# people have read it}}';
  assert.equal(format(pattern, { n: 0 }), 'Nobody has read it');
  assert.equal(format(pattern, { n: 1 }), '1 person has read it');
  assert.equal(format(pattern, { n: 7 }), '7 people have read it');

  const withOffset = '{n, plural, offset:1 one{you and # other} other{you and # others}}';
  assert.equal(format(withOffset, { n: 2 }), 'you and 1 other');
  assert.equal(format(withOffset, { n: 4 }), 'you and 3 others');
});

test('selectordinal uses the ordinal rules, which are a different set', () => {
  const pattern = '{n, selectordinal, one{#st} two{#nd} few{#rd} other{#th}} attempt';
  assert.equal(format(pattern, { n: 1 }), '1st attempt');
  assert.equal(format(pattern, { n: 2 }), '2nd attempt');
  assert.equal(format(pattern, { n: 11 }), '11th attempt');
  assert.equal(format(pattern, { n: 22 }), '22nd attempt');
});

test('select chooses by value and falls back to other', () => {
  const pattern = '{scope, select, hub{the hub} collection{the collection} other{this}}';
  assert.equal(format(pattern, { scope: 'hub' }), 'the hub');
  assert.equal(format(pattern, { scope: 'work_package' }), 'this');
  assert.equal(format(pattern, {}), 'this');
});

test('branches hold whole messages, not only text', () => {
  const nested =
    '{n, plural, one{{actor} added # label} other{{actor} added # labels}}';
  assert.equal(format(nested, { n: 1, actor: 'Anna' }), 'Anna added 1 label');
  assert.equal(format(nested, { n: 3, actor: 'Anna' }), 'Anna added 3 labels');
});

test('a number is written the way the locale writes numbers', () => {
  assert.equal(format('{count} tasks', { count: 1234 }), '1,234 tasks');
  assert.equal(format('{count} Aufgaben', { count: 1234 }, 'de'), '1.234 Aufgaben');
});

test('apostrophes follow ICU, which is the rule English trips over', () => {
  // A lone apostrophe is an apostrophe. Treating it as a quote would swallow the rest of the
  // sentence - in English, where nobody would look for a formatting bug.
  assert.equal(format("the item's owner"), "the item's owner");
  assert.equal(format("'{'not an argument'}'"), '{not an argument}');
  assert.equal(format("it''s done"), "it's done");
});

test('syntax this renderer does not implement is refused, by name', () => {
  // F1-07's condition for writing this rather than installing it. The message names the
  // construct, because "unsupported syntax" sends a reader looking and "{n, number}" sends them
  // to the one line to change.
  assert.throws(() => parse('{n, number}'), MessageSyntaxError);
  assert.throws(() => parse('{d, date, short}'), (error) => /`\{d, date\}` is not implemented/.test(error.message));
  assert.throws(() => parse('{n, plural, one{#}}'), /no `other` branch/);
  assert.throws(() => parse('{n, plural, singular{#} other{#}}'), /not a CLDR plural category/);
  assert.throws(() => parse('{name'), /not closed/);
  assert.throws(() => parse('a } stray'), /with no `\{` before it/);
});
