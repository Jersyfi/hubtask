// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { MenuItem } from '@hubtask/design-system/components';
import type { AdminTenant } from '@hubtask/sync-engine';

import { t } from '../i18n/i18n.svelte.ts';

/** What the installation allows around a workspace's acts. */
export interface WorkspaceActsContext {
  /** Several workspaces: suspending, deleting and changing an address exist only there. */
  readonly isMulti: boolean;
  /** Plans exist, so a workspace can move between them. */
  readonly hasPlans: boolean;
}

/**
 * A workspace's acts as its ⋯ menu offers them, in the list's row and on its record page alike:
 * what the row's state permits, the destructive one last (page-system PS-6). *Cancel deletion*
 * stands until the removal has begun; a legal hold leaves no deletion (P-05).
 */
export function workspaceActs(workspace: AdminTenant, context: WorkspaceActsContext): MenuItem[] {
  const items: MenuItem[] = [];
  if (workspace.status === 'ACTIVE' && context.isMulti) items.push({ id: 'suspend', label: t('app.instance.suspend') });
  if (workspace.status === 'PENDING_DELETION' && !workspace.purge_started_at) {
    items.push({ id: 'cancel-deletion', label: t('app.instance.cancel_deletion') });
  }
  if (workspace.status === 'SUSPENDED') items.push({ id: 'resume', label: t('app.instance.resume') });
  if (context.isMulti && workspace.status !== 'PENDING_DELETION') {
    items.push({ id: 'change-address', label: t('app.instance.address_action') });
  }
  if (context.hasPlans && !workspace.purge_started_at) items.push({ id: 'move', label: t('app.instance.move_action') });
  items.push({ id: 'quotas', label: t('app.instance.quotas') });
  items.push({ id: 'export', label: t('app.instance.export') });
  if (workspace.password_opening) items.push({ id: 'close-password', label: t('app.instance.password_close') });
  else if (workspace.status === 'ACTIVE') items.push({ id: 'open-password', label: t('app.instance.password_open_action') });
  if (context.isMulti && workspace.status !== 'PENDING_DELETION' && !workspace.legal_hold) {
    items.push({ id: 'delete', label: t('app.instance.delete'), isDestructive: true, hasSeparatorBefore: true });
  }
  return items;
}
