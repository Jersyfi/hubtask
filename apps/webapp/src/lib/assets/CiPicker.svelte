<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A configuration item chosen while writing (UC-AST-01 check 11): a hub the reader sees, then a
  // CI of its inventory narrowed by name. What it offers is what the hub's inventory answers the
  // reader, so a CI out of their sight is never offered; the words narrow the copy already read
  // and never travel to the server.

  import { untrack } from 'svelte';
  import { Input, Select, Spinner } from '@hubtask/design-system/components';
  import type { ConfigurationItem, ConfigurationItemList } from '@hubtask/sync-engine';

  import { cisPath, matchingCis } from '../data/cmdb.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The CI chosen, or none while nothing is. */
    chosen?: ConfigurationItem;
    onchoose: (ci: ConfigurationItem | undefined) => void;
  }

  const { chosen, onchoose }: Props = $props();

  const LIMIT = 8;

  const hubs = $derived(containers.hubs);
  let hubId = $state('');
  let term = $state('');
  let held = $state<readonly ConfigurationItem[] | undefined>(undefined);

  $effect(() => {
    if (hubId === '' && hubs[0]) hubId = hubs[0].id;
  });
  $effect(() => {
    const at = hubId;
    if (at === '') return;
    return untrack(() => {
      held = undefined;
      onchoose(undefined);
      return engine.subscribe<ConfigurationItemList>({ path: cisPath(at) }, (state) => {
        if (state.status === 'ready') held = state.data.data;
        if (state.status === 'failed') held = [];
      });
    });
  });

  const hits = $derived(matchingCis(held ?? [], term, LIMIT));
</script>

<div class="picker">
  <Select
    label={t('app.mentions.ci_hub')}
    options={hubs.map((hub) => ({ value: hub.id, label: hub.name }))}
    bind:value={hubId}
  />
  <Input
    label={t('app.mentions.ci_search')}
    value={term}
    oninput={(event) => { term = (event.currentTarget as HTMLInputElement).value; onchoose(undefined); }}
  />
  {#if held === undefined}
    <p class="quiet"><Spinner label={t('app.assets.loading')} /> {t('app.assets.loading')}</p>
  {:else if hits.length === 0}
    <p class="quiet" role="status">{t('app.mentions.ci_none')}</p>
  {:else}
    <ul class="hits" aria-label={t('app.mentions.ci_search')}>
      {#each hits as hit (hit.id)}
        <li>
          <button
            type="button"
            class="hit"
            data-ci-hit={hit.id}
            aria-pressed={chosen?.id === hit.id}
            aria-label={t('app.links.choose', { name: hit.name })}
            onclick={() => onchoose(hit)}
          >
            <span class="title">{hit.name}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker { display: flex; flex-direction: column; gap: var(--sp-100); }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .hits { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .hit {
    display: flex; align-items: baseline; gap: var(--sp-100); inline-size: 100%;
    padding-block: var(--density-row-block); padding-inline: var(--sp-100);
    border: var(--bw-hairline) solid transparent; border-radius: var(--r-md);
    background: transparent; color: var(--text-primary); font: inherit; text-align: start; cursor: pointer;
  }
  .hit:hover { background: var(--bg-surface-hover); }
  .hit[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--bg-surface-pressed); }
  .hit:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .title { min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
