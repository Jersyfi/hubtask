<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's hourly cost rates under *How we work* > *Time* (UC-TIM-08 check 8): of a person
  // or of a role, an amount in a currency, from a day to a day or on. A person's rate comes before
  // their role's; the server refuses two rates of one subject on a day.
  //
  // **Drawn only for MANAGE_COSTS or MANAGE_MEMBERS** (P-05): what a person earns is nobody else's.

  import { untrack } from 'svelte';
  import { Banner, Button, Dialog, Inline, Input, Radio, Stack, Table, type Column } from '@hubtask/design-system/components';
  import type { CostRate, CostRateList } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { RATES, minorDigits, minorOf, moneyText, time } from '../data/time.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { decimalSeparatorOf } from '../i18n/number.ts';
  import PersonPicker from '../people/PersonPicker.svelte';
  import { renderProblem } from '../problem.ts';

  const locale = $derived(messages.locale);
  const separator = $derived(decimalSeparatorOf(locale, manifest.supportedLocales));

  let rates = $state<CostRate[] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => untrack(() => engine.subscribe<CostRateList>({ path: RATES }, (next) => {
    if (next.status === 'ready') { rates = next.data.data; refusal = undefined; }
    if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
  })));
  $effect(() => {
    const ids = (rates ?? []).map((rate) => rate.account_id).filter((id): id is string => !!id);
    if (ids.length > 0) untrack(() => accounts.resolve(ids));
  });
  const subjectOf = (rate: CostRate) =>
    rate.account_id ? (accounts.nameOf(rate.account_id) ?? t('app.people.unnamed')) : t('app.rates.role_named', { role: rate.role ?? '' });

  const columns = $derived<Column[]>([
    { id: 'subject', label: t('app.rates.col_subject') },
    { id: 'rate', label: t('app.rates.col_rate'), align: 'end' },
    { id: 'from', label: t('app.rates.col_from') },
    { id: 'to', label: t('app.rates.col_to') },
    { id: 'actions', label: t('app.rates.col_actions'), isLabelHidden: true },
  ]);

  let isEditing = $state(false);
  let editing = $state<CostRate | undefined>(undefined);
  const draft = $state({ subject: 'person', person: '', role: '', amount: '', currency: 'EUR', from: '', to: '' });
  let failure = $state<string | undefined>(undefined);
  function open(rate: CostRate | undefined) {
    editing = rate;
    draft.subject = rate?.role ? 'role' : 'person';
    draft.person = rate?.account_id ?? '';
    draft.role = rate?.role ?? '';
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
      if (read) await time.changeRate(read, days);
      else await time.addRate({ ...days, ...(draft.subject === 'role' ? { role: draft.role.trim() } : { account_id: draft.person }) });
      toast.show({ text: t('app.rates.saved'), tone: 'success' });
      isEditing = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
  async function remove(rate: CostRate) {
    try {
      await time.removeRate(rate);
      toast.show({ text: t('app.rates.removed'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }
</script>

<section class="block" aria-labelledby="time-rates">
  <Inline justify="between">
    <h2 id="time-rates" class="heading">{t('app.rates.title')}</h2>
    <Button size="sm" tone="subtle" icon="plus" onclick={() => open(undefined)}>{t('app.rates.add')}</Button>
  </Inline>
  <p class="quiet">{t('app.rates.hint')}</p>
  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if rates}
    <Table label={t('app.rates.title')} isLabelHidden {columns}>
      {#snippet empty()}{t('app.rates.empty')}{/snippet}
      {#each rates as rate (rate.id)}
        <tr>
          <th scope="row">{subjectOf(rate)}</th>
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
      {#if !editing}
        <Radio label={t('app.rates.subject')} name="rate-subject" bind:value={draft.subject} options={[
          { value: 'person', label: t('app.rates.subject_person') },
          { value: 'role', label: t('app.rates.subject_role') },
        ]} />
        {#if draft.subject === 'person'}
          <PersonPicker label={t('app.rates.person')} noneLabel={t('app.person_picker.choose')} bind:value={draft.person} />
        {:else}
          <Input label={t('app.rates.role')} hint={t('app.rates.role_hint')} maxlength={64} bind:value={draft.role} />
        {/if}
      {:else}
        <p>{subjectOf(editing)}</p>
      {/if}
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
  p { margin: 0; }
</style>
