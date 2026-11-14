<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's software by product (UC-AST-13 check 3, scene invreg): its software objects folded by
  // publisher and product - or by the product an alias names -, one table of their versions grouped
  // by product, how many objects and installations each, narrowed by the words typed. The aliases
  // are the workspace's, kept in the administration, which a STRUCTURE holder reaches from here.

  import { untrack } from 'svelte';
  import { DataTable, Input, Spinner, Stack, StateLozenge, type DataColumn } from '@hubtask/design-system/components';
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
  // One table of every version, grouped by its product (WB-4): the group's head names the product,
  // its publisher, an alias that folded it and where it is installed.
  const groupOf = (product: (typeof shown)[number]) => `${product.publisher}/${product.product}`;
  const rows = $derived(shown.flatMap((product) =>
    (product.versions.length > 0 ? product.versions : [{ version: '', cis: 0, installations: product.installations }])
      .map((version) => ({ key: `${groupOf(product)}/${version.version}`, group: groupOf(product), ...version }))));
  const byGroup = $derived(new Map(shown.map((product) => [groupOf(product), product])));
  const columns = $derived<DataColumn[]>([
    { id: 'version', label: t('app.software.version'), priority: 1 },
    { id: 'cis', label: t('app.software.cis'), priority: 2, align: 'end' },
    { id: 'hosts', label: t('app.software.hosts'), priority: 2, align: 'end' },
  ]);
  type Row = (typeof rows)[number];
  let folded = $state<string[]>([]);
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
      <div data-software-table>
        <DataTable label={t('app.assets.section.software')} {columns} {rows} key={(row: Row) => row.key} group={(row: Row) => row.group}
          bind:foldedGroups={folded} foldGroupLabel={(group) => t('app.software.versions_of', { product: byGroup.get(group)?.product ?? group })}
          unfoldGroupLabel={(group) => t('app.software.versions_of', { product: byGroup.get(group)?.product ?? group })}>
          {#snippet groupHead(group: string)}
            {@const product = byGroup.get(group)}
            {#if product}
              <span class="head" data-software={product.product}>
                <strong>{product.product}</strong>
                <span class="meta">{product.publisher || t('app.software.no_publisher')}</span>
                {#if product.aliased}<StateLozenge label={t('app.software.aliased')} tone="info" />{/if}
                <span class="meta">{t('app.software.installations', { count: product.installations })}</span>
              </span>
            {/if}
          {/snippet}
          {#snippet cell(row: Row, column: DataColumn)}
            {#if column.id === 'version'}{row.version || t('app.software.no_version')}
            {:else if column.id === 'cis'}{row.cis}
            {:else}{row.installations}{/if}
          {/snippet}
        </DataTable>
      </div>
    {/if}
  </Stack>
{/if}

<style>
  .head { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .meta a { color: var(--text-primary); }
</style>
