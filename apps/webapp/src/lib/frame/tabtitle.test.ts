// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-INS-22 check 9: the browser tab names the workspace - with the page's title where the page
// told the frame one, alone where it did not, and the product where no workspace is known.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { tabTitleOf } from './tabtitle.ts';

test('a page with a title names itself and the workspace', () => {
  assert.equal(tabTitleOf('Today', 'Nordlicht Werkstatt'), 'Today – Nordlicht Werkstatt');
});

test('a page without a title names the workspace alone', () => {
  assert.equal(tabTitleOf(undefined, 'Nordlicht Werkstatt'), 'Nordlicht Werkstatt');
  assert.equal(tabTitleOf('  ', 'Nordlicht Werkstatt'), 'Nordlicht Werkstatt');
});

test('where no workspace is known, the product stands in its place', () => {
  assert.equal(tabTitleOf(undefined, undefined), 'Hubtask');
  assert.equal(tabTitleOf(undefined, ' '), 'Hubtask');
  assert.equal(tabTitleOf('Roadmap of Growth', undefined), 'Roadmap of Growth – Hubtask');
});
