// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// LazyView, the frame of every screen split by route (F-10): rendered on the server, a screen the
// module system already holds is drawn with the props it is handed, and one still loading draws
// nothing rather than a half screen. The browser's own fetch of a chunk is the build's to prove
// (chunks.test.js).

import { register } from 'node:module';
import { test } from 'node:test';
import assert from 'node:assert/strict';

register('./svelte-loader.js', import.meta.url);

globalThis.window ??= globalThis;
Object.defineProperty(globalThis, 'navigator', { value: { languages: ['en'], language: 'en', onLine: true }, configurable: true });
globalThis.localStorage ??= { getItem: () => null, setItem() {}, removeItem() {} };
globalThis.document ??= {
  documentElement: { setAttribute() {}, removeAttribute() {}, getAttribute() { return null; } },
  addEventListener() {},
  removeEventListener() {},
};

test('a loaded screen is drawn with its props, a loading one draws nothing', async () => {
  const { render } = await import('svelte/server');
  const LazyView = (await import('../src/lib/frame/LazyView.svelte')).default;
  const Greeting = (await import('./fixtures/Greeting.svelte')).default;

  const loaded = render(LazyView, { props: { load: () => ({ default: Greeting }), props: { name: 'Ada' } } });
  assert.match(loaded.body, /<p data-greeting[^>]*>Ada<\/p>/);

  const loading = render(LazyView, { props: { load: () => new Promise(() => {}), props: { name: 'Ada' } } });
  assert.ok(!loading.body.includes('data-greeting'), 'a screen still loading was drawn');
  assert.ok(!loading.body.includes('<h1'), 'a screen still loading said it could not be loaded');
});
