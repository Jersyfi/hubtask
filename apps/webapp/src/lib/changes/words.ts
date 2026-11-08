// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** A configuration version's and proposal's words, shared by the log and a version's page. */

import type { ConfigTemplateList, ResourceState } from '@hubtask/sync-engine';

import { messages, t } from '../i18n/i18n.svelte.ts';

/** A template by its name, never as `key@version`. */
export function templateNameOf(catalogue: ResourceState<ConfigTemplateList> | undefined, provenance: string): string {
  const key = provenance.split('@')[0] ?? provenance;
  const known = catalogue?.status === 'ready' ? catalogue.data.data.find((each) => each.provenance === provenance || each.key === key) : undefined;
  const preset = `app.structure.preset.${key.toUpperCase()}`;
  if (known?.origin === 'HUBTASK' || (!known && messages.has(preset))) return messages.has(preset) ? t(preset) : (known?.name ?? key);
  return known?.name ?? key;
}

/** *1 added · 2 changed · 0 removed*. */
export const countsOf = (each: { added: number; changed: number; removed: number }): string =>
  t('app.changes.counts', { added: each.added, changed: each.changed, removed: each.removed });
