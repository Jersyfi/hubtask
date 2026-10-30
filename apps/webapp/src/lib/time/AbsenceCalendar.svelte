<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The shared calendar of absences, a month at a time (UC-TIM-07 check 7): people by days, each
  // day away marked with its kind - never a reason - and a half day said so; below, the reader's
  // own absences to record, change and remove. Every plan's workload reads them from here.

  import { untrack } from 'svelte';
  import { Button, Dialog, Inline, Input, Radio, Stack, Switch, Table, type Column } from '@hubtask/design-system/components';
  import type { Absence, AbsenceList } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { absenceGrid, absencesPath, addMonths, monthDays, time } from '../data/time.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { renderProblem } from '../problem.ts';

  const KINDS = ['LEAVE', 'TRAINING', 'OTHER'] as const;

  const today = $derived(todayIn(actor.zone));
  let month = $state('');
  $effect(() => { if (!month) month = today.slice(0, 7); });
  const days = $derived(monthDays(month));
  const path = $derived(days.length > 0 ? absencesPath(days[0]!, days.at(-1)!) : '');

  let absences = $state<Absence[] | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  $effect(() => {
    const read = path;
    if (!read) return;
    return untrack(() => engine.subscribe<AbsenceList>({ path: read }, (next) => {
      if (next.status === 'ready') absences = next.data.data;
      if (next.status === 'failed') failure = renderProblem(next.error, messages).message;
    }));
  });
  const rows = $derived(absenceGrid(absences ?? [], days));
  $effect(() => { if (rows.length > 0) untrack(() => accounts.resolve(rows.map((row) => row.accountId))); });
  const mine = $derived((absences ?? []).filter((each) => each.account_id === actor.account?.id));

  const columns = $derived<Column[]>([
    { id: 'person', label: t('app.absences.col_person') },
    ...days.map((day) => ({ id: day, label: day.slice(8), align: 'end' as const })),
  ]);

  let isEditing = $state(false);
  let editing = $state<Absence | undefined>(undefined);
  const draft = $state({ from: '', to: '', kind: 'LEAVE' as string, half: false });
  function open(absence: Absence | undefined) {
    editing = absence;
    draft.from = absence?.from ?? today;
    draft.to = absence?.to ?? today;
    draft.kind = absence?.kind ?? 'LEAVE';
    draft.half = absence?.half ?? false;
    failure = undefined;
    isEditing = true;
  }
  async function save() {
    const fields = { from: draft.from, to: draft.to, kind: draft.kind, half: draft.half };
    try {
      const read = editing;
      if (read) await time.updateAbsence(read, fields, path);
      else await time.recordAbsence(fields, path);
      toast.show({ text: t('app.absences.saved'), tone: 'success' });
      isEditing = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
  async function remove(absence: Absence) {
    try {
      await time.removeAbsence(absence, path);
      toast.show({ text: t('app.absences.removed'), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<Stack gap="200">
  <div class="controls">
    <Button size="sm" tone="subtle" icon="chevron-left" onclick={() => (month = addMonths(month, -1))}>{t('app.absences.month_before')}</Button>
    <span class="heading" aria-live="polite">{month}</span>
    <Button size="sm" tone="subtle" icon="chevron-right" onclick={() => (month = addMonths(month, 1))}>{t('app.absences.month_after')}</Button>
    <Button size="sm" icon="plus" onclick={() => open(undefined)}>{t('app.absences.add')}</Button>
  </div>
  {#if failure && !isEditing}<p class="failure" role="alert">{failure}</p>{/if}

  <div class="scroll">
    <Table label={t('app.absences.title')} isLabelHidden {columns}>
      {#snippet empty()}{t('app.absences.empty')}{/snippet}
      {#each rows as row (row.accountId)}
        <tr>
          <th scope="row">{accounts.nameOf(row.accountId) ?? t('app.people.unnamed')}</th>
          {#each row.days as absence, i (i)}
            <td class="day" class:away={absence !== undefined} title={absence ? t(`app.absences.kind_${absence.kind}`) : undefined}>
              {#if absence}{t(`app.absences.short_${absence.kind}`)}{absence.half ? t('app.absences.half_mark') : ''}{/if}
            </td>
          {/each}
        </tr>
      {/each}
    </Table>
  </div>
  <p class="quiet">{t('app.absences.legend')}</p>

  <section class="block" aria-labelledby="my-absences">
    <h2 id="my-absences" class="heading">{t('app.absences.mine')}</h2>
    {#if mine.length === 0}
      <p class="quiet">{t('app.absences.none_mine')}</p>
    {:else}
      <ul class="list">
        {#each mine as absence (absence.id)}
          <li>
            <span>{t('app.absences.span', { from: absence.from, to: absence.to, kind: t(`app.absences.kind_${absence.kind}`) })}{absence.half ? ` · ${t('app.absences.half')}` : ''}</span>
            <Inline gap="050">
              <Button size="sm" tone="subtle" icon="pencil" onclick={() => open(absence)}>{t('app.absences.edit')}</Button>
              <Button size="sm" tone="subtle" icon="trash" onclick={() => void remove(absence)}>{t('app.absences.remove')}</Button>
            </Inline>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</Stack>

{#if isEditing}
  <Dialog bind:isOpen={isEditing} title={editing ? t('app.absences.edit') : t('app.absences.add')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.absences.from')} type="date" bind:value={draft.from} />
      <Input label={t('app.absences.to')} type="date" bind:value={draft.to} />
      <Radio label={t('app.absences.kind')} name="absence-kind" bind:value={draft.kind}
        options={KINDS.map((kind) => ({ value: kind, label: t(`app.absences.kind_${kind}`), hint: t(`app.absences.kind_${kind}_hint`) }))} />
      <Switch label={t('app.absences.half')} bind:checked={draft.half} />
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      <Inline justify="end"><Button onclick={() => void save()}>{t('app.absences.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .controls { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: center; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .scroll { overflow-x: auto; }
  .day { text-align: center; font-size: var(--fs-075); }
  .away { background: var(--status-warning-surface); color: var(--status-warning-text); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .block { display: flex; flex-direction: column; gap: var(--sp-100); }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--sp-050); }
  .list li { display: flex; flex-wrap: wrap; justify-content: space-between; gap: var(--sp-100); align-items: center; }
</style>
