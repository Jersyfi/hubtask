<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One device of the register opened beside it (work-surface §6.10, scene invreg 2, UC-AST-12
  // check 5): its number, name and holder, the device's life from requested to disposed
  // (`AssetLife`, as the object's page draws it), its stock facts - cost, purchase, warranty,
  // contract, stockroom -, and the moves its state allows as buttons. Disposing asks for the method
  // and the certificate's reference in a Dialog M before it is sent, as on the object's page.
  //
  // **Only the moves the domain allows are offered** (`assetMoves`), issuing only once a holder or a
  // stockroom is named, and only to a writer online on a screen that writes (P-05, UC-AST-01
  // check 10); a disposed device offers none.
  import { untrack } from 'svelte';
  import { Button, Dialog, IconButton, Input, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { CiAsset } from '@hubtask/sync-engine';

  import FailureNote from '../frame/FailureNote.svelte';
  import { accounts } from '../data/accounts.svelte.ts';
  import { assetMoves, ciAssetPath, cmdb, costText, DISPOSAL_METHODS, moveNeedsHolder, type AssetState, type DisposalMethod } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatMonth, formatPlainDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import AssetLife from './AssetLife.svelte';
  import { REGISTER_ACTS } from './inventoryscenes.ts';

  interface Props {
    ciId: string;
    /** Its row's words while the record is read: number, name. */
    tag?: string;
    name?: string;
    /** A writer, online, on a screen that writes. */
    mayAct: boolean;
    onclose: () => void;
    /** Inside a sheet, whose own head names the device and closes it. */
    isBare?: boolean;
  }

  const { ciId, tag, name, mayAct, onclose, isBare = false }: Props = $props();

  let asset = $state<CiAsset | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const id = ciId;
    return untrack(() => {
      asset = undefined;
      refusal = undefined;
      return engine.subscribe<CiAsset>({ path: ciAssetPath(id) }, (state) => {
        if (state.status === 'ready') asset = state.data;
        if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
      });
    });
  });

  // The contract's and the stockroom's names, read once each.
  let names = $state<Record<string, string>>({});
  $effect(() => {
    const ids = [asset?.contract_id, asset?.stockroom_id, ...(asset?.moves ?? []).map((each) => each.stockroom_id)].filter((id): id is string => Boolean(id));
    untrack(() => {
      for (const id of new Set(ids)) {
        if (id in names) continue;
        names = { ...names, [id]: '' };
        void cmdb.read(id).then((ci) => {
          if (ci) names = { ...names, [id]: ci.name };
        });
      }
    });
  });
  $effect(() => {
    const holder = asset?.assigned_to;
    if (holder) untrack(() => accounts.resolve([holder]));
  });
  const nameOf = (id: string) => names[id] || undefined;
  const holderName = $derived(asset?.assigned_to ? (accounts.nameOf(asset.assigned_to) ?? t('app.people.unnamed')) : undefined);

  const moves = $derived(asset ? REGISTER_ACTS.filter((to) => assetMoves(asset!.state as AssetState).includes(to)) : []);
  const actWord = (to: AssetState) =>
    to === 'IN_STOCK' && (asset?.state === 'DEPLOYED' || asset?.state === 'RETIRED') ? t('app.register.act.BACK_IN_STOCK') : t(`app.register.act.${to}`);

  let isMoving = $state(false);
  let isDisposeOpen = $state(false);
  let method = $state<DisposalMethod>('WIPED');
  let certificate = $state('');
  let moveFailure = $state<string | undefined>(undefined);

  async function move(to: AssetState, disposal?: { method: DisposalMethod; certificate: string }) {
    if (!asset) return;
    isMoving = true;
    try {
      await cmdb.moveAsset(asset, to, disposal);
      toast.show({ text: t('app.register.moved', { state: t(`app.register.state.${to}`) }), tone: 'success' });
      isDisposeOpen = false;
    } catch (error) {
      const said = renderProblem(error as never, messages).message;
      if (isDisposeOpen) moveFailure = said;
      else toast.show({ text: said, tone: 'danger' });
    } finally {
      isMoving = false;
    }
  }
  function act(to: AssetState) {
    if (to !== 'DISPOSED') {
      void move(to);
      return;
    }
    method = 'WIPED';
    certificate = '';
    moveFailure = undefined;
    isDisposeOpen = true;
  }
</script>

<aside class="panel" aria-labelledby={isBare ? undefined : 'register-panel-title'} data-register-panel={ciId} data-bare={isBare ? '' : undefined}>
  {#if !isBare}<div class="head">
    <h2 id="register-panel-title" class="title">
      {#if asset?.asset_tag || tag}<span class="tag">{asset?.asset_tag || tag}</span>{/if}
      <span>{[asset?.name ?? name ?? '', ...(holderName ? [holderName] : [])].join(' · ')}</span>
    </h2>
    <IconButton icon="x" label={t('app.sheet_close')} size="sm" tone="subtle" onclick={onclose} />
  </div>{/if}
  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !asset}
    <Spinner label={t('app.assets.loading')} />
  {:else}
    <AssetLife {asset} {nameOf} />
    <section class="facts" aria-labelledby="register-panel-stock">
      <h3 id="register-panel-stock" class="subheading">{t('app.register.stock')}</h3>
      <dl class="pairs">
        <dt>{t('app.register.cost')}</dt><dd>{costText(asset.cost_minor, asset.currency, messages.locale)}</dd>
        <dt>{t('app.register.purchased')}</dt><dd>{asset.purchased_on ? formatPlainDay(asset.purchased_on, messages.locale) : '—'}</dd>
        <dt>{t('app.register.warranty')}</dt><dd>{asset.warranty_until ? formatMonth(asset.warranty_until.slice(0, 7), messages.locale) : '—'}</dd>
        <dt>{t('app.register.contract')}</dt><dd>{#if asset.contract_id}<a href={`/cis/${asset.contract_id}`}>{nameOf(asset.contract_id) ?? t('app.assets.unknown_ci')}</a>{:else}—{/if}</dd>
        <dt>{t('app.register.stockroom')}</dt><dd>{#if asset.stockroom_id}<a href={`/cis/${asset.stockroom_id}`}>{nameOf(asset.stockroom_id) ?? t('app.assets.unknown_ci')}</a>{:else}—{/if}</dd>
      </dl>
      <a class="object" href={`/cis/${asset.ci_id}`}>{t('app.register.open_object')}</a>
    </section>
    {#if mayAct && moves.length > 0}
      <div class="acts" data-register-acts>
        {#each moves as to (to)}
          <Button size="sm" tone={to === 'DISPOSED' ? 'danger' : 'secondary'} isBusy={isMoving} data-register-move={to}
            disabledReason={moveNeedsHolder(asset, to) ? t('cmdb.asset_deploy_unassigned') : undefined}
            onclick={() => act(to)}>{actWord(to)}</Button>
        {/each}
      </div>
    {/if}
  {/if}
</aside>

{#if isDisposeOpen}
  <Dialog size="m" bind:isOpen={isDisposeOpen} title={t('app.register.dispose')} dismissLabel={t('app.assets.cancel')}
    primary={{ label: t('app.register.dispose'), tone: 'danger', isBusy: isMoving, disabledReason: certificate.trim() ? undefined : t('cmdb.disposal_certificate_required'), onclick: () => void move('DISPOSED', { method, certificate: certificate.trim() }) }}>
    <Stack gap="150">
      <p class="meta">{t('app.register.dispose_hint')}</p>
      <Select label={t('app.register.method_label')} options={DISPOSAL_METHODS.map((each) => ({ value: each, label: t(`app.register.method.${each}`) }))} bind:value={method} />
      <Input label={t('app.register.certificate')} hint={t('app.register.certificate_hint')} bind:value={certificate} maxlength={200} isRequired />
      {#if moveFailure}<FailureNote problem={moveFailure} />{/if}
    </Stack>
  </Dialog>
{/if}

<style>
  .panel {
    display: grid; align-content: start; gap: var(--sp-200); padding: var(--sp-200); min-inline-size: 0;
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface);
  }
  .panel[data-bare] { padding: 0; border: 0; background: none; }
  .head { display: flex; align-items: start; justify-content: space-between; gap: var(--sp-100); }
  .title { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); overflow-wrap: anywhere; }
  .tag { font-family: var(--font-mono); font-size: var(--fs-090); font-weight: var(--fw-regular); }
  .facts { display: grid; gap: var(--sp-100); padding-block-start: var(--sp-150); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .subheading { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-secondary); text-transform: uppercase; }
  .pairs { display: grid; grid-template-columns: minmax(0, max-content) minmax(0, 1fr); gap: var(--sp-100) var(--sp-200); margin: 0; }
  .pairs dt { color: var(--text-secondary); font-weight: var(--fw-medium); }
  .pairs dd { margin: 0; overflow-wrap: anywhere; }
  .pairs a, .object { color: var(--text-primary); }
  .object { font-size: var(--fs-090); justify-self: start; }
  .acts { display: flex; flex-wrap: wrap; gap: var(--sp-100); padding-block-start: var(--sp-150); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the link's press
     reaches layout.target beyond its drawn box (P-2). */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .object { position: relative; z-index: var(--z-raised); }
    .object::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
