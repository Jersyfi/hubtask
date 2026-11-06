// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** The app's one offer of a newer release (`appupdate.ts`), with fields the page reacts to. */

import { platform } from '../platform/index.ts';
import { UpdateOffer, type Cell } from './appupdate.ts';

/** The device's storage, or nothing where it cannot be reached (a sandbox, a test). */
function deviceStorage(): Pick<Storage, 'getItem' | 'setItem'> | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

function runeCell<T>(initial: T): Cell<T> {
  let value = $state(initial);
  return {
    get value() {
      return value;
    },
    set value(next: T) {
      value = next;
    },
  };
}

export const appUpdate = new UpdateOffer(platform.updates, deviceStorage(), runeCell);
