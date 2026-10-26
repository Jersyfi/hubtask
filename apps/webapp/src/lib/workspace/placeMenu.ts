// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's or a collection's own menu, built once for every place it is offered: its page head and
 * its row in the navigation draw the same items in the same order (design-system.md §11.8, §13
 * G7). Only what is done to the place itself - its settings, its place in the tree, its archive,
 * its privacy, the trash - and only to a reader who may; the page runs each item.
 */

import type { MenuItem } from '@hubtask/design-system/components';

export interface PlaceMenuInput {
  readonly type: 'HUB' | 'COLLECTION';
  /** Whether the reader may change the shape: everything but the trash asks it (UC-WRK-01 check 6). */
  readonly mayShape: boolean;
  /** Whether the reader may put the place in the trash: the owner's alone. */
  readonly mayDelete: boolean;
  /** `inherited` is archived above: nothing to archive or restore here. */
  readonly archival: 'active' | 'archived' | 'inherited';
  /** Turning a hub private or shared, where the reader may (UC-ID-16). */
  readonly privacy?: 'make_private' | 'make_shared';
  /** Why nothing can be changed now, kept on the items that change something. */
  readonly readOnlyReason?: string;
}

/** The items, grouped: set it up, place it, keep it, and the trash last and alone. */
export function placeMenu(input: PlaceMenuInput, t: (code: string) => string): MenuItem[] {
  const shaping: MenuItem[] = !input.mayShape
    ? []
    : [
        { id: 'settings', label: t('app.place.settings'), icon: 'settings', disabledReason: input.readOnlyReason },
        ...(input.type === 'COLLECTION'
          ? [{ id: 'move', label: t('app.move.to_hub'), icon: 'arrow-right-left' as const, disabledReason: input.readOnlyReason }]
          : []),
        { id: 'import', label: t('app.import.open'), icon: 'cloud-upload', disabledReason: input.readOnlyReason },
        { id: 'position', label: t('app.place.position'), icon: 'chevrons-up-down', disabledReason: input.readOnlyReason, hasSeparatorBefore: true },
        ...(input.archival !== 'inherited'
          ? [{ id: 'archive', label: input.archival === 'archived' ? t('app.workspace.unarchive') : t('app.workspace.archive'), icon: 'archive' as const }]
          : []),
      ];
  return [
    ...shaping,
    ...(input.privacy
      ? [{ id: 'privacy', label: input.privacy === 'make_shared' ? t('app.workspace.make_shared') : t('app.workspace.make_private'), icon: 'lock' as const, hasSeparatorBefore: shaping.length > 0 }]
      : []),
    ...(input.mayDelete
      ? [{ id: 'trash', label: t('app.workspace.trash'), icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true, disabledReason: input.readOnlyReason }]
      : []),
  ];
}
