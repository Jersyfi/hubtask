// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The desktop shell (ADR-0031): the web app inside the Tauri window on Windows and Linux, chosen
 * by `vite build --mode desktop` (`target.ts`). Everything it does is the shell's (`shell.ts`);
 * the desktop has every area of the web app.
 */
export { shellPlatform as platform } from './shell.ts';
