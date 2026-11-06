// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import { fromShell, isNewer, noticeFrom, productVersion, releasePage } from './update.ts';

/** The release's manifest as `release-apps.js` writes it, with the APK among its platforms. */
function manifest(version: string, platforms: Record<string, unknown> = { android: { url: 'https://example.invalid/hubtask.apk', signature: 'x' } }) {
  return JSON.stringify({ version, notes: 'https://example.invalid/tag', pub_date: '2026-10-29T10:00:00.000Z', platforms });
}

test('the Android app says when the manifest names a newer release, and opens its release page (UC-SYN-07 check 6)', () => {
  const found = noticeFrom(manifest('1.3.0+57'), '1.2.0');
  assert.deepEqual(found, { state: 'available', version: '1.3.0' });
  assert.equal(releasePage(found?.version ?? ''), 'https://github.com/Jersyfi/hubtask/releases/tag/v1.3.0');
});

test('the Android app says nothing for the same or an older release, whatever their build counters', () => {
  assert.equal(noticeFrom(manifest('1.2.0+60'), '1.2.0'), undefined);
  assert.equal(noticeFrom(manifest('1.1.9+70'), '1.2.0'), undefined);
});

test('the Android app says nothing for a manifest it cannot read, or one without an APK', () => {
  assert.equal(noticeFrom('not json', '1.2.0'), undefined);
  assert.equal(noticeFrom('null', '1.2.0'), undefined);
  assert.equal(noticeFrom(manifest('one point three'), '1.2.0'), undefined);
  assert.equal(noticeFrom(manifest('1.3.0+57', { 'windows-x86_64': { url: 'https://example.invalid/setup.exe' } }), '1.2.0'), undefined);
});

test('the notice never opens an address the manifest gives', () => {
  const found = noticeFrom(manifest('1.3.0+57', { android: { url: 'https://elsewhere.example/hubtask.apk' } }), '1.2.0');
  assert.ok(!releasePage(found?.version ?? '').includes('elsewhere'));
});

test('versions compare by semantic versioning, the build counter aside', () => {
  const cases: Array<[string, string, boolean]> = [
    ['1.3.0', '1.2.9', true],
    ['1.2.10', '1.2.9', true],
    ['2.0.0', '1.99.99', true],
    ['1.3.0', '1.3.0-rc.1', true],
    ['1.3.0-rc.2', '1.3.0-rc.1', true],
    ['1.3.0-rc.10', '1.3.0-rc.9', true],
    ['1.3.0-rc.1', '1.3.0', false],
    ['1.3.0+58', '1.3.0+57', false],
    ['1.2.0', '1.3.0', false],
    ['garbage', '1.0.0', false],
  ];
  for (const [candidate, current, newer] of cases) assert.equal(isNewer(candidate, current), newer, `${candidate} after ${current}`);
  assert.equal(productVersion('1.3.0+57'), '1.3.0');
  assert.equal(productVersion('1.3.0-rc.1+57'), '1.3.0-rc.1');
});

test("the desktop updater's answer reads as each state the shell answers (UC-SYN-07 check 5)", () => {
  assert.deepEqual(fromShell({ state: 'ready', version: '1.3.0+57', refused: '' }), { state: 'ready', version: '1.3.0' });
  assert.deepEqual(fromShell({ state: 'available', version: '1.3.0+57', refused: '' }), { state: 'available', version: '1.3.0' });
  assert.deepEqual(fromShell({ state: 'refused', version: '1.3.0+57', refused: 'package' }), { state: 'refused', version: '1.3.0', refused: 'package' });
  assert.deepEqual(fromShell({ state: 'refused', version: '1.3.0+57', refused: 'manifest' }), { state: 'refused', version: '1.3.0', refused: 'manifest' });
  assert.equal(fromShell({ state: 'current', version: '', refused: '' }), undefined);
  assert.equal(fromShell({}), undefined);
});
