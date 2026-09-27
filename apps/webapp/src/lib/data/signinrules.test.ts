// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The prediction, against the cases the domain has to agree with.
//
// The shared fixture under `api/` is what keeps the two implementations honest about a corpus; this
// file is about the arithmetic itself - the parts a fixture cannot express, like the order of the
// lines and what an empty field looks like.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  RULES_BEFORE_READING,
  carriesContextWord,
  classesOf,
  evaluate,
  flatten,
  localRulesHold,
  longestRun,
  unmetCount,
  type PasswordRules,
} from './signinrules.ts';

const rules = (overrides: Partial<PasswordRules> = {}): PasswordRules => ({
  ...RULES_BEFORE_READING,
  ...overrides,
});

const nobody = {};

test('length is counted in code points, not in bytes', () => {
  // "Straße" is six characters and eight bytes; a rule counting bytes would accept a shorter
  // password from a German keyboard than from an English one.
  const lines = evaluate(rules({ min_length: 6, common_passwords: false, context_words: false }), 'Straße', nobody);
  assert.equal(lines[0]?.state, 'met');
});

test('an emoji is one character and one class', () => {
  const counts = classesOf('ab🌍');
  assert.equal(counts.symbols, 1);
  assert.equal(counts.classes, 2);
  assert.equal(longestRun('aa🌍🌍🌍'), 3);
});

test('the four classes are Unicode categories, not ASCII ranges', () => {
  const counts = classesOf('Ωμέγα7!');
  assert.equal(counts.upper, 1, 'Ω is an uppercase letter');
  assert.ok(counts.lower >= 4);
  assert.equal(counts.digits, 1);
  assert.equal(counts.symbols, 1);
});

test('a list lookup sees through the obvious substitutions', () => {
  assert.equal(flatten('P@ssw0rd!'), 'passwordi');
  assert.ok(carriesContextWord('my-acme-2026', { workspaceName: 'Acme Industries' }));
  assert.ok(carriesContextWord('jerome!!2026', { email: 'jerome@example.eu' }));
  assert.ok(!carriesContextWord('unrelated words here', { email: 'jerome@example.eu' }));
});

test('a short local part is not a context word', () => {
  // `jw@example.eu` would otherwise forbid every password containing "jw" - which is most of them.
  assert.ok(!carriesContextWord('a jwel of a password', { email: 'jw@example.eu' }));
});

test('an empty field is unmet, never failed', () => {
  const lines = evaluate(rules(), '', nobody);
  assert.ok(lines.every((line) => line.state === 'unmet' || line.state === 'server'));
  assert.ok(!lines.some((line) => line.state === 'failed'));
});

test('the server’s lines wait until the local ones hold', () => {
  const policy = rules({ min_length: 12 });
  const early = evaluate(policy, 'short', nobody).filter((line) => line.isServerSide);
  assert.ok(early.every((line) => line.state === 'server'), 'nothing is being checked yet');

  const ready = evaluate(policy, 'a-long-enough-one', nobody).filter((line) => line.isServerSide);
  assert.ok(ready.every((line) => line.state === 'checking'));
});

test('a violation the server names turns exactly that line', () => {
  const lines = evaluate(rules(), 'a-long-enough-one', nobody, { violations: ['common'], isAnswered: true });
  const common = lines.find((line) => line.id === 'common');
  const context = lines.find((line) => line.id === 'context_words');
  assert.equal(common?.state, 'failed');
  assert.equal(context?.state, 'met');
});

test('an unreachable server leaves its lines unchecked rather than met', () => {
  const lines = evaluate(rules(), 'a-long-enough-one', nobody, { isUnreachable: true });
  assert.ok(lines.filter((line) => line.isServerSide).every((line) => line.state === 'unchecked'));
});

test('only the rules that are on become lines', () => {
  const lines = evaluate(rules({ min_digits: 1, min_classes: 3, max_repeat: 3 }), 'Abc123!!', nobody);
  const ids = lines.map((line) => line.id);
  assert.deepEqual(ids, ['min_length', 'min_digits', 'min_classes', 'max_repeat', 'context_words', 'common']);
  assert.ok(!ids.includes('min_uppercase'), 'a switch at zero draws no line');
});

test('the longest list is twelve, and the server’s four are last', () => {
  const everything = rules({
    min_length: 15, min_lowercase: 1, min_uppercase: 1, min_digits: 1, min_symbols: 1,
    min_classes: 3, max_repeat: 3, common_passwords: true, context_words: true,
    breach_check: true, history_count: 10, not_current: true,
  });
  const lines = evaluate(everything, '', nobody);
  assert.equal(lines.length, 12);
  assert.deepEqual(lines.slice(8).map((line) => line.id), ['common', 'breach', 'history', 'not_current']);
});

test('localRulesHold is what decides whether the server is asked', () => {
  const policy = rules({ min_length: 12, min_digits: 1 });
  assert.equal(localRulesHold(policy, '', nobody), false);
  assert.equal(localRulesHold(policy, 'no-digits-here', nobody), false);
  assert.equal(localRulesHold(policy, 'has-1-digit-here', nobody), true);
});

test('the count the live region announces is the number of lines still open', () => {
  const lines = evaluate(rules(), 'a-long-enough-one', nobody, { violations: ['common'], isAnswered: true });
  assert.equal(unmetCount(lines), 1);
});
