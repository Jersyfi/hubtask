<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The reader's capacity on *My time* (UC-TIM-06 check 7): the hours of each weekday, Monday
  // first, from a day on until the next capacity begins - what every plan's workload counts them
  // as able to give. The capacities so far are listed, the latest last.

  import { untrack } from 'svelte';
  import { Button, Inline, Input, Stack, Table, type Column } from '@hubtask/design-system/components';
  import type { Capacity, CapacityList } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { CAPACITIES, dayHeading, minutesOfHours, minutesText, time, weekDays, weekStartOf } from '../data/time.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { decimalSeparatorOf } from '../i18n/number.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { renderProblem } from '../problem.ts';

  const locale = $derived(messages.locale);
  const separator = $derived(decimalSeparatorOf(locale, manifest.supportedLocales));
  const heads = $derived(weekDays(weekStartOf(todayIn(actor.zone)) ?? todayIn(actor.zone)).map((day) => dayHeading(day, locale).split(' ')[0] ?? day));

  let capacities = $state<Capacity[] | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  $effect(() => untrack(() => engine.subscribe<CapacityList>({ path: CAPACITIES }, (next) => {
    if (next.status === 'ready') capacities = next.data.data;
    if (next.status === 'failed') failure = renderProblem(next.error, messages).message;
  })));

  const draft = $state({ from: '', hours: ['8', '8', '8', '8', '8', '0', '0'] });
  $effect(() => { if (!draft.from) draft.from = todayIn(actor.zone); });

  const columns = $derived<Column[]>([
    { id: 'from', label: t('app.capacity.col_from') },
    ...heads.map((head, i) => ({ id: `d${i}`, label: head, align: 'end' as const })),
    { id: 'actions', label: t('app.capacity.col_actions'), isLabelHidden: true },
  ]);

  async function save() {
    failure = undefined;
    const minutes = draft.hours.map((each) => minutesOfHours(each, separator));
    if (minutes.some((each) => each === undefined || (each ?? 0) > 1440)) { failure = t('app.capacity.hours_invalid'); return; }
    try {
      await time.setCapacity(draft.from, minutes.map((each) => each ?? 0));
      toast.show({ text: t('app.capacity.saved'), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
  async function remove(capacity: Capacity) {
    try {
      await time.removeCapacity(capacity.from);
      toast.show({ text: t('app.capacity.removed'), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<Stack gap="200">
  <p class="quiet">{t('app.capacity.hint')}</p>
  <div class="week">
    <Input label={t('app.capacity.col_from')} type="date" bind:value={draft.from} />
    {#each heads as head, i (i)}
      <Input label={head} inputmode="decimal" bind:value={draft.hours[i]} />
    {/each}
  </div>
  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  <Inline justify="end"><Button onclick={() => void save()}>{t('app.capacity.save')}</Button></Inline>

  {#if capacities && capacities.length > 0}
    <div class="scroll">
      <Table label={t('app.capacity.title')} {columns}>
        {#each capacities as capacity (capacity.from)}
          <tr>
            <th scope="row">{capacity.from}</th>
            {#each capacity.week_minutes as minutes, i (i)}<td class="number">{minutesText(minutes)}</td>{/each}
            <td><Button size="sm" tone="subtle" icon="trash" onclick={() => void remove(capacity)}>{t('app.capacity.remove')}</Button></td>
          </tr>
        {/each}
      </Table>
    </div>
  {:else if capacities}
    <p class="quiet">{t('app.capacity.none')}</p>
  {/if}
</Stack>

<style>
  .week { display: grid; grid-template-columns: repeat(auto-fill, minmax(12ch, 1fr)); gap: var(--sp-100); }
  .scroll { overflow-x: auto; }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
