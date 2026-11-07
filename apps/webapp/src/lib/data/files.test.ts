// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { fileHost, fileHref, fileLinksPath } from './files.ts';

// UC-INT-14 check 2: a link opens only an https address, and says the host it opens on.
test('a file link opens its https address alone', () => {
  assert.equal(fileHref('https://cloud.example.org/s/abc'), 'https://cloud.example.org/s/abc');
  assert.equal(fileHref('javascript:alert(1)'), undefined);
  assert.equal(fileHref('http://cloud.example.org/s/abc'), undefined);
  assert.equal(fileHost('https://contoso.sharepoint.com/sites/web/a.docx'), 'contoso.sharepoint.com');
  assert.equal(fileHost('not an address'), '');
  assert.equal(fileLinksPath('i-1'), '/items/i-1/file-links');
});
