<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Devices and licences* (work-surface §6.10, scene invreg 1, UC-AST-12 check 5): every object
  // with a register record as a calm table - number, device, with whom, state, cost, warranty, the
  // warranty in the warning tone once it ended or ends within six months -, narrowed to one state.
  // A row opens the device beside the table (`RegisterPanel`) and on a phone as a sheet; its address
  // names it (`?asset=`), so the panel survives a reload and the back button closes it.

  import { untrack } from 'svelte';
  import { DataTable, Select, Sheet, Spinner, StateLozenge, type DataColumn } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { CiAsset, CiAssetList, CiClass } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { ASSET_STATES, assetsPath, assetTone, classWord, costText, inventoryPath, type AssetState, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { renderProblem } from '../problem.ts';
  import { warrantyTone } from './inventoryscenes.ts';
  import RegisterPanel from './RegisterPanel.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    /** The device the address names: opened beside the table. */
    selected?: string;
    /** A writer, online: the panel's moves. */
    mayAct: boolean;
    onnavigate: (path: string) => void;
  }

  const { hubId, classes, words, selected, mayAct, onnavigate }: Props = $props();

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
  $effect(() => {
    const holders = (assets ?? []).map((each) => each.assigned_to).filter((id): id is string => Boolean(id));
    if (holders.length > 0) untrack(() => accounts.resolve(holders));
  });

  const today = $derived(todayIn(actor.zone));
  const stateOptions = $derived([
    { value: '', label: t('app.register.every_state') },
    ...ASSET_STATES.map((each) => ({ value: each, label: t(`app.register.state.${each}`) })),
  ]);
  const columns = $derived<DataColumn[]>([
    { id: 'tag', label: t('app.register.col.number'), priority: 2 },
    { id: 'device', label: t('app.register.col.device'), priority: 1 },
    { id: 'holder', label: t('app.register.col.holder'), priority: 2 },
    { id: 'state', label: t('app.register.col.state'), priority: 1, isStatus: true },
    { id: 'cost', label: t('app.register.col.cost'), priority: 2, align: 'end' },
    { id: 'warranty', label: t('app.register.col.warranty'), priority: 2 },
  ]);
  type Row = NonNullable<typeof assets>[number];
  const rowHref = (asset: Row) => `${inventoryPath(hubId, 'register')}?asset=${asset.ci_id}`;
  const close = () => onnavigate(inventoryPath(hubId, 'register'));
  const open = $derived(selected ? assets?.find((each) => each.ci_id === selected) : undefined);
  /** A warranty's month as a register shows it: *3/2028*. */
  const month = (day: string) => new Intl.DateTimeFormat(messages.locale, { month: 'numeric', year: 'numeric', timeZone: 'UTC' }).format(Date.parse(`${day}T12:00:00Z`));
</script>

{#if refusal}
  <FailureNote problem={refusal} />
{:else}
  <div class="register" data-register-scene data-open={selected && !viewport.isCompact ? '' : undefined}>
    <div class="list">
      <div class="tools"><Select label={t('app.register.state_label')} isLabelHidden size="sm" options={stateOptions} bind:value={chosenState} /></div>
      {#if !assets}
        <Spinner label={t('app.assets.loading')} />
      {:else if assets.length === 0}
        <p class="meta">{t('app.register.none')}</p>
      {:else}
        <div data-register>
          <DataTable label={t('app.assets.section.register')} kind="work" {columns} rows={assets} key={(asset: Row) => asset.ci_id} href={rowHref}
            rowLabel={(asset: Row) => asset.asset_tag || asset.name || ''} titleText={(asset: Row) => asset.name ?? ''}>
            {#snippet cell(asset: Row, column: DataColumn)}
              {#if column.id === 'tag'}<span class="tag" data-register-row={asset.ci_id} data-selected={asset.ci_id === selected ? '' : undefined}>{asset.asset_tag || '—'}</span>
              {:else if column.id === 'device'}
                <span class="device" data-asset={asset.ci_id}>{asset.class ? `${classWord(asset.class, classes, words)} · ` : ''}{asset.name ?? t('app.assets.unknown_ci')}</span>
              {:else if column.id === 'holder'}{asset.assigned_to ? (accounts.nameOf(asset.assigned_to) ?? '—') : '—'}
              {:else if column.id === 'state'}<StateLozenge label={t(`app.register.state.${asset.state}`)} tone={assetTone(asset)} />
              {:else if column.id === 'cost'}<span class="cost">{costText(asset.cost_minor, asset.currency, messages.locale)}</span>
              {:else if asset.warranty_until}
                {@const tone = warrantyTone(asset, today)}
                <span class="warranty" data-warranty={tone} title={tone === 'warning' ? (asset.warranty_ended ? t('app.register.warranty_ended') : t('app.register.warranty_soon')) : undefined}>
                  {#if tone === 'warning'}<StateLozenge label={month(asset.warranty_until)} tone="warning" />{:else}{month(asset.warranty_until)}{/if}
                </span>
              {:else}—{/if}
            {/snippet}
          </DataTable>
        </div>
      {/if}
    </div>
    {#if selected && !viewport.isCompact}
      {#key selected}
        <RegisterPanel ciId={selected} tag={open?.asset_tag} name={open?.name} {mayAct} onclose={close} />
      {/key}
    {/if}
  </div>
  {#if selected && viewport.isCompact}
    <Sheet title={open ? [open.asset_tag, open.name, ...(open.assigned_to ? [accounts.nameOf(open.assigned_to) ?? ''] : [])].filter(Boolean).join(' · ') : t('app.assets.section.register')} closeLabel={t('app.sheet_close')} isOpen onClose={close}>
      {#key selected}
        <RegisterPanel ciId={selected} tag={open?.asset_tag} name={open?.name} mayAct={false} onclose={close} isBare />
      {/key}
    </Sheet>
  {/if}
{/if}

<style>
  .register { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); align-items: start; }
  /* design-system-lint-ignore: `primitive.breakpoint.large` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) { .register[data-open] { grid-template-columns: minmax(0, 1fr) var(--layout-rail); } }
  .list { display: grid; gap: var(--sp-100); min-inline-size: 0; }
  .tools { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .tag { font-family: var(--font-mono); white-space: nowrap; }
  .tag[data-selected] { font-weight: var(--fw-semibold); }
  .cost { font-variant-numeric: tabular-nums; white-space: nowrap; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
