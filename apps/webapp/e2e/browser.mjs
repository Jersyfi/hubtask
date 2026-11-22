// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One Chromium per suite file, shared by its walks. Each walk still starts from nothing: what a
// walk opens lives in its own browser contexts - storage, cookies, routes, pages - and every
// context is closed when the walk ends, as closing a browser of its own would. Launching one per
// walk cost a browser start for each of about a thousand walks.

import { test } from 'node:test';

import { chromium } from 'playwright';

import './record.mjs';

let launched;

test.after(async () => {
  if (launched) await (await launched).close();
});

/**
 * The suite's Chromium for walk `t`. The contexts the walk opens through it are closed when the
 * walk ends - its own only, so walks that run side by side keep theirs.
 */
export async function sharedChromium(t) {
  launched ??= chromium.launch();
  const browser = await launched;
  const opened = new Set();
  t.after(() => Promise.all([...opened].map((context) => context.close().catch(() => undefined))));
  return new Proxy(browser, {
    get(target, name) {
      if (name === 'newContext') return async (...options) => {
        const context = await target.newContext(...options);
        opened.add(context);
        return context;
      };
      if (name === 'newPage') return async (...options) => {
        const page = await target.newPage(...options);
        opened.add(page.context());
        return page;
      };
      const value = Reflect.get(target, name, target);
      return typeof value === 'function' ? value.bind(target) : value;
    },
  });
}
