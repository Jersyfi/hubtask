<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A plan's budget and its burn (UC-TIM-08 check 8), on the plan's actuals for whoever manages
  // costs: the hours and the money budgeted against what was recorded and what it cost, whether
  // either is passed, the minutes no rate prices, and the burn week by week.
  //
  // **Drawn only for MANAGE_COSTS or MANAGE_MEMBERS** (P-05): the caller decides; the server
  // refuses anybody else all the same.

  import { untrack } from 'svelte';
  import { Badge, Banner, Button, Dialog, Inline, Input, Stack, Table, type Column } from '@hubtask/design-system/components';
  import type { BudgetBurn } from '@hubtask/sync-engine';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { budgetPath, burnShare, minorDigits, minorOf, minutesOfHours, minutesText, moneyText, time } from '../data/time.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { decimalSeparatorOf } from '../i18n/number.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    planId: string;
  }

  const { planId }: Props = $props();

  const locale = $derived(messages.locale);
  const separator = $derived(decimalSeparatorOf(locale, manifest.supportedLocales));

  let burn = $state<BudgetBurn | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = budgetPath(planId);
    return untrack(() => engine.subscribe<BudgetBurn>({ path }, (next) => {
      if (next.status === 'ready') { burn = next.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });
  const money = (minor: number) => moneyText(minor, burn?.currency ?? '', locale);
  const timeShare = $derived(burn ? burnShare(burn, 'time') : null);
  const moneyShare = $derived(burn ? burnShare(burn, 'money') : null);

  const columns = $derived<Column[]>([
    { id: 'week', label: t('app.budget.col_week') },
    { id: 'minutes', label: t('app.budget.col_time'), align: 'end' },
    { id: 'amount', label: t('app.budget.col_cost'), align: 'end' },
  ]);

  let isEditing = $state(false);
  const draft = $state({ hours: '', amount: '', currency: '' });
  let failure = $state<string | undefined>(undefined);
  function open() {
    if (!burn) return;
    const digits = 10 ** minorDigits(burn.currency || 'EUR');
    draft.hours = burn.budget_minutes ? String(burn.budget_minutes / 60).replace('.', separator) : '';
    draft.amount = burn.budget_amount_minor ? String(burn.budget_amount_minor / digits).replace('.', separator) : '';
    draft.currency = burn.currency;
    failure = undefined;
    isEditing = true;
  }
  async function save() {
    const read = burn;
    if (!read) return;
    const currency = draft.currency.trim().toUpperCase();
    const minutes = minutesOfHours(draft.hours, separator);
    const amount = currency ? minorOf(draft.amount, currency, separator) : draft.amount.trim() ? undefined : null;
    if (minutes === undefined) { failure = t('app.budget.hours_invalid'); return; }
    if (amount === undefined) { failure = t('app.budget.amount_invalid'); return; }
    try {
      await time.setBudget(planId, read, { minutes: minutes ?? 0, amount_minor: amount ?? 0, currency: amount ? currency : '' });
      toast.show({ text: t('app.budget.saved'), tone: 'success' });
      isEditing = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<section class="block" aria-labelledby="plan-budget">
  <Inline justify="between">
    <h2 id="plan-budget" class="heading">{t('app.budget.title')}</h2>
    {#if burn}<Button size="sm" tone="subtle" icon="pencil" onclick={open}>{t('app.budget.edit')}</Button>{/if}
  </Inline>
  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if burn}
    {#if !burn.budgeted}<p class="quiet">{t('app.budget.none')}</p>{/if}
    <dl class="facts">
      <dt>{t('app.budget.time')}</dt>
      <dd>
        {burn.budget_minutes > 0
          ? t('app.budget.of', { used: minutesText(burn.minutes), budget: minutesText(burn.budget_minutes), share: String(timeShare) })
          : minutesText(burn.minutes)}
        {#if burn.over_time}<Badge size="sm" tone="danger">{t('app.budget.over')}</Badge>{/if}
        {#if burn.budget_minutes > 0}<meter min="0" max={burn.budget_minutes} high={burn.budget_minutes * 0.9} value={burn.minutes}></meter>{/if}
      </dd>
      <dt>{t('app.budget.money')}</dt>
      <dd>
        {burn.budget_amount_minor > 0
          ? t('app.budget.of', { used: money(burn.amount_minor), budget: money(burn.budget_amount_minor), share: String(moneyShare) })
          : t('app.budget.no_money')}
        {#if burn.over_money}<Badge size="sm" tone="danger">{t('app.budget.over')}</Badge>{/if}
        {#if burn.budget_amount_minor > 0}<meter min="0" max={burn.budget_amount_minor} high={burn.budget_amount_minor * 0.9} value={burn.amount_minor}></meter>{/if}
      </dd>
      {#if burn.unpriced_minutes > 0}
        <dt>{t('app.budget.unpriced')}</dt>
        <dd>{minutesText(burn.unpriced_minutes)}</dd>
      {/if}
    </dl>
    {#if burn.weeks.length > 0}
      <div class="scroll">
        <Table label={t('app.budget.burn')} {columns}>
          {#each burn.weeks as week (week.week)}
            <tr>
              <th scope="row">{week.week}</th>
              <td class="number">{minutesText(week.minutes)}</td>
              <td class="number">{burn.currency ? money(week.amount_minor) : ''}</td>
            </tr>
          {/each}
        </Table>
      </div>
    {/if}
  {/if}
</section>

{#if isEditing}
  <Dialog bind:isOpen={isEditing} title={t('app.budget.edit')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.budget.hours')} hint={t('app.budget.hours_hint')} bind:value={draft.hours} />
      <Input label={t('app.budget.amount')} bind:value={draft.amount} />
      <Input label={t('app.budget.currency')} hint={t('app.budget.currency_hint')} maxlength={3} bind:value={draft.currency} />
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      <Inline justify="end"><Button onclick={() => void save()}>{t('app.budget.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .facts { display: grid; grid-template-columns: max-content 1fr; gap: var(--sp-050) var(--sp-200); margin: 0; }
  .facts dt { color: var(--text-secondary); }
  .facts dd { margin: 0; display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: center; }
  .scroll { overflow-x: auto; }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
