// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one set, and the one name space.
//
// Base and custom are merged here rather than kept apart at the call site, because a caller
// choosing between `<Icon name="tag" />` and `<CustomIcon name="bucket" />` is a caller who has to
// know which set an icon came from - which is exactly the thing this file exists to hide. Where a
// mark moves from one to the other, nothing at a call site changes.

import { BASE_ICONS } from './base.ts';
import { CUSTOM_ICONS } from './custom.ts';
import type { IconNode } from './node.ts';

/**
 * A name collision is a failure, not a precedence rule. Two icons under one name means one of them
 * is unreachable, and the day it happens is the day a `bucket` becomes something else - so the
 * check is here, at module load, and `icons.test.js` asserts it separately for the message.
 */
const overlap = Object.keys(CUSTOM_ICONS).filter((name) => name in BASE_ICONS);
if (overlap.length > 0) {
  throw new Error(
    `these names exist in both the base and the custom set: ${overlap.join(', ')}. ` +
      'Rename the custom mark, or drop the base icon from build/icons.js.',
  );
}

export const ICONS = { ...BASE_ICONS, ...CUSTOM_ICONS } as Record<string, readonly IconNode[]>;

/** Every name the product may ask for. Anything else does not compile. */
export type IconName = keyof typeof BASE_ICONS | keyof typeof CUSTOM_ICONS;

export const ICON_NAMES = Object.keys(ICONS).sort() as IconName[];

export { BASE_ICONS, CUSTOM_ICONS };
export type { IconNode };
