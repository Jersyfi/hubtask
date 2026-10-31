<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Who may do what* (A11) and *view as* (A6), drawn as *People*'s *Permissions* tab, which
  // carries the head, the trail and the title (UC-ID-26 checks 7 and 8).
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

  import { Banner, PermissionMatrix, Select, Spinner, Stack } from '@hubtask/design-system/components';
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

  interface Props {
    /** The person *view as* opened this for, from the address (`?person=`). */
    person?: string;
  }

  const { person }: Props = $props();

  const WORKSPACE_PLACE = '';

  $effect(() =>
    untrack(() => {
      const closers = [roles.open(), people.openScope({ scopeType: 'TENANT' }), containers.start()];
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
  const rows = $derived(
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
</script>

<Stack gap="300">
  <p class="quiet">{t('app.access.intro')}</p>

  <div class="filters">
    <Select label={t('app.access.person')} bind:value={who} placeholder={t('app.people.choose_person')} options={personOptions} />
    <Select label={t('app.access.hub')} bind:value={hub} options={hubOptions} onchange={() => (collection = WORKSPACE_PLACE)} />
    {#if hub}
      <Select label={t('app.access.collection')} bind:value={collection} options={collectionOptions} />
    {/if}
    <Select label={t('app.access.permission')} bind:value={asked} options={permissionOptions} />
  </div>

  {#if failure}<Banner tone="danger">{failure}</Banner>{/if}

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

  {#if !catalogue}
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

  {#if settingRows.length > 0}
    <section class="writers" aria-labelledby="setting-writers">
      <Stack gap="100">
        <h3 class="sub" id="setting-writers">{t('app.access.settings_title')}</h3>
        <p class="quiet small">{t('app.access.settings_intro')}</p>
        <div class="scroll">
          <table data-testid="setting-writers">
            <thead>
              <tr>
                <th scope="col">{t('app.access.setting_column')}</th>
                <th scope="col">{t('app.access.level_column')}</th>
                <th scope="col">{t('app.access.roles_column')}</th>
              </tr>
            </thead>
            <tbody>
              {#each settingRows as row (`${row.key}:${row.level}`)}
                <tr>
                  <th scope="row">{t(labelCode(row.key))} <span class="mono quiet small">{row.key}</span></th>
                  <td>{t(`app.access.level_${row.level}`)}</td>
                  <td>{row.roles.length > 0 ? row.roles.map((role) => roleLabel(role, custom)).join(', ') : t('app.access.no_role_carries')}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </Stack>
    </section>
  {/if}
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .sub { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }

  .verdict { margin: 0; font-weight: var(--fw-semibold); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .filters { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-150); }

  .list { margin: 0; padding-inline-start: var(--sp-200); display: grid; gap: var(--sp-050); }

  .mono { font-family: var(--font-mono); }

  .scroll { overflow-x: auto; }

  table { width: 100%; border-collapse: collapse; font-size: var(--fs-075); }

  th, td {
    padding: var(--sp-050) var(--sp-100);
    border-bottom: var(--bw-hairline) solid var(--border-subtle);
    text-align: start;
    vertical-align: top;
  }

  thead th { font-weight: var(--fw-semibold); color: var(--text-secondary); }

  tbody th { font-weight: var(--fw-regular); }

  .answer {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
</style>
