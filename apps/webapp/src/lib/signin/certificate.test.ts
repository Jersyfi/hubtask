// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { certificateState } from './certificate.ts';

const now = Date.parse('2026-11-01T09:00:00Z');

test('a certificate ending within thirty days is ending', () => {
  assert.equal(certificateState('2026-11-30T09:00:00Z', now), 'ending');
});

test('a certificate ending later is not warned of', () => {
  assert.equal(certificateState('2026-12-02T09:00:00Z', now), undefined);
});

test('a certificate past its end has ended', () => {
  assert.equal(certificateState('2026-11-01T09:00:00Z', now), 'ended');
});

test('a provider without certificates says nothing', () => {
  assert.equal(certificateState(null, now), undefined);
  assert.equal(certificateState('not a date', now), undefined);
});
