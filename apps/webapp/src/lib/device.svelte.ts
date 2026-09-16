// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The two preferences that belong to this device rather than to the account (ADR-0043, F5-12):
 * the theme and reduced motion. One store, so that the profile shows them side by side and
 * `main.ts` starts them with one call; each attribute is still set by its own module.
 */

import { applyMotion, readMotionChoice, storeMotionChoice, type MotionChoice } from './motion.ts';
import { readThemeChoice, startTheme, storeThemeChoice, type ThemeChoice } from './theme.ts';

/** `localStorage` where it can be reached; nothing where it cannot (a sandbox, a test). */
function storage() {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

class Device {
  #theme = $state<ThemeChoice>('system');
  #motion = $state<MotionChoice>('system');
  #stopTheme: (() => void) | undefined;

  get theme(): ThemeChoice {
    return this.#theme;
  }

  get motion(): MotionChoice {
    return this.#motion;
  }

  /** Reads what this device kept and applies it. Before the first paint, like the theme always was. */
  start(): void {
    this.#theme = readThemeChoice(storage());
    this.#motion = readMotionChoice(storage());
    this.#stopTheme = startTheme(this.#theme);
    applyMotion(document.documentElement, this.#motion);
  }

  setTheme(choice: ThemeChoice): void {
    this.#theme = choice;
    storeThemeChoice(storage(), choice);
    this.#stopTheme?.();
    this.#stopTheme = startTheme(choice);
  }

  setMotion(choice: MotionChoice): void {
    this.#motion = choice;
    storeMotionChoice(storage(), choice);
    applyMotion(document.documentElement, choice);
  }
}

export const device = new Device();
