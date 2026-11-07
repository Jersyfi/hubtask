// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { LIVE_UNREACHABLE_AFTER, isLiveUnreachable } from './liveunreachable.ts';

test('the live channel is out of reach only after repeated failures while the stream is live', () => {
  assert.ok(isLiveUnreachable('offline', LIVE_UNREACHABLE_AFTER, 'live'));
  assert.ok(!isLiveUnreachable('offline', LIVE_UNREACHABLE_AFTER - 1, 'live'), 'one dropped connection is not a proxy');
  assert.ok(!isLiveUnreachable('offline', 9, 'offline'), 'a device without a network is offline, not behind a proxy');
  assert.ok(!isLiveUnreachable('offline', 9, 'reconnecting'), 'a server that does not answer at all is not a proxy');
  assert.ok(!isLiveUnreachable('connected', 9, 'live'));
});
