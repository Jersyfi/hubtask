// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether a part of the people panel is drawn, by what the manifest says of the entry's type.
 *
 * A capability the type refuses is refused outright: the part is absent, not drawn with the
 * refusal's reason - an activity has no member list and never will, so a section saying so is a
 * control the reader can never use (design-system.md §11). Until the manifest is read the part
 * waits behind the gate's pending label rather than being guessed either way.
 */

import type { Verdict } from '../data/capability.ts';

export type Part = 'drawn' | 'deciding' | 'absent';

export function partOf(verdict: Verdict): Part {
  switch (verdict.status) {
    case 'permitted':
      return 'drawn';
    case 'undetermined':
      return 'deciding';
    default:
      return 'absent';
  }
}
