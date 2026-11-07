// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { FileStoreConnection, FileStoreItem } from '@hubtask/sync-engine';

import { backTo, connectionLabel, enterFolder, fileStoreFolderPath, linkOf, sortItems } from './filestore.ts';

const item = (ref: string, name: string, folder: boolean, address?: string): FileStoreItem => ({
  ref, name, folder, ...(address ? { address, provider: 'NEXTCLOUD' as const } : {}),
});

test('UC-INT-17: a picker walks into folders and back, the top kept', () => {
  const top = [{ ref: '', name: 'Nextcloud' }];
  const specs = enterFolder(top, item('/Specs', 'Specs', true));
  const year = enterFolder(specs, item('/Specs/2026', '2026', true));
  assert.deepEqual(year.map((s) => s.ref), ['', '/Specs', '/Specs/2026']);
  assert.deepEqual(enterFolder(year, item('/Specs/a.pdf', 'a.pdf', false)), year);
  assert.deepEqual(backTo(year, 1).map((s) => s.ref), ['', '/Specs']);
  assert.deepEqual(backTo(year, -1).map((s) => s.ref), ['']);
  assert.equal(fileStoreFolderPath('c1', '/Specs/Login flow'), '/integrations/file-stores/c1/items?folder=%2FSpecs%2FLogin%20flow');
  assert.equal(fileStoreFolderPath('c1', ''), '/integrations/file-stores/c1/items');
});

test('UC-INT-17: folders come first, and a picked file is an https link with its store and name', () => {
  const sorted = sortItems([item('/b.pdf', 'b.pdf', false), item('/Z', 'Z', true), item('/a.pdf', 'a.pdf', false)], 'en');
  assert.deepEqual(sorted.map((i) => i.name), ['Z', 'a.pdf', 'b.pdf']);
  assert.deepEqual(linkOf(item('/a.pdf', 'a.pdf', false, 'https://cloud.example.org/index.php/f/1')), {
    address: 'https://cloud.example.org/index.php/f/1', provider: 'NEXTCLOUD', name: 'a.pdf',
  });
  assert.equal(linkOf(item('/a.pdf', 'a.pdf', false, 'http://cloud.example.org/f/1')), undefined);
  assert.equal(linkOf(item('/a.pdf', 'a.pdf', false)), undefined);
});

test('UC-INT-17: a connection is named by its store, its login and a Nextcloud\'s host', () => {
  const nextcloud: FileStoreConnection = { id: 'c', kind: 'NEXTCLOUD', server: 'https://cloud.example.org', login: 'ada', created_at: '', used_at: '' };
  assert.equal(connectionLabel(nextcloud, 'Nextcloud'), 'Nextcloud · ada@cloud.example.org');
  assert.equal(connectionLabel({ ...nextcloud, kind: 'MICROSOFT', server: undefined, login: 'ada@example.org' }, 'Microsoft'), 'Microsoft · ada@example.org');
});
