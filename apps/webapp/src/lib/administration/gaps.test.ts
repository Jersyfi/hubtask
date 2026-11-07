// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { gapsOf, keepsSecrets } from './gaps.ts';

test('an installation without a mail server or a key says so on the overview', () => {
  assert.deepEqual(gapsOf({ mail: false, encryption: false }).map((gap) => gap.feature), ['notifications', 'webhooks']);
  assert.deepEqual(gapsOf({ mail: true, encryption: true }), [], 'everything set up is no gap');
  assert.deepEqual(gapsOf(undefined), [], 'a manifest not read yet is no gap');
  assert.deepEqual(gapsOf({}), [], 'a server that does not say is no gap');
});

test('the webhook form is offered only where the secret can be kept', () => {
  assert.equal(keepsSecrets({ encryption: false }), false);
  assert.equal(keepsSecrets({ encryption: true }), true);
  assert.equal(keepsSecrets(undefined), true, 'unknown leaves the server to decide');
});
