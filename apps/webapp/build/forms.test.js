// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A file control is the design system's UploadField (user-experience.md §5.10): a native file
// input drawn by a screen shows the browser's own control, whose focus ring and words are not
// ours (WCAG 2.4.7). Read from the source, because a new screen copies its neighbour's shape.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');

function sources(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return sources(full);
    return entry.name.endsWith('.svelte') ? [full] : [];
  });
}

test('no screen draws a native file input of its own', () => {
  const found = sources(SRC)
    .filter((file) => /type="file"/.test(fs.readFileSync(file, 'utf8')))
    .map((file) => path.relative(SRC, file));
  assert.deepEqual(found, [], 'use UploadField (variant "button" in a toolbar)');
});

test('no screen asks for a time zone as free text', () => {
  // A zone is a ZonePicker (user-experience.md §5.4): a list from the platform, the device's zone
  // by default, and free text only inside it, where the platform lists none.
  const found = [];
  for (const file of sources(SRC)) {
    if (file.endsWith(`${path.sep}ZonePicker.svelte`)) continue;
    const source = fs.readFileSync(file, 'utf8');
    for (const tag of source.matchAll(/<Input\b[^>]*?label=\{t\('([a-z_.]+)'\)/gs)) {
      if (/(^|[._])(zone|timezone|time_zone)(_field)?$/.test(tag[1])) found.push(`${path.relative(SRC, file)}: ${tag[1]}`);
    }
  }
  assert.deepEqual(found, [], 'use ZonePicker');
});

test('no screen asks for a recurrence rule as its only control', () => {
  // A schedule is a FrequencyPicker (user-experience.md §5.4): daily, weekly or monthly at an hour,
  // the raw rule behind Advanced inside it - nobody has to know RFC 5545 to set a nightly job.
  const found = [];
  for (const file of sources(SRC)) {
    if (file.endsWith(`${path.sep}FrequencyPicker.svelte`)) continue;
    const source = fs.readFileSync(file, 'utf8');
    for (const tag of source.matchAll(/<Input\b[^>]*?label=\{t\('([a-z_.]+)'\)/gs)) {
      if (/(^|[._])(rrule|schedule_rule)$/.test(tag[1])) found.push(`${path.relative(SRC, file)}: ${tag[1]}`);
    }
  }
  assert.deepEqual(found, [], 'use FrequencyPicker');
});
