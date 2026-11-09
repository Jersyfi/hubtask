// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { backupColumnFigure, privacyColumnFigure, runsColumnFigure } from './areafigures.ts';

const t = (code: string, params?: Record<string, string>) => `${code}${params?.['count'] ? `:${params['count']}` : ''}`;

test('a missing backup target is named only once the list has answered empty', () => {
  assert.equal(backupColumnFigure(undefined, t), undefined);
  assert.equal(backupColumnFigure([{}], t), undefined);
  assert.equal(backupColumnFigure([], t), 'app.admin.figure_no_target');
});

test('the failed runs are counted, and nothing is said without one', () => {
  assert.equal(runsColumnFigure([{ status: 'SUCCEEDED' }], t), undefined);
  assert.equal(runsColumnFigure([{ status: 'FAILED' }, { status: 'SKIPPED' }, { status: 'FAILED' }], t), 'app.admin.figure_failed:2');
});

test('the privacy requests due soon or overdue are counted', () => {
  assert.equal(privacyColumnFigure(['ahead', undefined], t), undefined);
  assert.equal(privacyColumnFigure(['overdue', 'soon', 'ahead'], t), 'app.admin.figure_owed:2');
});
