// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { invoke } from '@tauri-apps/api/core';

import type { BrowserSignIn, Platform } from './index.ts';
import { awaitReturn, shellPlatform, shellSignIn } from './shell.ts';
import { noticeFrom } from './update.ts';

/**
 * The Android app (ADR-0031, ADR-0032): the desktop's shell (`shell.ts`) with what only a phone
 * does, chosen by `vite build --mode mobile` (`target.ts`).
 *
 * - The sign-in's code comes back to the app's own scheme, `eu.hubtask.app:/oauth` (identity.md
 *   §15.3, RFC 8252 §7.1), which Android hands to the app; the shell sends it on as the desktop's
 *   listener does. A passkey step-up still answers at the loopback listener (`keyRelay.ts`).
 * - The workspace's administration and the installation's area are not in this bundle; their rows
 *   name the web app of the server the app is signed in to and open it there (UC-SYN-07 check 7).
 * - An entry's link goes to the system's share sheet (check 8), and the system bars take the
 *   page's colour.
 * - A newer release is a notice on *Today* that opens its download (check 6, ADR-0097): Tauri's
 *   updater has no Android, and the app asks for no permission to install packages.
 */

/** Where the built-in client's consent sends the code on Android (`AppRedirectAndroid`). */
export const ANDROID_REDIRECT = 'eu.hubtask.app:/oauth';

const signIn: BrowserSignIn = Object.create(shellSignIn, {
  listenForSignIn: {
    value: async (_page: unknown, returned: Parameters<BrowserSignIn['listenForSignIn']>[1]) => {
      awaitReturn(returned);
      return ANDROID_REDIRECT;
    },
  },
});

export const platform: Platform = {
  ...shellPlatform,
  target: 'mobile',
  browserSignIn: signIn,
  webApp: {
    areas: ['administration', 'instance'],
    open: (url) => invoke('open_in_browser', { url }),
  },
  sendLink: (title, url) => invoke('send_link', { title, url }),
  paintBars: (background, isDark) => {
    void invoke('paint_bars', { background, isDark }).catch(() => {});
  },
  updates: shellPlatform.updates && {
    ...shellPlatform.updates,
    offersOn: 'today',
    laterLasts: 'version',
    check: async () => {
      const manifest = await invoke<string>('release_manifest').catch(() => undefined);
      return manifest === undefined ? undefined : noticeFrom(manifest, shellPlatform.updates?.current ?? '');
    },
    install: () => Promise.reject(new Error('update.not_on_this_system')),
  },
};
