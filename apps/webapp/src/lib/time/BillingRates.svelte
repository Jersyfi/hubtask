<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's or a collection's billing rates on its actuals (UC-TIM-08 check 9): what an hour of its
  // billable time is charged to its client, an amount in a currency, from a day to a day or on. A
  // collection's own rate comes before its hub's; the server refuses two rates of one plan on a day.
  //
  // **Drawn only for MANAGE_COSTS or MANAGE_MEMBERS at the plan** (P-05), as the budget is.

  import { untrack } from 'svelte';
  import { Banner, Button, Dialog, Inline, Input, Stack, Table, type Column } from '@hubtask/design-system/components';
  import type { BillingRate, BillingRateList } from '@hubtask/sync-engine';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { billingRatesPath, minorDigits, minorOf, moneyText, time } from '../data/time.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { decimalSeparatorOf } from '../i18n/number.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The hub or collection whose billing rates these are. */
    planId: string;
  }

  const { planId }: Props = $props();

  const locale = $derived(messages.locale);
  const separator = $derived(decimalSeparatorOf(locale, manifest.supportedLocales));

  let rates = $state<BillingRate[] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = billingRatesPath(planId);
    return untrack(() => engine.subscribe<BillingRateList>({ path }, (next) => {
      if (next.status === 'ready') { rates = next.data.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });

  const columns = $derived<Column[]>([
    { id: 'rate', label: t('app.billing.col_rate'), align: 'end' },
    { id: 'from', label: t('app.rates.col_from') },
    { id: 'to', label: t('app.rates.col_to') },
    { id: 'actions', label: t('app.rates.col_actions'), isLabelHidden: true },
  ]);

  let isEditing = $state(false);
  let editing = $state<BillingRate | undefined>(undefined);
  const draft = $state({ amount: '', currency: 'EUR', from: '', to: '' });
  let failure = $state<string | undefined>(undefined);
  function open(rate: BillingRate | undefined) {
    editing = rate;
    draft.currency = rate?.currency ?? 'EUR';
    draft.amount = rate ? String(rate.hour_minor / 10 ** minorDigits(rate.currency)).replace('.', separator) : '';
    draft.from = rate?.from ?? '';
    draft.to = rate?.to ?? '';
    failure = undefined;
    isEditing = true;
  }
  async function save() {
    const currency = draft.currency.trim().toUpperCase();
    const amount = minorOf(draft.amount, currency, separator);
    if (amount === null || amount === undefined) { failure = t('app.rates.amount_invalid'); return; }
    if (!draft.from) { failure = t('app.rates.from_required'); return; }
    const days = { hour_minor: amount, currency, from: draft.from, ...(draft.to ? { to: draft.to } : {}) };
    try {
      const read = editing;
      if (read) await time.changeBillingRate(read, days);
      else await time.addBillingRate(planId, days);
      toast.show({ text: t('app.rates.saved'), tone: 'success' });
      isEditing = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
  async function remove(rate: BillingRate) {
    try {
      await time.removeBillingRate(rate);
      toast.show({ text: t('app.rates.removed'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }
</script>

<section class="block" aria-labelledby="billing-rates">
  <Inline justify="between">
    <h2 id="billing-rates" class="heading">{t('app.billing.title')}</h2>
    <Button size="sm" tone="subtle" icon="plus" onclick={() => open(undefined)}>{t('app.rates.add')}</Button>
  </Inline>
  <p class="quiet">{t('app.billing.hint')}</p>
  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if rates}
    <Table label={t('app.billing.title')} isLabelHidden {columns}>
      {#snippet empty()}{t('app.billing.empty')}{/snippet}
      {#each rates as rate (rate.id)}
        <tr>
          <td class="number">{moneyText(rate.hour_minor, rate.currency, locale)}</td>
          <td>{rate.from}</td>
          <td>{rate.to ?? t('app.rates.open_ended')}</td>
          <td>
            <Inline gap="050">
              <Button size="sm" tone="subtle" icon="pencil" onclick={() => open(rate)}>{t('app.rates.edit')}</Button>
              <Button size="sm" tone="subtle" icon="trash" onclick={() => void remove(rate)}>{t('app.rates.remove')}</Button>
            </Inline>
          </td>
        </tr>
      {/each}
    </Table>
  {/if}
</section>

{#if isEditing}
  <Dialog bind:isOpen={isEditing} title={editing ? t('app.rates.edit') : t('app.rates.add')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.rates.amount')} bind:value={draft.amount} />
      <Input label={t('app.rates.currency')} hint={t('app.budget.currency_hint')} maxlength={3} bind:value={draft.currency} />
      <Input label={t('app.rates.col_from')} type="date" bind:value={draft.from} />
      <Input label={t('app.rates.col_to')} hint={t('app.rates.to_hint')} type="date" bind:value={draft.to} />
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      <Inline justify="end"><Button onclick={() => void save()}>{t('app.rates.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
