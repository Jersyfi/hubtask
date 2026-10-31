// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { formTokenOf, publicFormAddress, submissionOf } from './publicforms.ts';

// UC-SVC-13 check 1: the link carries its token in the fragment, and the screen reads back only a
// form token.
test('a public form link keeps its token in the fragment', () => {
  const address = publicFormAddress('https://hub.example.org/', 'hbt_form_abc_DEF-1');
  assert.equal(address, 'https://hub.example.org/form#hbt_form_abc_DEF-1');
  assert.equal(formTokenOf(new URL(address).hash), 'hbt_form_abc_DEF-1');
  assert.equal(formTokenOf('#hbt_page_abc'), '');
  assert.equal(formTokenOf('#hbt_form_a<b'), '');
  assert.equal(formTokenOf(''), '');
});

test('a submission sends what was typed and leaves the empty out', () => {
  assert.deepEqual(
    submissionOf({ entryType: 'feedback', title: ' Lob ', notes: '  ', name: '', email: ' kaya@example.org ', answers: {} }),
    { entry_type: 'feedback', title: 'Lob', requester_email: 'kaya@example.org' },
  );
});
