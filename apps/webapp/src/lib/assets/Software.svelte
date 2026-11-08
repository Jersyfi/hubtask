<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's software by product (UC-AST-13 check 3): its software CIs folded by publisher and
  // product - or by the product an alias names -, each with its versions, how many CIs and
  // installations each, narrowed by the words typed. The aliases themselves are the workspace's
  // and are kept under *Configuration items*, which an administrator reaches from here.

  import { untrack } from 'svelte';
  import { Badge, DataTable, Input, Spinner, Stack, type DataColumn } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { SoftwareProductList } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { matchingProducts, softwarePath } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hubId: string;
  }

  const { hubId }: Props = $props();

  let products = $state<SoftwareProductList['data'] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    return untrack(() =>
      engine.subscribe<SoftwareProductList>({ path: softwarePath(at) }, (state) => {
        if (state.status === 'ready') products = state.data.data;
        if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
      }),
    );
  });

  let term = $state('');
  const shown = $derived(products ? matchingProducts(products, term) : []);
  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const versionColumns = $derived<DataColumn[]>([
    { id: 'version', label: t('app.software.version'), priority: 1 },
    { id: 'cis', label: t('app.software.cis'), priority: 2 },
    { id: 'hosts', label: t('app.software.hosts'), priority: 2 },
  ]);
  type Version = (typeof shown)[number]['versions'][number];
</script>

{#if refusal}
  <FailureNote problem={refusal} />
{:else if !products}
  <Spinner label={t('app.assets.loading')} />
{:else}
  <Stack gap="150">
    <p class="meta">
      {t('app.software.hint')}
      {#if mayShape}<a href="/assets/settings">{t('app.software.aliases_link')}</a>{/if}
    </p>
    {#if products.length === 0}
      <p class="meta">{t('app.software.none')}</p>
    {:else}
      <Input label={t('app.software.search')} bind:value={term} />
      {#if shown.length === 0}
        <p class="meta" role="status">{t('app.software.no_match')}</p>
      {/if}
      <ul class="rows">
        {#each shown as product (`${product.publisher}/${product.product}`)}
          <li class="row" data-software={product.product}>
            <div class="head">
              <strong>{product.product}</strong>
              <span class="meta">{product.publisher || t('app.software.no_publisher')}</span>
              {#if product.aliased}<Badge size="sm" tone="info">{t('app.software.aliased')}</Badge>{/if}
              <span class="meta">{t('app.software.installations', { count: product.installations })}</span>
            </div>
            {#if product.versions.length > 0}
              <DataTable label={t('app.software.versions_of', { product: product.product })} columns={versionColumns} rows={product.versions}
                key={(version: Version) => version.version}>
                {#snippet cell(version: Version, column: DataColumn)}
                  {#if column.id === 'version'}{version.version || t('app.software.no_version')}
                  {:else if column.id === 'cis'}{version.cis}
                  {:else}{version.installations}{/if}
                {/snippet}
              </DataTable>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </Stack>
{/if}

<style>
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: grid; gap: var(--sp-100); padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface);
  }
  .head { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .meta a { color: var(--text-primary); }
</style>
