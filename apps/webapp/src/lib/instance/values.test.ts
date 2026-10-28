// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import type { InstanceSetting, InstanceSettings } from '@hubtask/sync-engine';

import { changesBetween, countOf, documentOf, draftEntry, isOfferedAiReachable, lockCode, planBodyOf, rowsOf } from './values.ts';

const count = (extra: Partial<InstanceSetting> = {}): InstanceSetting => ({
  set: false, locked: false, kind: 'COUNT', minimum: 8, maximum: 128, workspace_may: 'TIGHTEN', ...extra,
});

test('a count is read as a number, and anything else as it was typed, for the server to refuse', () => {
  assert.equal(countOf('12'), 12);
  assert.equal(countOf(' 0 '), 0);
  assert.equal(countOf(''), undefined);
  assert.equal(countOf('3.5'), '3.5');
  assert.equal(countOf('twelve'), 'twelve');
});

test('the lock is worded by what an open key leaves a workspace free to do', () => {
  assert.equal(lockCode(count(), false), 'app.instance.lock_tighten_open');
  assert.equal(lockCode(count({ workspace_may: 'RAISE' }), true), 'app.instance.lock_raise_locked');
  assert.equal(lockCode(count({ workspace_may: 'REPLACE' }), false), 'app.instance.lock_replace_open');
  assert.equal(lockCode({ set: false, locked: false, kind: 'LOCALE' }, false), undefined);
});

test('what was decided is sent in its kind, and what was not is left out', () => {
  const draft: InstanceSettings = {
    sign_in: {
      min_length: count({ set: true, value: 14, locked: true }),
      min_digits: count(),
    },
    quotas: { items: { set: true, value: 500, locked: false, kind: 'COUNT', minimum: 0, workspace_may: 'RAISE' } },
    blocklist_file: '',
  };
  const document = documentOf(draft);
  assert.deepEqual(document.sign_in, { min_length: { set: true, value: 14, locked: true } });
  assert.deepEqual(document.quotas, { items: { set: true, value: 500, locked: false } });
  assert.deepEqual(document.legal, {});
});

test("the modules the installation decided travel with every save, so a save elsewhere keeps them (UC-INS-21 check 6)", () => {
  const planning: InstanceSetting = { set: true, value: true, locked: true, kind: 'FLAG', workspace_may: 'REPLACE' };
  const goals: InstanceSetting = { set: false, locked: false, kind: 'FLAG', workspace_may: 'REPLACE' };
  const document = documentOf({ module: { planning, goals }, blocklist_file: '' });
  assert.deepEqual(document.module, { planning: { set: true, value: true, locked: true } });
  assert.deepEqual(planBodyOf('Team', [], { module: { planning } }).module, { planning: { set: true, value: true, locked: true } });
});

test('the areas are drawn in the screen order, each key in its declared position', () => {
  const rows = rowsOf({
    ai: { sources: { set: false, locked: false, kind: 'CHOICE', options: ['EITHER'] } },
    sign_in: { min_digits: count({ position: 3 }), min_length: count({ position: 0 }) },
  });
  assert.deepEqual(rows.map((row) => `${row.area}.${row.key}`), ['sign_in.min_length', 'sign_in.min_digits', 'ai.sources']);
});

test("the terms' version is locked exactly when the terms link is, while the form is open", () => {
  const link = (locked: boolean): InstanceSetting => ({
    set: true, value: 'https://host.example/terms', locked, kind: 'LINK', workspace_may: 'REPLACE',
  });
  const version: InstanceSetting = { set: true, value: '3', locked: false, kind: 'TEXT', workspace_may: 'REPLACE' };
  const draft = (locked: boolean): InstanceSettings => ({
    legal: { terms_url: link(locked), terms_version: version },
    blocklist_file: '',
  });

  assert.equal(draftEntry(draft(true), 'legal', 'terms_version')?.locked, true);
  assert.equal(draftEntry(draft(false), 'legal', 'terms_version')?.locked, false);
  // Any other key keeps its own lock, and an undecided one is undecided.
  assert.equal(draftEntry(draft(true), 'legal', 'terms_url')?.locked, true);
  const undecided: InstanceSettings = { legal: { terms_version: { ...version, set: false } }, blocklist_file: '' };
  assert.equal(draftEntry(undecided, 'legal', 'terms_version'), undefined);
});

test('the metering switch travels through a save unchanged', () => {
  // A `PUT` replaces the level: a document without the switch would switch metering off.
  assert.equal(documentOf({ metering: true }).metering, true);
  assert.equal(documentOf({ metering: false }).metering, false);
});

test("the operator's reserved short names travel through a save unchanged", () => {
  // A `PUT` replaces the level: a document without the list would empty it.
  assert.deepEqual(documentOf({ reserved_short_names: ['intranet', 'shop'] }).reserved_short_names, ['intranet', 'shop']);
});

test("a plan's body carries its name, what it leaves out and only what it decides", () => {
  const decided: InstanceSetting = { set: true, value: 6, locked: true, kind: 'COUNT', workspace_may: 'RAISE' };
  const open: InstanceSetting = { set: false, locked: false, kind: 'COUNT', workspace_may: 'RAISE' };
  const body = planBodyOf('Family', ['WEBHOOKS'], { quotas: { accounts: decided, items: open } });
  assert.deepEqual(body, {
    display_name: 'Family', left_out: ['WEBHOOKS'],
    sign_in: {}, legal: {}, quotas: { accounts: { set: true, value: 6, locked: true } }, ai: {}, module: {},
  });
});

test('the save bar counts each value a draft changes: a value, a lock, a decision, the blocklist', () => {
  const read: InstanceSettings = {
    sign_in: { min_length: count({ set: true, value: 12 }), max_repeat: count() },
    blocklist_file: '/etc/hubtask/blocklist.txt',
  };
  assert.equal(changesBetween(read, read), 0);
  assert.equal(changesBetween(JSON.parse(JSON.stringify(read)), read), 0, 'a copy is no change');
  const draft: InstanceSettings = {
    sign_in: {
      min_length: count({ set: true, value: 14, locked: true }),
      max_repeat: count({ set: true, value: 3 }),
    },
    blocklist_file: '',
  };
  assert.equal(changesBetween(draft, read), 3);
  // A key the operator stops deciding is a change too.
  assert.equal(changesBetween({ ...read, sign_in: { max_repeat: count() } }, read), 1);
});

test('an offered model is reachable only where the installation lets workspaces use the offered model', () => {
  const sources = (extra: Partial<InstanceSetting>): InstanceSettings => ({
    ai: { sources: { set: false, locked: false, kind: 'CHOICE', options: ['EITHER', 'OFFERED', 'OWN', 'NONE'], ...extra } },
  });
  assert.equal(isOfferedAiReachable(undefined), undefined);
  // Undecided: the product's default, their own model only.
  assert.equal(isOfferedAiReachable(sources({})), false);
  assert.equal(isOfferedAiReachable(sources({ set: true, value: 'OWN' })), false);
  assert.equal(isOfferedAiReachable(sources({ set: true, value: 'NONE', locked: true })), false);
  assert.equal(isOfferedAiReachable(sources({ set: true, value: 'OFFERED' })), true);
  assert.equal(isOfferedAiReachable(sources({ set: true, value: 'EITHER' })), true);
});
