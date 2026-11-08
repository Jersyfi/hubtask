<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The shared calendar of absences, a month at a time (UC-TIM-07 check 7): people by days, each
  // day away marked with its kind - never a reason - and a half day said so; below, the reader's
  // own absences to record, change and remove. Every plan's workload reads them from here.

  import { untrack } from 'svelte';
  import { Button, DataTable, Dialog, Input, Radio, Stack, Switch, type DataColumn, type DataRowMenu } from '@hubtask/design-system/components';
  import type { Absence, AbsenceList } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { absenceGrid, absencesPath, addMonths, dayHeading, monthDays, time } from '../data/time.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatPlainDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { renderProblem } from '../problem.ts';

  const KINDS = ['LEAVE', 'TRAINING', 'OTHER'] as const;

  const today = $derived(todayIn(actor.zone));
  let month = $state('');
  $effect(() => { if (!month) month = today.slice(0, 7); });
  const days = $derived(monthDays(month));
  /** The month in the reader's words (§5.10), never `2026-11`. */
  const monthText = $derived(month ? new Intl.DateTimeFormat(messages.locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(Date.parse(`${month}-01T00:00:00Z`)) : '');
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

  // A person's month is one strip of day marks in one column, not a column per day: the calendar
  // stays one DataTable that never scrolls sideways (page-system §4.3, PS-4). Priority 2 makes the
  // strip the row's line on a phone, where a column that is neither title nor line is not drawn.
  const columns = $derived<DataColumn[]>([
    { id: 'person', label: t('app.absences.col_person'), priority: 1 },
    { id: 'month', label: t('app.absences.col_month'), priority: 2 },
  ]);
  type Row = (typeof rows)[number];
  const mineColumns = $derived<DataColumn[]>([{ id: 'span', label: t('app.absences.mine'), priority: 1 }]);
  const mineMenu = (absence: Absence): DataRowMenu => ({
    label: t('app.page.more_for', { name: formatPlainDay(absence.from, messages.locale) }),
    items: [{ id: 'remove', label: t('app.absences.remove'), isDestructive: true }],
  });

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
    <span class="heading" aria-live="polite">{monthText}</span>
    <Button size="sm" tone="subtle" icon="chevron-right" onclick={() => (month = addMonths(month, 1))}>{t('app.absences.month_after')}</Button>
    <Button size="sm" icon="plus" onclick={() => open(undefined)}>{t('app.absences.add')}</Button>
  </div>
  {#if failure && !isEditing}<p class="failure" role="alert">{failure}</p>{/if}

  <DataTable label={t('app.absences.title')} {columns} {rows} key={(row: Row) => row.accountId}>
    {#snippet cell(row: Row, column: DataColumn)}
      {#if column.id === 'person'}{accounts.nameOf(row.accountId) ?? t('app.people.unnamed')}
      {:else}
        <ol class="month">
          {#each row.days as absence, i (i)}
            <li class="day" class:week={i % 7 === 0}
              aria-label={`${dayHeading(days[i] ?? '', messages.locale)}${absence ? `: ${t(`app.absences.kind_${absence.kind}`)}` : ''}`}
              title={absence ? t(`app.absences.kind_${absence.kind}`) : undefined}>
              <span class="number" aria-hidden="true">{i + 1}</span>
              <span class="mark" class:away={absence !== undefined}>{#if absence}{t(`app.absences.short_${absence.kind}`)}{absence.half ? t('app.absences.half_mark') : ''}{/if}</span>
            </li>
          {/each}
        </ol>
      {/if}
    {/snippet}
    {#snippet empty()}<p class="quiet">{t('app.absences.empty')}</p>{/snippet}
  </DataTable>
  <p class="quiet">{t('app.absences.legend')}</p>

  <section class="block" aria-labelledby="my-absences">
    <h2 id="my-absences" class="heading">{t('app.absences.mine')}</h2>
    {#if mine.length === 0}
      <p class="quiet">{t('app.absences.none_mine')}</p>
    {:else}
      <DataTable label={t('app.absences.mine')} columns={mineColumns} rows={mine} key={(absence: Absence) => absence.id}
        onopen={(absence: Absence) => open(absence)} menu={mineMenu} onmenu={(absence: Absence) => void remove(absence)}
        rowLabel={(absence: Absence) => formatPlainDay(absence.from, messages.locale)} actionsLabel={t('app.page.row_actions')}
        sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')}>
        {#snippet cell(absence: Absence)}
          {t('app.absences.span', { from: formatPlainDay(absence.from, messages.locale), to: formatPlainDay(absence.to, messages.locale), kind: t(`app.absences.kind_${absence.kind}`) })}{absence.half ? ` · ${t('app.absences.half')}` : ''}
        {/snippet}
        {#snippet verb(absence: Absence)}
          <Button size="sm" tone="subtle" icon="pencil" onclick={() => open(absence)}>{t('app.absences.edit')}</Button>
        {/snippet}
      </DataTable>
    {/if}
  </section>
</Stack>

{#if isEditing}
  <Dialog bind:isOpen={isEditing} title={editing ? t('app.absences.edit') : t('app.absences.add')} dismissLabel={t('app.links.cancel')} size="m"
    primary={{ label: t('app.absences.save'), onclick: () => void save() }}>
    <Stack gap="150">
      <Input label={t('app.absences.from')} type="date" bind:value={draft.from} />
      <Input label={t('app.absences.to')} type="date" bind:value={draft.to} />
      <Radio label={t('app.absences.kind')} name="absence-kind" bind:value={draft.kind}
        options={KINDS.map((kind) => ({ value: kind, label: t(`app.absences.kind_${kind}`), hint: t(`app.absences.kind_${kind}_hint`) }))} />
      <Switch label={t('app.absences.half')} bind:checked={draft.half} />
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
    </Stack>
  </Dialog>
{/if}

<style>
  .controls { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: center; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .month { display: grid; grid-template-columns: repeat(31, minmax(0, 1fr)); gap: var(--bw-hairline); margin: 0; padding: 0; list-style: none; container-type: inline-size; }
  .day { display: flex; flex-direction: column; min-inline-size: 0; text-align: center; font-size: var(--fs-075); }
  .mark { min-block-size: var(--sp-300); overflow: hidden; border-radius: var(--r-sm); background: var(--bg-surface-sunken); line-height: var(--sp-300); }
  /* The day's number above its mark: every day where a day is wide enough, every week's first on a
     phone - spilling over the narrow days beside it, never widening the strip (PS-4). */
  .number { visibility: hidden; color: var(--text-secondary); text-align: start; white-space: nowrap; }
  .week .number { visibility: visible; }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a container query cannot read a custom property. */
  @container (inline-size >= 600px) {
    .number { visibility: visible; text-align: center; }
  }
  .away { background: var(--status-warning-surface); color: var(--status-warning-text); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .block { display: flex; flex-direction: column; gap: var(--sp-100); }
</style>
