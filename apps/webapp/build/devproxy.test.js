// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The development server forwards the realtime socket as well as requests: `lib/data/engine.ts`
// opens its WebSocket under `/api/v1`, and a proxy without `ws` leaves every page and whiteboard
// of the documented dev walk at "Connecting…".

import { test } from 'node:test';
import assert from 'node:assert/strict';

import config from '../vite.config.ts';

test('the dev proxy forwards the API and its WebSocket upgrade', () => {
  // The configuration is chosen by the build's mode (`platform/target.ts`); the dev server is `serve`.
  const served = typeof config === 'function' ? config({ mode: 'development', command: 'serve' }) : config;
  const api = served.server?.proxy?.['/api'];
  assert.equal(typeof api, 'object');
  assert.equal(api.ws, true);
  assert.equal(api.changeOrigin, false);
});
