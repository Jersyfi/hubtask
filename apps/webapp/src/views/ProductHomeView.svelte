<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's workspace-wide home (user-experience.md §2.2, §5.1): the one door the products band
  // opens. One anatomy for every product - the trail *Products › <product>*, the title with what
  // the product does, the views as `PlaceTabs` where it has several, then the hubs the reader may
  // read where the product is on, each opening the hub's own screen of it. A hub narrows the
  // workspace's modules (UC-INS-21 check 3), so each hub is asked which are on there.
  import { untrack } from 'svelte';
  import { Button, EmptyState, Icon, ListRow, PageHeader, PlaceTabs, Skeleton, Stack, type PlaceTab } from '@hubtask/design-system/components';

  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';
  import { markOf } from '../lib/workspace/look.ts';
  import { hubCreationHere, reachesAdministrationArea } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { live } from '../lib/data/live.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { modules, modulesHere } from '../lib/data/modules.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { productOf } from '../lib/navigation.ts';
  import { homeTabs, hubScreenOf, productDoors } from '../lib/product/home.ts';
  import NotesView from './NotesView.svelte';

  interface Props {
    module: string;
    /** The view of the home, where it has several: the pages' *My notes*. */
    tab?: string;
    onnavigate: (path: string) => void;
  }
  const { module, tab, onnavigate }: Props = $props();

  const row = $derived(productOf(module));
  const name = $derived(row ? t(row.code) : '');
  $effect(() => page.entitle(name));

  const tabs = $derived<PlaceTab[]>(homeTabs(module).map((each) => ({ id: each.id, label: t(each.code), href: each.path })));
  const currentTab = $derived(tab ?? tabs[0]?.id);

  /** The hubs the tree draws, and for each the modules on there, once it has answered. */
  const hubs = $derived(containers.hubs.filter((hub) => !live.hasLost(hub.id)));
  let onAt = $state<Record<string, readonly string[]>>({});
  const hubKey = $derived(hubs.map((hub) => hub.id).join(','));
  $effect(() => {
    const ids = hubKey === '' ? [] : hubKey.split(',');
    return untrack(() => {
      const stops = ids.map((hubId) =>
        modules.here(hubId, (next) => {
          if (next.status === 'ready') onAt = { ...onAt, [hubId]: next.data.modules_on };
        }),
      );
      return () => {
        for (const stop of stops) stop();
      };
    });
  });
  const isReading = $derived(
    containers.hubsState.status === 'loading' || containers.hubsState.status === 'idle' || hubs.some((hub) => onAt[hub.id] === undefined),
  );
  const hubsWith = $derived(
    hubs.filter((hub) => onAt[hub.id] !== undefined && (modulesHere(manifest.value?.modules_on, onAt[hub.id]) as readonly string[]).includes(module)),
  );

  const doors = $derived(productDoors(module, { isAdministrationReachable: reachesAdministrationArea() === true }));
  const creation = $derived(hubCreationHere());
  const mayCreateHub = $derived(creation === 'shared' || creation === 'private');
  let isCreatingHub = $state(false);
</script>

<Stack gap="300">
  <PageHeader
    title={name}
    subtitle={t(`app.modules.hint_${module.toLowerCase()}`)}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'products', label: t('app.nav.products') },
        { id: 'product', label: name },
      ],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  {#if tabs.length > 1}
    <PlaceTabs label={name} {tabs} current={currentTab} onnavigate={(id) => onnavigate(tabs.find((each) => each.id === id)?.href ?? '/')} />
  {/if}

  {#if currentTab === 'notes'}
    <NotesView {onnavigate} isEmbedded />
  {:else}
    {#if doors.length > 0}
      <ul class="doors">
        {#each doors as door (door.id)}
          <li>
            <ListRow href={door.path} onactivate={() => onnavigate(door.path)}>
              {#snippet leading()}<Icon name={door.icon} size="sm" />{/snippet}
              {t(door.code)}
            </ListRow>
          </li>
        {/each}
      </ul>
    {/if}

    {#if hubScreenOf(module, '') !== undefined}
      <section aria-labelledby="product-hubs">
        <Stack gap="100">
          <h2 class="heading" id="product-hubs">{t('app.product_home.hubs')}</h2>
          {#if isReading && hubsWith.length === 0}
            <div aria-busy="true"><Skeleton lines={3} /></div>
          {:else if hubsWith.length === 0}
            <EmptyState
              kind="unused"
              icon={row?.icon ?? 'hub'}
              title={t('app.product_home.none', { product: name })}
              description={t(mayCreateHub ? 'app.product_home.none_body_create' : 'app.product_home.none_body')}
            >
              {#snippet action()}
                {#if mayCreateHub}
                  <Button tone="primary" icon="plus" onclick={() => (isCreatingHub = true)}>{t('app.workspace.create_hub')}</Button>
                {/if}
              {/snippet}
            </EmptyState>
          {:else}
            <ul class="hubs">
              {#each hubsWith as hub (hub.id)}
                {@const path = hubScreenOf(module, hub.id) ?? `/hubs/${hub.id}`}
                <li>
                  <ListRow href={path} onactivate={() => onnavigate(path)} label={hub.name}>
                    {#snippet leading()}<Icon name={markOf(hub)} size="sm" />{/snippet}
                    <span class="name" title={hub.name}>{hub.name}</span>
                  </ListRow>
                </li>
              {/each}
            </ul>
          {/if}
        </Stack>
      </section>
    {/if}
  {/if}
</Stack>

{#if isCreatingHub && mayCreateHub}
  <CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" isPrivateOnly={creation === 'private'} oncreated={(id) => onnavigate(hubScreenOf(module, id) ?? `/hubs/${id}`)} />
{/if}

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .doors, .hubs { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  /* One line with an ellipsis and the whole name as the tooltip (user-experience.md §2.5). */
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
