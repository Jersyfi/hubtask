// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Target } from '../data/backup.svelte.ts';
import { exportTargetChoices } from './targets.ts';

const target = (over: Partial<Target>): Target => ({
  id: '01a0e2e0-0000-7000-8000-000000001901',
  name: 'Exports',
  kind: 'S3',
  scope: 'INSTANCE',
  config: { bucket: 'installation-exports', endpoint: 'https://s3.example.org' },
  encryption_mode: 'AES256_GCM',
  enabled: true,
  ...over,
});

test('the export offers a target by its name and identifier, and nothing of where it points', () => {
  const choices = exportTargetChoices([target({})]);
  assert.deepEqual(choices, [{ value: '01a0e2e0-0000-7000-8000-000000001901', label: 'Exports' }]);
  const shown = JSON.stringify(choices);
  assert.ok(!shown.includes('installation-exports') && !shown.includes('s3.example.org'));
});

test('a disabled target is not offered, since the export would refuse it', () => {
  assert.deepEqual(exportTargetChoices([target({ enabled: false })]), []);
});
