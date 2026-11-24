<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The shipped roles and the workspace's own (UC-ID-26 check 6) as *People*'s *Roles* list page
  // (page-system §7.1, §9.2): one row per role, opening its record page (RoleView) with what it
  // carries and who holds it; *New role* in the head opens the role's dialog.
  //
  // **Deleting a role goes through the configuration's plan** (user-experience.md §5.9), on the
  // role's own page: its reach and refusals are shown before the button, and *Undo* plans it back.

  import { untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    DataTable,
    EmptyState,
    FilterBar,
    ListPage,
    Radio,
    Spinner,
    type DataColumn,
    type Filter,
  } from '@hubtask/design-system/components';

  import PeopleStrip from '../lib/people/PeopleStrip.svelte';
  import RoleDialog from '../lib/people/RoleDialog.svelte';
  import { peopleHead } from '../lib/people/peoplehead.svelte.ts';
  import { holdersByRole } from '../lib/people/roleholders.ts';
  import { roleLabel } from '../lib/people/rolewords.ts';
  import { matchesChoice, matchesSearch } from '../lib/page/list.ts';
  import { mayHere } from '../lib/data/capability.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { grantsOf, roles } from '../lib/data/roles.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { sorting } from '../lib/sorting.svelte.ts';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';
  import SectionFindings from '../lib/settings/SectionFindings.svelte';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  const TENANT = { scopeType: 'TENANT' } as const;

  $effect(() =>
    untrack(() => {
      const closers = [roles.open(), groups.open(), people.openScope(TENANT)];
      return () => closers.forEach((close) => close?.());
    }),
  );
  $effect(() => page.entitle(t('app.people.title')));

  // Who holds each shipped role at the workspace, once every page of its memberships is read.
  const heldBy = $derived(holdersByRole(people.along({}), people.isCompleteAt(TENANT)));

  const catalogue = $derived(roles.catalogue);

  interface RoleRow {
    readonly id: string;
    readonly name: string;
    readonly isOwn: boolean;
    readonly permissions: number;
    readonly held?: string;
  }

  const rows = $derived<RoleRow[]>(
    catalogue
      ? [
          ...catalogue.shipped.map((role) => {
            const held = heldBy?.get(role.role) ?? { people: 0, groups: 0 };
            return {
              id: role.role,
              name: roleLabel(role.role),
              isOwn: false,
              permissions: catalogue.catalogue.filter((entry) => entry.shipped.includes(role.role)).length,
              held: heldBy
                ? held.groups === 0
                  ? t('app.roles.held_people', { people: held.people })
                  : t('app.roles.held_people_groups', { people: held.people, groups: held.groups })
                : undefined,
            };
          }),
          ...catalogue.custom.map((role) => ({
            id: role.key,
            name: role.name,
            isOwn: true,
            permissions: role.permissions.length,
            held: t('app.roles.held_grants', { count: String(grantsOf(catalogue, role.key).length) }),
          })),
        ]
      : [],
  );

  // The search and the kind narrow the roles loaded; `GET /roles` answers them whole.
  let search = $state('');
  let kind = $state('');
  const shown = $derived(rows.filter((row) => matchesSearch(search, [row.name]) && matchesChoice(kind || undefined, row.isOwn ? 'own' : 'shipped')));
  const chips = $derived<Filter[]>([
    { id: 'kind', label: t('app.roles.kind_chip', { kind: kind ? t(`app.roles.kind_${kind}`) : t('app.page.any') }), isActive: kind !== '' },
  ]);

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.roles.name'), isSortable: true, priority: 1 },
    { id: 'held', label: t('app.roles.held_column'), priority: 2 },
    { id: 'permissions', label: t('app.roles.permissions_column'), isSortable: true, priority: 3 },
    { id: 'kind', label: t('app.roles.kind_column'), isStatus: true, priority: 3 },
  ]);
  const order = sorting({ rows: () => shown, read: (row: RoleRow, columnId) => (columnId === 'permissions' ? row.permissions : row.name) });

  const mayDefine = $derived(mayHere('MANAGE_MEMBERS').status === 'permitted');
  let defining = $state<'new' | undefined>(undefined);
  let isFileOpen = $state(false);
  const primary = $derived(
    mayDefine && catalogue ? { label: t('app.roles.new'), icon: 'plus' as const, opener: 'new-role', onclick: () => (defining = 'new') } : undefined,
  );

  function clearFilters(): void {
    search = '';
    kind = '';
  }
</script>

<ListPage head={peopleHead(primary, viewAsFileAction(() => (isFileOpen = true)))}>
  {#snippet strip()}<PeopleStrip current="roles" {onnavigate} />{/snippet}
  {#snippet intro()}<p class="quiet">{t('app.roles.intro')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.roles.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={chips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(kind ? 1 : 0) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(shown.length) })}
      clearLabel={t('app.page.clear')}
      onclear={clearFilters}
      countLabel={catalogue ? t('app.page.found', { shown: String(shown.length), count: String(rows.length) }) : undefined}
    >
      {#snippet picker(_filter: Filter)}
        <Radio
          label={t('app.roles.kind_column')}
          bind:value={kind}
          options={[{ value: '', label: t('app.page.any') }, { value: 'shipped', label: t('app.roles.kind_shipped') }, { value: 'own', label: t('app.roles.kind_own') }]}
        />
      {/snippet}
    </FilterBar>
  {/snippet}

  <SectionFindings sections={['roles']} />
  <WorkspaceGate permission="MANAGE_MEMBERS" />

  {#if !catalogue && roles.state?.status === 'failed'}
    <Banner tone="warning">{renderProblem(roles.state.error as never, messages).message}</Banner>
  {:else}
    <DataTable
      label={t('app.people.tab_roles')}
      {columns}
      rows={order.rows}
      key={(row: RoleRow) => row.id}
      href={(row: RoleRow) => `/administration/roles/${encodeURIComponent(row.id)}`}
      rowLabel={(row: RoleRow) => row.name}
      sort={order.sort}
      onSort={(next) => order.by(next)}
      shownLabel={catalogue ? t('app.page.showing', { count: String(shown.length) }) : undefined}
    >
      {#snippet cell(row: RoleRow, column: DataColumn)}
        {#if column.id === 'name'}
          <span class="name" data-shipped-role={row.isOwn ? undefined : row.id}>{row.name}</span>
        {:else if column.id === 'held'}
          {#if row.held}<span class="quiet" data-held>{row.held}</span>{/if}
        {:else if column.id === 'permissions'}
          {t('app.roles.permission_count', { count: String(row.permissions) })}
        {:else}
          <Badge tone={row.isOwn ? 'info' : 'neutral'}>{t(row.isOwn ? 'app.roles.kind_own' : 'app.roles.kind_shipped')}</Badge>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if !catalogue}
          <p class="quiet waiting"><Spinner label={t('app.roles.reading')} /> <span>{t('app.roles.reading')}</span></p>
        {:else}
          <EmptyState kind="filtered" title={t('app.roles.filtered_title')}>
            {#snippet action()}<Button onclick={clearFilters}>{t('app.people.filtered_clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

<ViewSectionsAsFile sections={['roles']} bind:isOpen={isFileOpen} isTriggerHidden />

{#if mayDefine}
  <RoleDialog role={defining} onClose={() => (defining = undefined)} />
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  .waiting { padding: var(--sp-200); }
</style>
