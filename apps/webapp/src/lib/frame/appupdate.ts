// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installed apps' offer of a newer release (UC-SYN-07 checks 5 and 6): looked for once at
 * start, shown in the frame's banner zone until the person installs it, says *Later* or closes a
 * refusal; *About Hubtask* keeps saying it after *Later*. The browser has no `updates` and never
 * asks.
 *
 * *Later* lasts as the platform says: until the next start on the desktop, which checks again
 * then; until a newer version than the one hidden in the Android app, remembered on this device.
 * Plain TypeScript, so a test holds it; `appupdate.svelte.ts` makes its fields reactive.
 */

import type { AppUpdate, AppUpdates } from '../platform/index.ts';

/** One field the page reads: a rune in the app, a plain holder in a test. */
export interface Cell<T> {
  value: T;
}

export const plainCell = <T>(value: T): Cell<T> => ({ value });

type DeviceStorage = Pick<Storage, 'getItem' | 'setItem'>;

export const LATER_KEY = 'hubtask.update.later';

export class UpdateOffer {
  #updates: AppUpdates | undefined;
  #storage: DeviceStorage | undefined;
  #found: Cell<AppUpdate | undefined>;
  #isHidden: Cell<boolean>;
  #isInstalling: Cell<boolean>;
  #started = false;

  constructor(updates: AppUpdates | undefined, storage: DeviceStorage | undefined, cell: <T>(value: T) => Cell<T> = plainCell) {
    this.#updates = updates;
    this.#storage = storage;
    this.#found = cell<AppUpdate | undefined>(undefined);
    this.#isHidden = cell(false);
    this.#isInstalling = cell(false);
  }

  /** What was found, whether or not the banner shows it: what *About Hubtask* reads. */
  get found(): AppUpdate | undefined {
    return this.#found.value;
  }

  /** What the banner shows now. */
  get banner(): AppUpdate | undefined {
    return this.#isHidden.value ? undefined : this.#found.value;
  }

  /** This app's version; nothing in the browser. */
  get current(): string | undefined {
    return this.#updates?.current;
  }

  get offersOn(): AppUpdates['offersOn'] | undefined {
    return this.#updates?.offersOn;
  }

  get isInstalling(): boolean {
    return this.#isInstalling.value;
  }

  /** Looks once per start; a second call is the same look. */
  async start(): Promise<void> {
    if (!this.#updates || this.#started) return;
    this.#started = true;
    const found = await this.#updates.check().catch(() => undefined);
    this.#isHidden.value = found !== undefined && this.#updates.laterLasts === 'version' && this.#readLater() === found.version;
    this.#found.value = found;
  }

  /** Hides the offer; a refusal's *Close* is the same. */
  later(): void {
    this.#isHidden.value = true;
    const found = this.#found.value;
    if (this.#updates?.laterLasts !== 'version' || !found) return;
    try {
      this.#storage?.setItem(LATER_KEY, found.version);
    } catch {
      // Not remembered: the notice comes back at the next start, which is the safe side.
    }
  }

  /** *Install and restart*, only for what the shell checked: the app ends here when it succeeds. */
  async install(): Promise<void> {
    if (!this.#updates || this.#found.value?.state !== 'ready') return;
    this.#isInstalling.value = true;
    try {
      await this.#updates.install();
    } catch {
      this.#isInstalling.value = false;
    }
  }

  /** Opens a page of the releases in the system's browser. */
  open(url: string): void {
    void this.#updates?.open(url).catch(() => {});
  }

  #readLater(): string | null {
    try {
      return this.#storage?.getItem(LATER_KEY) ?? null;
    } catch {
      return null;
    }
  }
}
