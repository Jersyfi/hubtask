// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * *View as file* in a page's head (page-system §6, the mockup's sign-in): a quiet secondary act,
 * drawn only for a reader who may read the configuration (P-05). The dialog is
 * `ViewSectionsAsFile` with its own trigger hidden.
 */

import type { PageAction } from '@hubtask/design-system/components';

import { holdsHere } from '../data/capability.svelte.ts';
import { t } from '../i18n/i18n.svelte.ts';

/** Whether the reader may read the workspace's configuration document. */
export function mayReadConfiguration(): boolean {
  return holdsHere('STRUCTURE') === true || holdsHere('READ_CONFIGURATION') === true;
}

/** The head's *View as file*, or nothing for a reader who may not read the file. */
export function viewAsFileAction(onclick: () => void): PageAction | undefined {
  return mayReadConfiguration() ? { label: t('app.settings.view_as_file'), isQuiet: true, onclick } : undefined;
}
