<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Who may do what* (A11) and *view as* (A6) as *People*'s fourth list page (page-system §9.2):
  // the person, the place and the permission are chips in the filter bar, the matrix is the list.
  // *Who may change which setting* is the fifth tab, the same page in its other `view`: its table,
  // narrowed by the search and the place and continued at its foot (UC-ID-26 checks 7 and 8).
  //
  // **The matrix is the server's catalogue against the roles** - the shipped seven and the
  // workspace's own - from `GET /roles`; nothing is compiled in. For a chosen person and place it
  // narrows to the roles that count for them there, and `GET /access/explain` says where each is
  // held, what they may do, and - for a chosen permission - whether and through which role. *View
  // as* on a person in the people tab opens this with that person chosen.
  //
  // **Nothing here is editable.** What a role carries is changed on the roles tab; this says what
  // it means.

  import { untrack } from 'svelte';

  import {
    Banner,
    DataTable,
    FilterBar,
    ListPage,
    PermissionMatrix,
    Select,
    Spinner,
    Stack,
    type DataColumn,
    type Filter,
  } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import type { AccessExplanation, AccessSource, ResourceState, SettingsCatalogue } from '@hubtask/sync-engine';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { matrixOf, roles, settingWritersOf, type Place } from '../lib/data/roles.svelte.ts';
  import { settingsCatalogue } from '../lib/data/settingsCatalogue.svelte.ts';
  import { labelCode } from '../lib/settings/catalogue.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { permissionLabel, roleLabel } from '../lib/people/rolewords.ts';
  import { renderProblem } from '../lib/problem.ts';
  import PeopleStrip from '../lib/people/PeopleStrip.svelte';
  import { peopleHead } from '../lib/people/peoplehead.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';

  interface Props {
    /** The person *view as* opened this for, from the address (`?person=`). */
    person?: string;
    onnavigate?: (path: string) => void;
    /** The matrix, or who may change which setting. */
    view?: 'matrix' | 'settings';
  }

  const { person, onnavigate, view = 'matrix' }: Props = $props();

  const WORKSPACE_PLACE = '';

  $effect(() =>
    untrack(() => {
      const closers = [roles.open(), people.openScope({ scopeType: 'TENANT' }), containers.start(), groups.open()];
      return () => closers.forEach((close) => close());
    }),
  );

  // svelte-ignore state_referenced_locally
  let who = $state(person ?? '');
  let hub = $state(WORKSPACE_PLACE);
  let collection = $state(WORKSPACE_PLACE);
  let asked = $state('');

  $effect(() => {
    if (person) who = person;
  });

  $effect(() => {
    if (hub) {
      const chosen = hub;
      return untrack(() => containers.openLevel(chosen));
    }
    return undefined;
  });

  const place = $derived<Place>(
    collection ? { scopeType: 'COLLECTION', scopeId: collection } : hub ? { scopeType: 'HUB', scopeId: hub } : { scopeType: 'TENANT' },
  );

  let explained = $state<AccessExplanation | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let isAsking = $state(false);

  $effect(() => {
    const account = who;
    const where = place;
    const permission = asked;
    if (!account) {
      explained = undefined;
      return;
    }
    let isCurrent = true;
    isAsking = true;
    failure = undefined;
    untrack(() =>
      roles
        .explain(where, account, permission || undefined)
        .then((answer) => {
          if (isCurrent) explained = answer;
        })
        .catch((error: unknown) => {
          if (!isCurrent) return;
          explained = undefined;
          failure = renderProblem(error as never, messages).message;
        })
        .finally(() => {
          if (isCurrent) isAsking = false;
        }),
    );
    return () => {
      isCurrent = false;
    };
  });

  const catalogue = $derived(roles.catalogue);
  const custom = $derived(catalogue?.custom ?? []);
  const matrix = $derived(catalogue ? matrixOf(catalogue, explained?.roles) : undefined);

  const columns = $derived((matrix?.columns ?? []).map((id) => ({ id, label: permissionLabel(id) })));
  const allRows = $derived(
    (matrix?.rows ?? []).map((row) => ({ id: row.id, label: row.isCustom ? (row.name ?? row.id) : roleLabel(row.id), permissions: row.permissions })),
  );

  // Who may change which setting (A11): the settings catalogue's writers against the same roles,
  // narrowed with the matrix to the roles that count for a chosen person.
  let settings = $state<ResourceState<SettingsCatalogue> | undefined>(undefined);
  $effect(() => untrack(() => settingsCatalogue.read((next) => (settings = next))));
  const settingRows = $derived(
    catalogue && settings?.status === 'ready' ? settingWritersOf(settings.data.settings, catalogue, explained?.roles) : [],
  );

  const personOptions = $derived(
    people.candidates({}).map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );
  const hubOptions = $derived([
    { value: WORKSPACE_PLACE, label: t('app.people.managed_where_workspace') },
    ...containers.hubs.map((each) => ({ value: each.id, label: each.name })),
  ]);
  const collectionOptions = $derived([
    { value: WORKSPACE_PLACE, label: t('app.access.whole_hub') },
    ...(hub ? containers.collectionsOf(hub) : []).map((each) => ({ value: each.id, label: each.name })),
  ]);
  const permissionOptions = $derived([
    { value: '', label: t('app.access.any_permission') },
    ...(matrix?.columns ?? []).filter((id) => !id.endsWith(':*')).map((id) => ({ value: id, label: permissionLabel(id) })),
  ]);

  function sourceText(source: AccessSource): string {
    const role = roleLabel(source.role, custom);
    if (source.scope_type === 'TENANT' || !source.scope_id) return t('app.access.held_at_workspace', { role });
    if (source.scope_type === 'ITEM') return t('app.access.held_on_entry', { role });
    const name = containers.find(source.scope_id)?.name ?? t('app.roles.place_unknown');
    return source.scope_type === 'HUB' ? t('app.access.held_at_hub', { role, place: name }) : t('app.access.held_at_collection', { role, place: name });
  }

  const whoName = $derived(who ? (accounts.nameOf(who) ?? t('app.people.unnamed')) : '');

  $effect(() => page.entitle(t('app.people.title')));

  // The search and the place narrow the settings' table, which the catalogue answers whole: a
  // chosen hub keeps the settings changed at a hub, a chosen collection those of a collection.
  let search = $state('');
  const placeLevel = $derived(collection ? 'COLLECTION' : hub ? 'HUB' : undefined);
  const shownSettings = $derived(
    settingRows.filter((row) => (!placeLevel || row.level === placeLevel) && matchesSearch(search, [t(labelCode(row.key)), t(`app.access.level_${row.level}`), ...row.roles.map((role) => roleLabel(role, custom))])),
  );
  // 87 settings in the seeded workspace: the first rows, continued at the table's foot; any
  // narrowing starts again at the first.
  const continued = new Continued(() => shownSettings);
  $effect(() => {
    void search;
    void who;
    void place;
    untrack(() => continued.restart());
  });
  const optionLabel = (options: readonly { value: string; label: string }[], value: string) => options.find((option) => option.value === value)?.label ?? '';
  const chips = $derived<Filter[]>([
    { id: 'person', label: t('app.access.person_chip', { name: who ? whoName : t('app.page.any') }), isActive: who !== '' },
    { id: 'hub', label: t('app.access.place_chip', { name: optionLabel(hubOptions, hub) }), isActive: hub !== WORKSPACE_PLACE },
    ...(hub ? [{ id: 'collection', label: t('app.access.collection_chip', { name: optionLabel(collectionOptions, collection) }), isActive: collection !== WORKSPACE_PLACE }] : []),
    { id: 'permission', label: t('app.access.permission_chip', { name: optionLabel(permissionOptions, asked) }), isActive: asked !== '' },
  ]);
  const viewChips = $derived(view === 'settings' ? chips.filter((chip) => chip.id !== 'permission') : chips);
  const activeFilters = $derived(viewChips.filter((chip) => chip.isActive).length);
  // On the matrix the search narrows the roles, the rows it has.
  const rows = $derived(allRows.filter((row) => matchesSearch(search, [row.label])));

  function clearFilters(): void {
    who = '';
    hub = WORKSPACE_PLACE;
    collection = WORKSPACE_PLACE;
    asked = '';
    search = '';
  }

  interface SettingRow {
    readonly key: string;
    readonly level: string;
    readonly roles: readonly string[];
  }
  const settingColumns = $derived<DataColumn[]>([
    { id: 'setting', label: t('app.access.setting_column'), priority: 1 },
    { id: 'roles', label: t('app.access.roles_column'), priority: 2 },
    { id: 'level', label: t('app.access.level_column'), priority: 3 },
  ]);
</script>

<ListPage head={peopleHead()}>
  {#snippet strip()}<PeopleStrip current={view === 'settings' ? 'settings' : 'permissions'} {onnavigate} />{/snippet}
  {#snippet intro()}<p class="quiet">{t(view === 'settings' ? 'app.access.settings_intro' : 'app.access.intro')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t(view === 'settings' ? 'app.access.search' : 'app.access.search_roles')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={viewChips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(activeFilters) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(view === 'settings' ? shownSettings.length : rows.length) })}
      clearLabel={t('app.page.clear')}
      onclear={clearFilters}
      countLabel={view === 'settings'
        ? settingRows.length > 0 ? t('app.access.settings_found', { shown: String(shownSettings.length), count: String(settingRows.length) }) : undefined
        : catalogue ? t('app.page.found', { shown: String(rows.length), count: String(allRows.length) }) : undefined}
    >
      {#snippet picker(filter: Filter)}
        {#if filter.id === 'person'}
          <Select label={t('app.access.person')} bind:value={who} placeholder={t('app.people.choose_person')} options={personOptions} />
        {:else if filter.id === 'hub'}
          <Select label={t('app.access.hub')} bind:value={hub} options={hubOptions} onchange={() => (collection = WORKSPACE_PLACE)} />
        {:else if filter.id === 'collection'}
          <Select label={t('app.access.collection')} bind:value={collection} options={collectionOptions} />
        {:else}
          <Select label={t('app.access.permission')} bind:value={asked} options={permissionOptions} />
        {/if}
      {/snippet}
    </FilterBar>
  {/snippet}


  {#if failure}<FailureNote problem={failure} />{/if}

  {#if who}
    <section class="answer" aria-live="polite">
      <Stack gap="100">
        <Banner tone="info">{t('app.access.viewing_as', { name: whoName })}</Banner>
        {#if isAsking && !explained}
          <p class="waiting"><Spinner label={t('app.access.asking')} /> <span>{t('app.access.asking')}</span></p>
        {:else if explained}
          {#if explained.asked}
            <p class="verdict">
              {#if explained.holds}
                {t('app.access.holds_yes', { name: whoName, permission: permissionLabel(explained.asked) })}
                {(explained.held_through ?? []).map(sourceText).join('; ')}
              {:else}
                {t('app.access.holds_no', { name: whoName, permission: permissionLabel(explained.asked) })}
              {/if}
            </p>
          {/if}
          {#if explained.sources.length === 0}
            <p class="quiet">{t('app.access.no_roles', { name: whoName })}</p>
          {:else}
            <h3 class="sub">{t('app.access.roles_here')}</h3>
            <ul class="list">
              {#each explained.sources as source, index (index)}
                <li>{sourceText(source)}</li>
              {/each}
            </ul>
            <h3 class="sub">{t('app.access.may_here')}</h3>
            <p class="quiet small">{explained.permissions.map(permissionLabel).join(' · ') || t('app.roles.carries_nothing')}</p>
          {/if}
          {#if explained.requester}
            <p class="quiet small">{t('app.access.requester')}</p>
          {/if}
        {/if}
      </Stack>
    </section>
  {/if}

  {#if view === 'settings'}
    {#if settingRows.length > 0}
      <DataTable
        label={t('app.access.settings_title')}
        columns={settingColumns}
        rows={continued.shown}
        key={(row: SettingRow) => `${row.key}:${row.level}`}
        shownLabel={continued.shownLabel}
        more={continued.more}
        perPage={continued.perPage}
      >
        {#snippet cell(row: SettingRow, column: DataColumn)}
          {#if column.id === 'setting'}
            {t(labelCode(row.key))}
          {:else if column.id === 'level'}
            {t(`app.access.level_${row.level}`)}
          {:else}
            {row.roles.length > 0 ? row.roles.map((role) => roleLabel(role, custom)).join(', ') : t('app.access.no_role_carries')}
          {/if}
        {/snippet}
        {#snippet empty()}<p class="quiet waiting">{t('app.access.settings_filtered')}</p>{/snippet}
      </DataTable>
    {:else}
      <p class="waiting"><Spinner label={t('app.roles.reading')} /> <span>{t('app.roles.reading')}</span></p>
    {/if}
  {:else if !catalogue}
    {#if roles.state?.status === 'failed'}
      <Banner tone="warning">{renderProblem(roles.state.error as never, messages).message}</Banner>
    {:else}
      <p class="waiting"><Spinner label={t('app.roles.reading')} /> <span>{t('app.roles.reading')}</span></p>
    {/if}
  {:else}
    <PermissionMatrix
      label={t('app.access.matrix')}
      roleColumnLabel={t('app.people.role_column')}
      permissions={columns}
      roles={rows}
      carriedLabel={t('app.permissions.yes')}
      notCarriedLabel={t('app.permissions.no')}
    />
    <p class="quiet small">{t('app.access.families_note')}</p>
  {/if}

</ListPage>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .sub { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }

  .verdict { margin: 0; font-weight: var(--fw-semibold); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .list { margin: 0; padding-inline-start: var(--sp-200); display: grid; gap: var(--sp-050); }

  .answer {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
</style>
