// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The parts a screen loads on demand, each a chunk of its own (F-10): a place's settings and the
// editors an entry opens. Kept in one table so the app precaches them with the screens - a part
// loaded only inside its screen would be missing offline until it had been opened once
// (offline-sync.md §13).

import { blockLibraries } from '@hubtask/editor/libraries';

/** A place's settings: the work of a place draws none of them. */
export const settingsParts = {
  StructurePanel: () => import('../workspace/StructurePanel.svelte'),
  ModuleSwitches: () => import('../workspace/ModuleSwitches.svelte'),
  CustomFieldsDialog: () => import('../entries/CustomFieldsDialog.svelte'),
  LabelsDialog: () => import('../entries/LabelsDialog.svelte'),
  PoliciesDialog: () => import('../workspace/PoliciesDialog.svelte'),
  HandleField: () => import('../mail/HandleField.svelte'),
  KeyPrefixField: () => import('../entries/KeyPrefixField.svelte'),
  PageDraftsSwitch: () => import('../pages/PageDraftsSwitch.svelte'),
  ContainerEditDialog: () => import('../workspace/ContainerEditDialog.svelte'),
  HubCalendars: () => import('../servicedesk/HubCalendars.svelte'),
  HubSlaPolicies: () => import('../servicedesk/HubSlaPolicies.svelte'),
  TemplatesGallery: () => import('../settings/TemplatesGallery.svelte'),
  SampleEntries: () => import('../settings/SampleEntries.svelte'),
  CodeHooks: () => import('../code/CodeHooks.svelte'),
  ChatBots: () => import('../chat/ChatBots.svelte'),
};

/** The editors an entry opens on demand: an entry's first paint carries none of them. */
export const entryParts = {
  RecurrencePanel: () => import('../entries/RecurrencePanel.svelte'),
  ReminderPanel: () => import('../entries/ReminderPanel.svelte'),
  CoverPanel: () => import('../media/CoverPanel.svelte'),
};

/** What a page's blocks load on demand: the emoji picker's sets and the editor's libraries. */
export const emojiSets = {
  de: () => import('emojibase-data/de/compact.json'),
  en: () => import('emojibase-data/en/compact.json'),
};

/** Every part's loader, for the precache. */
export function partLoaders(): (() => Promise<unknown>)[] {
  return [...Object.values(settingsParts), ...Object.values(entryParts), ...Object.values(emojiSets), ...Object.values(blockLibraries)];
}
