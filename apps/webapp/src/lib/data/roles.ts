// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Roles as the administration reads them (UC-ID-26): what a role editor offers, the *who may do
 * what* matrix, and what a person holds at a place. Pure functions, tested beside.
 *
 * **What is offered is what the reader holds** (D-05): the server refuses a role carrying a
 * permission its definer lacks, so offering one would be a button the server refuses. A field or a
 * step permission is held by the owner and the administrators as a family, which no list of
 * permissions can spell, so a shipped `OWNER` or `ADMIN` among the reader's sources stands for
 * every one of them.
 */

import type { AccessExplanation, CustomRole, PermissionEntry, RoleCatalogue } from '@hubtask/sync-engine';

export type PermissionKind = 'BASE' | 'FIELD' | 'STEP';

export const FIELD_WRITE = 'FIELD_WRITE:';
export const TAKE_STEP = 'TAKE_STEP:';

/** The family a permission belongs to, as the server sorts it. */
export function kindOf(permission: string): PermissionKind {
  if (permission.startsWith(FIELD_WRITE)) return 'FIELD';
  if (permission.startsWith(TAKE_STEP)) return 'STEP';
  return 'BASE';
}

/** Whether a permission names a family (`FIELD_WRITE:*`) rather than one field or step. */
export function isFamily(permission: string): boolean {
  return permission.endsWith(':*');
}

/** The roles that shape the workspace hold every field and step permission. */
const FAMILY_HOLDERS = ['OWNER', 'ADMIN'];

/** Whether the explained person holds a permission at the place explained. */
export function holds(explained: AccessExplanation | undefined, permission: string): boolean {
  if (!explained) return false;
  if (explained.permissions.includes(permission)) return true;
  if (kindOf(permission) === 'BASE') return false;
  return explained.sources.some((source) => !source.custom && FAMILY_HOLDERS.includes(source.role));
}

/**
 * Whether the reader writes a field (UC-ID-27 check 4): any field that is not restricted, and a
 * restricted one only with its `FIELD_WRITE:<key>` - never before the answer has arrived.
 */
export function mayWriteField(field: { key: string; restricted?: boolean }, explained: AccessExplanation | null | undefined): boolean {
  return field.restricted !== true || holds(explained ?? undefined, `${FIELD_WRITE}${field.key}`);
}

/** A permission a role editor shows, and whether the reader may set or clear it. */
export interface Offer {
  readonly permission: string;
  readonly kind: PermissionKind;
  /** The area of the administration it belongs to; `fields` and `workflows` for the families. */
  readonly group: string;
  /** False where the reader lacks it: drawn only when the role already carries it, and locked. */
  readonly isHeld: boolean;
}

/** The objects a field or step permission may name: restricted fields and published steps. */
export interface Objects {
  readonly restrictedFields: readonly string[];
  readonly steps: readonly { readonly workflow: string; readonly step: string }[];
}

/**
 * What a role editor offers: every permission of the catalogue the reader holds, each restricted
 * field's and each step's in place of their family, and - locked - what the role being changed
 * carries that the reader lacks, so saving keeps it rather than dropping it unseen.
 */
export function offers(
  catalogue: readonly PermissionEntry[],
  objects: Objects,
  explained: AccessExplanation | undefined,
  carried: readonly string[] = [],
): Offer[] {
  const out: Offer[] = [];
  const seen = new Set<string>();
  const add = (permission: string, kind: PermissionKind, group: string): void => {
    if (seen.has(permission)) return;
    const isHeld = holds(explained, permission);
    if (!isHeld && !carried.includes(permission)) return;
    seen.add(permission);
    out.push({ permission, kind, group, isHeld });
  };
  for (const entry of catalogue) {
    if (entry.kind === 'BASE') {
      add(entry.permission, 'BASE', entry.group);
    } else if (entry.kind === 'FIELD') {
      for (const key of objects.restrictedFields) add(`${FIELD_WRITE}${key}`, 'FIELD', entry.group);
    } else {
      for (const { workflow, step } of objects.steps) add(`${TAKE_STEP}${workflow}/${step}`, 'STEP', entry.group);
    }
  }
  // What the role carries that names an object no longer there: still shown, so it can be cleared.
  for (const permission of carried) {
    const kind = kindOf(permission);
    add(permission, kind, kind === 'FIELD' ? 'fields' : kind === 'STEP' ? 'workflows' : 'other');
  }
  return out;
}

/**
 * The permissions a save sends: those the reader set among what they hold, and whatever the role
 * carried that the reader may not touch - the server refuses a change to a permission its changer
 * lacks, adding or removing.
 */
export function chosen(offered: readonly Offer[], checked: ReadonlySet<string>, carried: readonly string[] = []): string[] {
  return offered
    .filter((offer) => (offer.isHeld ? checked.has(offer.permission) : carried.includes(offer.permission)))
    .map((offer) => offer.permission);
}

/** The key a new role takes: lower case, a letter first, as the contract's pattern says. */
export const ROLE_KEY = /^[a-z][a-z0-9_]{1,62}$/;

/** A key proposed from a name: `Service lead` → `service_lead`; empty where none fits. */
export function keyFrom(name: string): string {
  const key = name
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^[^a-z]+/, '')
    .replace(/_+$/, '')
    .slice(0, 63);
  return ROLE_KEY.test(key) ? key : '';
}

/** One row of the matrix: a role, shipped or the workspace's own. */
export interface MatrixRow {
  readonly id: string;
  readonly isCustom: boolean;
  /** A custom role's name; a shipped role is worded by the client. */
  readonly name?: string;
  readonly permissions: readonly string[];
}

/** The matrix's columns and rows. */
export interface Matrix {
  readonly columns: readonly string[];
  readonly rows: readonly MatrixRow[];
}

/**
 * *Who may do what* (A11): the catalogue's permissions against the roles. The columns are the
 * catalogue's base permissions and its two families, then each field or step permission a custom
 * role names; a shipped role carries what the catalogue says it holds, and the owner and the
 * administrators every field and step permission.
 *
 * Narrowed to `only` - the roles that count for a chosen person - where it is given.
 */
export function matrixOf(catalogue: RoleCatalogue, only?: readonly string[]): Matrix {
  const columns: string[] = catalogue.catalogue.map((entry) => entry.permission);
  for (const role of catalogue.custom) {
    for (const permission of role.permissions) {
      if (!columns.includes(permission)) columns.push(permission);
    }
  }
  const shippedRow = (role: string): MatrixRow => ({
    id: role,
    isCustom: false,
    permissions: columns.filter((column) => {
      const entry = catalogue.catalogue.find((each) => each.permission === column);
      if (entry) return entry.shipped.includes(role as never);
      return FAMILY_HOLDERS.includes(role);
    }),
  });
  const customRow = (role: CustomRole): MatrixRow => ({
    id: role.key,
    isCustom: true,
    name: role.name,
    permissions: role.permissions,
  });
  const rows = [...catalogue.shipped.map((role) => shippedRow(role.role)), ...catalogue.custom.map(customRow)];
  return { columns, rows: only ? rows.filter((row) => only.includes(row.id)) : rows };
}

/** How many places hold a custom role, for its row on the roles screen. */
export function grantsOf(catalogue: RoleCatalogue | undefined, key: string): RoleCatalogue['grants'] {
  return (catalogue?.grants ?? []).filter((grant) => grant.role === key);
}
