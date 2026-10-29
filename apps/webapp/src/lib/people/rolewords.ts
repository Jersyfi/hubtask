// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How a role and a permission of the catalogue are worded on the roles screens (UC-ID-26). A
 * permission this build has no words for is shown as the server's own token: the catalogue is the
 * server's, and hiding an unknown entry would say it means nothing.
 */

import type { CustomRole } from '@hubtask/sync-engine';

import { t } from '../i18n/i18n.svelte.ts';
import { FIELD_WRITE, TAKE_STEP } from '../data/roles.ts';

const DESCRIBED = new Set([
  'READ', 'WRITE_ITEMS', 'WRITE_ASSIGNED', 'COMMENT', 'STRUCTURE', 'AUTOMATION', 'DELETE_CONTAINER',
  'MANAGE_MEMBERS', 'READ_CONFIGURATION', 'AUDIT_READ', 'REQUEST',
]);

const SHIPPED = new Set(['OWNER', 'ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR']);

/** A permission's words: a base permission's name, a field's or a step's with the object named. */
export function permissionLabel(permission: string): string {
  if (permission === `${FIELD_WRITE}*`) return t('app.roles.family_field');
  if (permission === `${TAKE_STEP}*`) return t('app.roles.family_step');
  if (permission.startsWith(FIELD_WRITE)) return t('app.roles.field_write', { field: permission.slice(FIELD_WRITE.length) });
  if (permission.startsWith(TAKE_STEP)) {
    const [workflow = '', step = ''] = permission.slice(TAKE_STEP.length).split('/');
    return t('app.roles.take_step', { workflow, step });
  }
  return DESCRIBED.has(permission) ? t(`app.permissions.name.${permission.toLowerCase()}`) : permission;
}

/** A role's words: a shipped role's name, a custom role's own name, else its key. */
export function roleLabel(role: string, custom: readonly CustomRole[] = []): string {
  if (SHIPPED.has(role)) return t(`app.people.role.${role.toLowerCase()}`);
  return custom.find((each) => each.key === role)?.name ?? role;
}
