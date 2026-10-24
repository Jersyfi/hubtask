// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The preferences that belong to this device rather than to the account (ADR-0043): the theme,
 * reduced motion, how an entry opens from a list and how wide its panel is (design-system.md
 * §11.8, §11.10). One store, so that the profile shows them side by side and `main.ts` starts them
 * with one call; each attribute is still set by its own module.
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

/** Whether an entry chosen in a list opens in the panel beside it or whole, as its page. */
export type OpenMode = 'panel' | 'whole';

const OPEN_KEY = 'hubtask.entries.open';
const PANEL_KEY = 'hubtask.panel.width';

class Device {
  #theme = $state<ThemeChoice>('system');
  #motion = $state<MotionChoice>('system');
  #openMode = $state<OpenMode>('panel');
  #panelWidth = $state<number | undefined>(undefined);
  #stopTheme: (() => void) | undefined;

  get theme(): ThemeChoice {
    return this.#theme;
  }

  get motion(): MotionChoice {
    return this.#motion;
  }

  get openMode(): OpenMode {
    return this.#openMode;
  }

  /** The width the person dragged the panel to; none until they did. */
  get panelWidth(): number | undefined {
    return this.#panelWidth;
  }

  /** Reads what this device kept and applies it. Before the first paint, like the theme always was. */
  start(): void {
    this.#theme = readThemeChoice(storage());
    this.#motion = readMotionChoice(storage());
    try {
      this.#openMode = storage()?.getItem(OPEN_KEY) === 'whole' ? 'whole' : 'panel';
      const width = Number.parseFloat(storage()?.getItem(PANEL_KEY) ?? '');
      this.#panelWidth = Number.isFinite(width) && width > 0 ? width : undefined;
    } catch {
      // A browser that refuses storage opens entries in the panel at its first width.
    }
    this.#stopTheme = startTheme(this.#theme);
    applyMotion(document.documentElement, this.#motion);
  }

  setTheme(choice: ThemeChoice): void {
    this.#theme = choice;
    storeThemeChoice(storage(), choice);
    this.#stopTheme?.();
    this.#stopTheme = startTheme(choice);
  }

  setOpenMode(mode: OpenMode): void {
    this.#openMode = mode;
    try {
      storage()?.setItem(OPEN_KEY, mode);
    } catch {
      // Kept for as long as the page is open.
    }
  }

  setPanelWidth(width: number): void {
    this.#panelWidth = width;
    try {
      storage()?.setItem(PANEL_KEY, String(width));
    } catch {
      // Kept for as long as the page is open.
    }
  }

  setMotion(choice: MotionChoice): void {
    this.#motion = choice;
    storeMotionChoice(storage(), choice);
    applyMotion(document.documentElement, choice);
  }
}

export const device = new Device();
