// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { layoutKey, openingLayout, rememberLayout, type LayoutStore } from './layoutmemory.ts';

const DRAWABLE = ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'];

function memory(): LayoutStore & { readonly kept: Map<string, string> } {
  const kept = new Map<string, string>();
  return { kept, getItem: (key) => kept.get(key) ?? null, setItem: (key, value) => void kept.set(key, value) };
}

const refusing: LayoutStore = {
  getItem: () => {
    throw new Error('storage refused');
  },
  setItem: () => {
    throw new Error('storage refused');
  },
};

const open = (store: LayoutStore | undefined, accountId: string | undefined, collectionId: string, extra = {}) =>
  openingLayout({ store, accountId, collectionId, offered: [], preferred: undefined, drawable: DRAWABLE, ...extra });

test('the key names the person and the collection', () => {
  assert.equal(layoutKey('a1', 'c1'), 'hubtask.layout.a1.c1');
});

test('a choice is kept per collection and per person', () => {
  const store = memory();
  rememberLayout(store, 'anna', 'kitchen', 'KANBAN');
  rememberLayout(store, 'anna', 'garden', 'TIMELINE');
  assert.equal(open(store, 'anna', 'kitchen'), 'KANBAN');
  assert.equal(open(store, 'anna', 'garden'), 'TIMELINE');
  assert.equal(open(store, 'mara', 'kitchen'), 'LIST_COLLAPSED', 'another person on the same browser starts afresh');
});

test('a remembered layout the collection no longer offers gives way to its default', () => {
  const store = memory();
  rememberLayout(store, 'anna', 'kitchen', 'TIMELINE');
  assert.equal(open(store, 'anna', 'kitchen', { offered: ['LIST_COLLAPSED', 'KANBAN'], preferred: 'KANBAN' }), 'KANBAN');
  assert.equal(open(store, 'anna', 'kitchen', { offered: ['LIST_COLLAPSED', 'KANBAN'] }), 'LIST_COLLAPSED');
});

test('nothing the client cannot draw is opened, whatever was kept', () => {
  const store = memory();
  store.setItem(layoutKey('anna', 'kitchen'), 'GANTT');
  assert.equal(open(store, 'anna', 'kitchen'), 'LIST_COLLAPSED');
});

test('a refused storage falls back in silence', () => {
  assert.doesNotThrow(() => rememberLayout(refusing, 'anna', 'kitchen', 'KANBAN'));
  assert.equal(open(refusing, 'anna', 'kitchen', { preferred: 'KANBAN' }), 'KANBAN');
  assert.equal(open(undefined, 'anna', 'kitchen'), 'LIST_COLLAPSED');
});

test('without a known person nothing is kept or read', () => {
  const store = memory();
  rememberLayout(store, undefined, 'kitchen', 'KANBAN');
  assert.equal(store.kept.size, 0);
});
