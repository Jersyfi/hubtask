<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What each role actually means here.
  //
  // **Everything on this screen comes from `/meta/capabilities`.** Not one role, permission or
  // qualifier is compiled in: the manifest answers the matrix, and the contract says why in its own
  // words — "a client that does not know them offers buttons the server refuses". An installation
  // that adds a role gets a row here without a release of this client.
  //
  // **The columns are the union of what the manifest reports**, in the order it reports it, so a
  // permission this build has never heard of still gets a column. Its heading is the server's own
  // token, because inventing a sentence for a permission nobody has described would be worse than
  // showing the name it actually has.
  //
  // **Nothing here is editable, and that is not a missing feature.** Changing what a role carries
  // is not an operation this product has: a role is granted, and this says what the grant means.

  import { PermissionMatrix, Stack, type MatrixRole } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  /** The permissions this build has wording for. One it does not is shown as its own token. */
  const DESCRIBED = new Set([
    'READ',
    'WRITE_ITEMS',
    'STRUCTURE',
    'MANAGE_MEMBERS',
    'AUTOMATION',
    'DELETE_CONTAINER',
    'READ_CONFIGURATION',
    'AUDIT_READ',
  ]);

  /** The same, for the four kinds of access into a single entry. */
  const KINDS = ['read', 'create', 'change', 'comment'];

  const described = $derived(manifest.value?.roles ?? []);

  /** Every permission any role carries, in first-seen order — the manifest's own order. */
  const permissions = $derived.by(() => {
    const seen: string[] = [];
    for (const role of described) {
      for (const permission of role.permissions ?? []) {
        if (!seen.includes(permission)) seen.push(permission);
      }
    }
    return seen.map((id) => ({
      id,
      label: DESCRIBED.has(id) ? t(`app.permissions.name.${id.toLowerCase()}`) : id,
    }));
  });

  const accessKinds = $derived(
    KINDS.filter((kind) => described.some((role) => role.item_access?.[kind as 'read'] !== undefined)).map(
      (kind) => ({ id: kind, label: t(`app.permissions.kind.${kind}`) }),
    ),
  );

  const roles = $derived<MatrixRole[]>(
    described.map((role) => ({
      id: role.role ?? '',
      label: role.role ? t(`app.people.role.${role.role.toLowerCase()}`) : '',
      permissions: role.permissions ?? [],
      itemAccess: (role.item_access ?? {}) as Record<string, string>,
    })),
  );

  const accessLabels = {
    ALL: t('app.permissions.access_all'),
    ASSIGNED: t('app.permissions.access_assigned'),
    NONE: t('app.permissions.access_none'),
  };
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.permissions.title')}</h1>
    <p class="quiet">{t('app.permissions.intro')}</p>

    {#if roles.length === 0}
      <!-- The manifest has not arrived, or this installation describes no roles. Both are the same
           thing to a reader: there is nothing to show yet. -->
      <p class="quiet">{t('app.permissions.unknown')}</p>
    {:else}
      <PermissionMatrix
        label={t('app.permissions.title')}
        roleColumnLabel={t('app.people.role_column')}
        {permissions}
        {accessKinds}
        {accessLabels}
        {roles}
        carriedLabel={t('app.permissions.yes')}
        notCarriedLabel={t('app.permissions.no')}
      />
      <p class="quiet small">{t('app.permissions.assigned_note')}</p>
    {/if}
  </Stack>
</div>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }
</style>
