// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Today is the reader's day. `new Date().toISOString().slice(0, 10)` - or the same of `Date.now()`
// moved by some time - is today in Greenwich: in New York at eight in the evening it is tomorrow,
// in Berlin after midnight yesterday. `todayIn(zone)` in `lib/i18n/zone.ts` is the one way; date
// arithmetic on a UTC-midnight value (`Date.UTC(...)`) is not today and stays allowed.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
const GREENWICH_TODAY = /new Date\((?:\s*Date\.now\(\)[^)]*)?\)\.toISOString\(\)\.slice\(0,\s*10\)/;

function sources(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return sources(full);
    return /\.(ts|svelte)$/.test(entry.name) && !/\.test\.ts$/.test(entry.name) ? [full] : [];
  });
}

test('no screen takes today as the day in Greenwich', () => {
  const offending = sources(SRC).flatMap((file) =>
    fs.readFileSync(file, 'utf8').split('\n').flatMap((line, index) =>
      GREENWICH_TODAY.test(line) ? [`${path.relative(SRC, file)}:${index + 1}`] : []));
  assert.deepEqual(offending, [], `today in Greenwich, use todayIn(zone): ${offending.join(', ')}`);
});
