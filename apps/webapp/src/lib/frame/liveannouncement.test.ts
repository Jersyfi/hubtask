// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { liveAnnouncement } from './liveannouncement.ts';

test('a page that connects for the first time says nothing, before or after the language arrived', () => {
  assert.equal(liveAnnouncement(undefined, 'reconnecting', false), undefined);
  assert.equal(liveAnnouncement('off', 'reconnecting', false), undefined);
  assert.equal(liveAnnouncement('reconnecting', 'live', false), undefined);
});

test('losing the connection once it was had, and getting it back, are said', () => {
  assert.equal(liveAnnouncement('live', 'reconnecting', true), 'app.live.lost');
  assert.equal(liveAnnouncement('reconnecting', 'live', true), 'app.live.became_live');
});

test('the device losing its network is said whenever it happens; nothing is said twice', () => {
  assert.equal(liveAnnouncement('reconnecting', 'offline', false), 'app.live.device_offline');
  assert.equal(liveAnnouncement('live', 'offline', true), 'app.live.device_offline');
  assert.equal(liveAnnouncement('live', 'live', true), undefined);
  assert.equal(liveAnnouncement('live', 'off', true), undefined);
});
