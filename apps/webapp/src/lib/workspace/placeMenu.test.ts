// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { placeMenu } from './placeMenu.ts';

const t = (code: string) => code;
const ids = (items: { id: string }[]) => items.map((item) => item.id);

test('a collection\'s owner is offered its settings, its place, its archive and the trash - no edit, no entry act', () => {
  assert.deepEqual(ids(placeMenu({ type: 'COLLECTION', mayShape: true, mayDelete: true, archival: 'active' }, t)), ['settings', 'move', 'import', 'position', 'archive', 'trash']);
});

test('a hub has nowhere to move to, and offers its privacy where the reader may', () => {
  assert.deepEqual(ids(placeMenu({ type: 'HUB', mayShape: true, mayDelete: false, archival: 'active', privacy: 'make_private' }, t)), ['settings', 'import', 'position', 'archive', 'privacy']);
});

test('a reader who may change nothing of the place is offered nothing', () => {
  assert.deepEqual(placeMenu({ type: 'COLLECTION', mayShape: false, mayDelete: false, archival: 'active' }, t), []);
});

test('archived above, there is nothing to archive or restore here; archived here, it is restored', () => {
  assert.ok(!ids(placeMenu({ type: 'COLLECTION', mayShape: true, mayDelete: false, archival: 'inherited' }, t)).includes('archive'));
  const restore = placeMenu({ type: 'COLLECTION', mayShape: true, mayDelete: false, archival: 'archived' }, t).find((item) => item.id === 'archive');
  assert.equal(restore?.label, 'app.workspace.unarchive');
});

test('a collection offers to copy its mail address where places have one (ADR-0092 M1)', () => {
  assert.deepEqual(ids(placeMenu({ type: 'COLLECTION', mayShape: false, mayDelete: false, archival: 'active', hasMail: true }, t)), ['mail']);
  assert.equal(ids(placeMenu({ type: 'COLLECTION', mayShape: true, mayDelete: false, archival: 'active', hasMail: true }, t))[0], 'mail');
  assert.ok(!ids(placeMenu({ type: 'HUB', mayShape: true, mayDelete: false, archival: 'active', hasMail: true }, t)).includes('mail'));
  assert.ok(!ids(placeMenu({ type: 'COLLECTION', mayShape: true, mayDelete: false, archival: 'archived', hasMail: true }, t)).includes('mail'));
});
