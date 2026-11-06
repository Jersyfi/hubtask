// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { addressAfterChange } from './change.ts';

// UC-INS-19 check 7: after the change the workspace answers at the new address only, so the owner's
// browser goes there - on the scheme and port it came by.
test('the browser goes on to the new address, on the scheme and port it came by', () => {
  assert.equal(addressAfterChange('familie-meier.tasks.example.net', { protocol: 'https:', port: '' }),
    'https://familie-meier.tasks.example.net/');
  assert.equal(addressAfterChange('familie-meier.localhost', { protocol: 'http:', port: '8080' }),
    'http://familie-meier.localhost:8080/');
});
