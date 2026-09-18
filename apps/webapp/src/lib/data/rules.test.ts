// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { inboundAddressOf } from './rules.ts';

test('the inbound address is the route under the API at the origin, with the token as the credential', () => {
  assert.equal(
    inboundAddressOf('https://hubtask.example', 'hbt_hook_abc'),
    'https://hubtask.example/api/v1/automation/inbound/hbt_hook_abc',
  );
  // A trailing slash on the origin does not double up.
  assert.equal(inboundAddressOf('https://hubtask.example/', 'hbt_hook_abc'), 'https://hubtask.example/api/v1/automation/inbound/hbt_hook_abc');
});
