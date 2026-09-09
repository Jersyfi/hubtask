// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// `OneTimeSecret`'s two decisions, and the discipline the component itself has to keep.
//
// The second half is a source reading rather than a rendering test, for the reason
// conventions.test.js gives: what a person cannot check by looking is whether the rule held
// everywhere, every time. Here the rule is that the value goes nowhere but the DOM — a component's
// own state dies with the component, so what has to be proved is that nothing was copied out of it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { MASK_LENGTH, canCopy, mask, mayDismiss } from '../src/secret.ts';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const component = fs.readFileSync(path.join(packageRoot, 'src', 'OneTimeSecret.svelte'), 'utf8');

test('the mask tells nothing about the value', () => {
  const short = mask('a');
  const long = mask('ZP4T-K9WQ-3MNB-7XRD-2LFV-8HGS-6CJY-1AEU');
  assert.equal(short, long, 'a mask as long as the value tells a shoulder-surfer how long it is');
  assert.equal(short.length, MASK_LENGTH);
  for (const character of 'ZP4TK9WQ') {
    assert.ok(!short.includes(character), `the mask contains ${character} from the value`);
  }
});

test('an acknowledgement gates the dismissal only where one is required', () => {
  assert.equal(mayDismiss({ isRequired: false, hasAcknowledged: false }), true);
  assert.equal(mayDismiss({ isRequired: true, hasAcknowledged: false }), false);
  assert.equal(mayDismiss({ isRequired: true, hasAcknowledged: true }), true);
});

test('a browser with no clipboard is not an error', () => {
  assert.equal(canCopy({ writeText: () => {} }), true);
  assert.equal(canCopy({}), false, 'an insecure origin has no writeText');
  assert.equal(canCopy(undefined), false);
  assert.equal(canCopy(null), false);
});

test('the component holds the value nowhere that outlives it', () => {
  // A component's own state is destroyed with the component, so the only way a one-time value
  // survives an unmount is if something copied it out. These are the ways that happens.
  for (const escape of [
    'localStorage',
    'sessionStorage',
    'indexedDB',
    'document.cookie',
    'history.pushState',
    'history.replaceState',
  ]) {
    assert.ok(!component.includes(escape), `OneTimeSecret writes the value to ${escape}`);
  }
  // A `<script module>` block runs once per module rather than once per instance: a value held
  // there would outlive every mount, which is exactly what this component may not do.
  assert.ok(
    !/<script[^>]*\bmodule\b/.test(component),
    'OneTimeSecret has module-level state, which outlives the component that held the value',
  );
});

test('the component drops what it was showing when it goes', () => {
  // Not the value - that is the caller's prop and goes with the caller. What this clears is the
  // fact that it *was* revealed, so a panel reopened for a different secret does not open already
  // showing it.
  assert.match(
    component,
    /\$effect\(\(\)\s*=>\s*\(\)\s*=>\s*\{/,
    'OneTimeSecret has no teardown, so a remount would reopen in the state the last one left',
  );
});
