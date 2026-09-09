<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper. Everything the matrix draws is handed in, so the demo is where the
  // manifest's shape is written out — a fixture standing in for `/meta/capabilities`'s `roles`.

  import PermissionMatrix, { type MatrixRole } from './PermissionMatrix.svelte';
  import type { Column } from './Table.svelte';

  const { mode = 'full' }: { mode?: 'full' | 'unknown' | 'permissions' } = $props();

  const permissions: Column[] = [
    { id: 'READ', label: 'Read' },
    { id: 'WRITE_ITEMS', label: 'Write' },
    { id: 'STRUCTURE', label: 'Structure' },
    { id: 'MANAGE_MEMBERS', label: 'People' },
    { id: 'AUTOMATION', label: 'Automation' },
    { id: 'DELETE_CONTAINER', label: 'Delete' },
    { id: 'READ_CONFIGURATION', label: 'Configuration' },
    { id: 'AUDIT_READ', label: 'Audit' },
  ];

  const accessKinds: Column[] = [
    { id: 'read', label: 'Read an entry' },
    { id: 'create', label: 'Create' },
    { id: 'change', label: 'Change' },
    { id: 'comment', label: 'Comment' },
  ];

  const accessLabels = { ALL: 'Any', ASSIGNED: 'Assigned only', NONE: 'No' };

  const roles: MatrixRole[] = [
    {
      id: 'OWNER',
      label: 'Owner',
      permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'AUTOMATION', 'DELETE_CONTAINER', 'READ_CONFIGURATION', 'AUDIT_READ'],
      itemAccess: { read: 'ALL', create: 'ALL', change: 'ALL', comment: 'ALL' },
    },
    {
      id: 'ADMIN',
      label: 'Admin',
      permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'AUTOMATION', 'READ_CONFIGURATION'],
      itemAccess: { read: 'ALL', create: 'ALL', change: 'ALL', comment: 'ALL' },
    },
    {
      id: 'MEMBER',
      label: 'Member',
      permissions: ['READ', 'WRITE_ITEMS'],
      itemAccess: { read: 'ALL', create: 'ALL', change: 'ALL', comment: 'ALL' },
    },
    {
      id: 'CONTRIBUTOR',
      label: 'Contributor',
      permissions: ['READ', 'WRITE_ITEMS'],
      // The row the qualifiers exist for: create is unqualified because a created entry is
      // assigned to its creator, and change is not.
      itemAccess: { read: 'ALL', create: 'ALL', change: 'ASSIGNED', comment: 'ALL' },
    },
    {
      id: 'VIEWER',
      label: 'Viewer',
      permissions: ['READ'],
      itemAccess: { read: 'ALL', create: 'NONE', change: 'NONE', comment: 'NONE' },
    },
    {
      id: 'GUEST',
      label: 'Guest',
      permissions: ['READ'],
      itemAccess: { read: 'ALL', create: 'NONE', change: 'NONE', comment: 'ALL' },
    },
    {
      id: 'AUDITOR',
      label: 'Auditor',
      permissions: ['READ_CONFIGURATION', 'AUDIT_READ'],
      itemAccess: { read: 'NONE', create: 'NONE', change: 'NONE', comment: 'NONE' },
    },
  ];

  /** An installation this build has never met: a role, a permission and a qualifier it cannot word. */
  const withUnknowns: MatrixRole[] = [
    ...roles.slice(0, 2),
    {
      id: 'DEPUTY',
      label: 'Deputy',
      permissions: ['READ', 'WRITE_ITEMS', 'ESCALATE'],
      itemAccess: { read: 'ALL', create: 'ALL', change: 'DELEGATED', comment: 'ALL' },
    },
  ];
</script>

{#if mode === 'permissions'}
  <PermissionMatrix
    label="What each role may do"
    roleColumnLabel="Role"
    {permissions}
    {roles}
    carriedLabel="Yes"
    notCarriedLabel="No"
  />
{:else if mode === 'unknown'}
  <PermissionMatrix
    label="What each role may do"
    roleColumnLabel="Role"
    {permissions}
    {accessKinds}
    {accessLabels}
    roles={withUnknowns}
    carriedLabel="Yes"
    notCarriedLabel="No"
  />
{:else}
  <PermissionMatrix
    label="What each role may do"
    roleColumnLabel="Role"
    {permissions}
    {accessKinds}
    {accessLabels}
    {roles}
    carriedLabel="Yes"
    notCarriedLabel="No"
  />
{/if}
