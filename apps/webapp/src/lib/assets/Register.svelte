<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's asset register (UC-AST-12 check 5): every CI with an asset record, its tag, state,
  // warranty, holder and cost, narrowed to one state. A record is registered and moved on the CI's
  // own screen, which each row opens.

  import { untrack } from 'svelte';
  import { Badge, Banner, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { CiAsset, CiAssetList, CiClass } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { ASSET_STATES, assetsPath, assetTone, classWord, costText, type AssetState, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
  }

  const { hubId, classes, words }: Props = $props();

  let chosenState = $state('');
  let assets = $state<readonly CiAsset[] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = assetsPath(hubId, (chosenState || undefined) as AssetState | undefined);
    return untrack(() => {
      assets = undefined;
      return engine.subscribe<CiAssetList>({ path }, (read) => {
        if (read.status === 'ready') assets = read.data.data;
        if (read.status === 'failed') refusal = renderProblem(read.error, messages).message;
      });
    });
  });

  const stateOptions = $derived([
    { value: '', label: t('app.register.every_state') },
    ...ASSET_STATES.map((each) => ({ value: each, label: t(`app.register.state.${each}`) })),
  ]);
</script>

{#if refusal}
  <Banner tone="danger">{refusal}</Banner>
{:else}
  <Stack gap="150">
    <p class="meta">{t('app.register.hint')}</p>
    <div><Select label={t('app.register.state_label')} size="sm" options={stateOptions} bind:value={chosenState} /></div>
    {#if !assets}
      <Spinner label={t('app.assets.loading')} />
    {:else if assets.length === 0}
      <p class="meta">{t('app.register.none')}</p>
    {:else}
      <div class="scroll">
        <table class="register" data-register>
          <thead>
            <tr>
              <th scope="col">{t('app.register.item')}</th>
              <th scope="col">{t('app.register.tag')}</th>
              <th scope="col">{t('app.register.state_label')}</th>
              <th scope="col">{t('app.register.warranty')}</th>
              <th scope="col">{t('app.register.holder')}</th>
              <th scope="col">{t('app.register.cost')}</th>
            </tr>
          </thead>
          <tbody>
            {#each assets as asset (asset.ci_id)}
              <tr data-asset={asset.ci_id}>
                <th scope="row">
                  <a href={`/cis/${asset.ci_id}`}>{asset.name ?? t('app.assets.unknown_ci')}</a>
                  {#if asset.class}<span class="meta">{classWord(asset.class, classes, words)}</span>{/if}
                </th>
                <td>{asset.asset_tag || '—'}</td>
                <td><Badge size="sm" tone={assetTone(asset)}>{t(`app.register.state.${asset.state}`)}</Badge></td>
                <td>
                  {asset.warranty_until ?? '—'}
                  {#if asset.warranty_ended}<Badge size="sm" tone="warning">{t('app.register.warranty_ended')}</Badge>{/if}
                </td>
                <td>{asset.assigned_to ? (accounts.nameOf(asset.assigned_to) ?? '—') : '—'}</td>
                <td>{costText(asset.cost_minor, asset.currency, messages.locale)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </Stack>
{/if}

<style>
  .scroll { overflow-x: auto; }
  .register { border-collapse: collapse; inline-size: 100%; font-size: var(--fs-075); }
  .register th, .register td { text-align: start; padding: var(--sp-050) var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); vertical-align: baseline; }
  .register th[scope='row'] { display: grid; gap: var(--sp-025); font-weight: var(--fw-medium); }
  .register a { color: var(--text-primary); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; font-weight: var(--fw-regular); }
</style>
