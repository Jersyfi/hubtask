<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A configuration item's asset record on its screen (UC-AST-12 check 5): its tag, cost, purchase,
  // supplier, warranty, contract, stockroom, holder and the request it came from, the state it is
  // in, and the disposal with its certificate once it is disposed of. A writer registers the record,
  // changes it and moves it on.
  //
  // **Only the moves the domain allows are offered** (`assetMoves`), and deploying is offered only
  // once a holder or a stockroom is named; disposing asks for the method and the certificate's
  // reference before it is sent. A disposed asset is kept and offers no move.

  import { untrack } from 'svelte';
  import { Badge, Button, Dialog, Input, Select, Spinner, Stack, type SelectOption } from '@hubtask/design-system/components';
  import type { CiAsset, CiClass, ConfigurationItem, WorkItem } from '@hubtask/sync-engine';

  import EntryPicker from '../entries/EntryPicker.svelte';
  import { accounts } from '../data/accounts.svelte.ts';
  import {
    assetDraft, assetMissing, assetMoves, assetRegister, assetTone, ciAssetPath, cmdb, costText, DISPOSAL_METHODS, moveNeedsHolder,
    type AssetDraft, type AssetState, type DisposalMethod,
  } from '../data/cmdb.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    ci: ConfigurationItem;
    classes: readonly CiClass[];
    /** The hub's CIs the contract and the stockroom are picked from and named by. */
    hubCis: readonly ConfigurationItem[];
    mayWrite: boolean;
  }

  const { ci, classes, hubCis, mayWrite }: Props = $props();

  // Keyed on the CI by its caller.
  const read = resource<CiAsset>(untrack(() => ciAssetPath(ci.id)));
  const asset = $derived(read.state.status === 'ready' ? read.state.data : undefined);
  const none = $derived(read.state.status === 'failed' && read.state.error.status === 404);
  const failure = $derived(read.state.status === 'failed' && !none ? renderProblem(read.state.error, messages).message : undefined);

  const of = (kind: string) =>
    hubCis.filter((each) => !each.merged_into && (classes.find((one) => one.key === each.class)?.lineage.includes(kind) ?? false));
  const options = (list: readonly ConfigurationItem[], empty: string): SelectOption[] => [{ value: '', label: empty }, ...list.map((each) => ({ value: each.id, label: each.name }))];
  const contractOptions = $derived(options(of('contract'), t('app.register.none_chosen')));
  const stockroomOptions = $derived(options(of('location'), t('app.register.none_chosen')));
  const holderOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.assets.nobody') },
    ...people.candidates({ hubId: ci.hub_id }).map((id) => ({ value: id, label: accounts.nameOf(id) ?? id })),
  ]);
  const nameOf = (id: string | null) => (id ? (hubCis.find((each) => each.id === id)?.name ?? t('app.assets.unknown_ci')) : undefined);
  const stateWord = (state: AssetState) => t(`app.register.state.${state}`);

  // Registering or changing it.
  let isOpen = $state(false);
  let draft = $state<AssetDraft>(assetDraft());
  let request = $state<WorkItem | undefined>(undefined);
  let isSaving = $state(false);
  const missing = $derived(assetMissing(draft));

  function openEdit() {
    draft = assetDraft(asset);
    request = undefined;
    isOpen = true;
  }

  async function save() {
    isSaving = true;
    try {
      const body = assetRegister({ ...draft, request: request?.id ?? draft.request }, !asset);
      await cmdb.registerAsset(ci, body, asset?.version);
      toast.show({ text: t('app.register.saved'), tone: 'success' });
      isOpen = false;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isSaving = false;
    }
  }

  // Moving it on.
  let moveTo = $state('');
  const moveOptions = $derived<SelectOption[]>(
    asset ? assetMoves(asset.state as AssetState).map((each) => ({ value: each, label: stateWord(each) })) : [],
  );
  let isDisposeOpen = $state(false);
  let method = $state<DisposalMethod>('WIPED');
  let certificate = $state('');
  let isMoving = $state(false);

  async function move(to: AssetState, disposal?: { method: DisposalMethod; certificate: string }) {
    if (!asset) return;
    isMoving = true;
    try {
      await cmdb.moveAsset(asset, to, disposal);
      toast.show({ text: t('app.register.moved', { state: stateWord(to) }), tone: 'success' });
      moveTo = '';
      isDisposeOpen = false;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isMoving = false;
    }
  }

  function startMove() {
    if (!moveTo) return;
    if (moveTo === 'DISPOSED') {
      method = 'WIPED';
      certificate = '';
      isDisposeOpen = true;
      return;
    }
    void move(moveTo as AssetState);
  }
</script>

<section class="part" aria-labelledby="ci-asset" data-ci-asset>
  <div class="part-head">
    <h2 id="ci-asset" class="heading">{t('app.register.asset')}</h2>
    {#if mayWrite && !ci.merged_into && (none || (asset && asset.state !== 'DISPOSED'))}
      <Button size="sm" tone="secondary" icon={asset ? 'pencil' : 'plus'} data-register-asset onclick={openEdit}>
        {asset ? t('app.assets.edit') : t('app.register.register')}
      </Button>
    {/if}
  </div>
  {#if failure}
    <p class="refusal" role="alert">{failure}</p>
  {:else if none}
    <p class="meta">{t('app.register.no_record')}</p>
  {:else if !asset}
    <Spinner label={t('app.assets.loading')} />
  {:else}
    <div><Badge tone={assetTone(asset)}>{stateWord(asset.state as AssetState)}</Badge></div>
    <dl class="pairs">
      <dt>{t('app.register.tag')}</dt><dd>{asset.asset_tag || '—'}</dd>
      <dt>{t('app.register.cost')}</dt><dd>{costText(asset.cost_minor, asset.currency, messages.locale)}</dd>
      <dt>{t('app.register.purchased')}</dt><dd>{asset.purchased_on ?? '—'}</dd>
      <dt>{t('app.register.supplier')}</dt><dd>{asset.supplier || '—'}</dd>
      <dt>{t('app.register.warranty')}</dt>
      <dd>{asset.warranty_until ?? '—'}{#if asset.warranty_ended}<Badge size="sm" tone="warning">{t('app.register.warranty_ended')}</Badge>{/if}</dd>
      <dt>{t('app.register.contract')}</dt><dd>{#if asset.contract_id}<a href={`/cis/${asset.contract_id}`}>{nameOf(asset.contract_id)}</a>{:else}—{/if}</dd>
      <dt>{t('app.register.stockroom')}</dt><dd>{#if asset.stockroom_id}<a href={`/cis/${asset.stockroom_id}`}>{nameOf(asset.stockroom_id)}</a>{:else}—{/if}</dd>
      <dt>{t('app.register.holder')}</dt><dd>{asset.assigned_to ? (accounts.nameOf(asset.assigned_to) ?? '—') : '—'}</dd>
      <dt>{t('app.register.request')}</dt><dd>{#if asset.request_id}<a href={`/items/${asset.request_id}`}>{t('app.register.request_open')}</a>{:else}—{/if}</dd>
      {#if asset.disposal}
        <dt>{t('app.register.disposal')}</dt>
        <dd data-disposal>
          {t(`app.register.method.${asset.disposal.method}`)} · {t('app.register.certificate_value', { reference: asset.disposal.certificate })}
          · {formatDateTime(asset.disposal.at, messages.locale)}{#if asset.disposal.by} · {accounts.nameOf(asset.disposal.by) ?? '—'}{/if}
        </dd>
      {/if}
    </dl>
    {#if mayWrite && moveOptions.length > 0}
      <div class="move">
        <Select label={t('app.register.move_to')} size="sm" options={moveOptions} placeholder={t('app.register.move_pick')} bind:value={moveTo} />
        <Button
          size="sm"
          tone="primary"
          data-move-asset
          isBusy={isMoving}
          disabledReason={!moveTo ? t('app.register.move_pick') : moveNeedsHolder(asset, moveTo as AssetState) ? t('cmdb.asset_deploy_unassigned') : undefined}
          onclick={startMove}
        >{t('app.register.move')}</Button>
      </div>
    {/if}
  {/if}
</section>

{#if isOpen}
  <Dialog bind:isOpen title={asset ? t('app.register.change') : t('app.register.register')} dismissLabel={t('app.assets.cancel')} isWide>
    <Stack gap="150">
      {#if !asset}
        <Select label={t('app.register.state_label')} options={(['REQUESTED', 'ORDERED', 'IN_STOCK'] as const).map((each) => ({ value: each, label: stateWord(each) }))} bind:value={draft.state} />
      {/if}
      <Input label={t('app.register.tag')} bind:value={draft.tag} maxlength={100} />
      <div class="pair">
        <Input label={t('app.register.cost')} inputmode="decimal" bind:value={draft.cost} />
        <Input label={t('app.register.currency')} hint={t('app.register.currency_hint')} bind:value={draft.currency} maxlength={3} />
      </div>
      <div class="pair">
        <Input label={t('app.register.purchased')} type="date" bind:value={draft.purchased} />
        <Input label={t('app.register.warranty')} type="date" bind:value={draft.warranty} />
      </div>
      <Input label={t('app.register.supplier')} bind:value={draft.supplier} maxlength={200} />
      <Select label={t('app.register.contract')} options={contractOptions} bind:value={draft.contract} />
      <Select label={t('app.register.stockroom')} options={stockroomOptions} bind:value={draft.stockroom} />
      <Select label={t('app.register.holder')} options={holderOptions} bind:value={draft.holder} />
      <fieldset class="request">
        <legend>{t('app.register.request')}</legend>
        {#if draft.request && !request}
          <div class="move">
            <a href={`/items/${draft.request}`}>{t('app.register.request_open')}</a>
            <Button size="sm" tone="subtle" onclick={() => (draft.request = '')}>{t('app.register.request_clear')}</Button>
          </div>
        {/if}
        <EntryPicker chosen={request} onchoose={(found) => (request = found)} />
      </fieldset>
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" isBusy={isSaving} disabledReason={missing[0] ? t(missing[0]) : undefined} onclick={() => void save()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isDisposeOpen}
  <Dialog bind:isOpen={isDisposeOpen} title={t('app.register.dispose')} dismissLabel={t('app.assets.cancel')}>
    <Stack gap="150">
      <p class="meta">{t('app.register.dispose_hint')}</p>
      <Select label={t('app.register.method_label')} options={DISPOSAL_METHODS.map((each) => ({ value: each, label: t(`app.register.method.${each}`) }))} bind:value={method} />
      <Input label={t('app.register.certificate')} hint={t('app.register.certificate_hint')} bind:value={certificate} maxlength={200} isRequired />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDisposeOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button
        tone="danger"
        data-confirm-disposal
        isBusy={isMoving}
        disabledReason={certificate.trim() ? undefined : t('cmdb.disposal_certificate_required')}
        onclick={() => void move('DISPOSED', { method, certificate: certificate.trim() })}
      >{t('app.register.dispose')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .part { display: grid; gap: var(--sp-100); }
  .part-head { display: flex; flex-wrap: wrap; align-items: end; justify-content: space-between; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .pairs { display: grid; grid-template-columns: minmax(0, max-content) minmax(0, 1fr); gap: var(--sp-050) var(--sp-150); margin: 0; }
  .pairs dt { color: var(--text-secondary); }
  .pairs dd { margin: 0; display: flex; flex-wrap: wrap; gap: var(--sp-050); align-items: baseline; overflow-wrap: anywhere; }
  .pairs a { color: var(--text-primary); font-weight: var(--fw-medium); }
  .move, .pair { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
  .request { display: grid; gap: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-100) var(--sp-150); margin: 0; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
