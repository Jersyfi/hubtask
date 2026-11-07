// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a change of structure reaches, as the save bar says it before the save (configuration/ui.md
 * A3): "37 entries no longer shown · 2 views no longer offered". The numbers are the server's
 * preview (`previewContainerStructure`); this only phrases them. Pure, and tested beside itself.
 */

import type { StructurePreview } from '@hubtask/sync-engine';

import type { Translate } from '../palette/sources.ts';

/** The save bar's line for a preview: what stops showing, or that nothing does. */
export function reachOf(preview: StructurePreview, t: Translate): string {
  if (!preview.narrows) return t('app.settings.reach_nothing');
  const parts: string[] = [];
  const hidden = preview.levels.reduce((sum, level) => sum + level.entries, 0);
  if (hidden > 0) parts.push(t('app.settings.reach_entries', { count: String(hidden) }));
  const values = preview.fields.reduce((sum, field) => sum + field.entries, 0);
  if (values > 0) parts.push(t('app.settings.reach_values', { count: String(values) }));
  if (preview.views.length > 0) parts.push(t('app.settings.reach_views', { count: String(preview.views.length) }));
  return parts.length > 0 ? parts.join(' · ') : t('app.settings.reach_nothing');
}
