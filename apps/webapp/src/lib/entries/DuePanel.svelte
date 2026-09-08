<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // When an entry is wanted, and when it starts.
  //
  // **Two controls, because the model has two shapes.** A due date is three fields written
  // together and cleared together — "none of them means anything alone" — and has a writer of its
  // own. A start is one instant with nothing qualifying it, and is a plain scalar on the patch.
  // That is D-01's decision, and a panel that treated them alike would be flattening it.
  //
  // **The zone travels with the date.** It defaults to the reader's account zone, and an entry set
  // in another one says so: the control draws the note, and the sentence is this file's, because a
  // component writes no text.
  //
  // **An all-day date is a date.** No time, in any locale — a time drawn for one is a midnight
  // that shifts with the viewer (`i18n-l10n.md` §4).

  import { Button, DueDateControl, Input, Stack, type DueDate } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { dueInputFor, dueOf } from '../data/due.ts';
  import { items } from '../data/items.svelte.ts';
  import { formatDue } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { dateIn, timeIn } from '../i18n/zone.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    item: WorkItem;
    /** Why the dates cannot be changed — archived, or a role that may not. */
    disabledReason?: string;
  }

  const { item, disabledReason }: Props = $props();

  const zone = $derived(actor.zone);
  const due = $derived(dueOf(item, zone));

  /**
   * What a foreign zone means, as one sentence with both clocks in it.
   *
   * The control decides *whether* to show it — it knows whether the zone is the reader's — and
   * this decides what it says. Both times are formatted through `Intl`, so the phrase is the
   * reader's own even though the fact is the entry's.
   */
  const zoneNote = $derived.by(() => {
    if (!item.due_at || !due?.zone || item.due_date_only) return undefined;
    return t('app.due.zone_note', {
      time: formatDue(item.due_at, messages.locale, due.zone, { showZone: true }),
      reader_time: formatDue(item.due_at, messages.locale, zone, { showZone: true }),
    });
  });

  /** The start, as the two halves an input pair needs, on the reader's clock. */
  const startDate = $derived(item.start_at ? (dateIn(item.start_at, zone) ?? '') : '');
  const startTime = $derived(item.start_at ? (timeIn(item.start_at, zone) ?? '') : '');

  let draftStartDate = $state('');
  let draftStartTime = $state('');
  let failure = $state<string | undefined>(undefined);

  // Seeded from the entry, and re-seeded when a different entry arrives. In an effect rather than
  // a `$derived`, because these are what somebody is typing into.
  $effect(() => {
    draftStartDate = startDate;
    draftStartTime = startTime;
  });

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    failure = undefined;
    try {
      await work();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  function setDue(next: DueDate | null) {
    void attempt(async () => {
      if (next === null) {
        await items.clearDue(item.id, item.version);
        return;
      }
      const input = dueInputFor(next, zone);
      // A date the platform could not read is not sent as one. The control's own fields are a date
      // input and a time input, so this is close to unreachable — and sending `undefined` as an
      // instant would be a 422 about a field nobody typed.
      if (!input) return;
      await items.setDue(item.id, input, item.version);
    });
  }

  function saveStart() {
    void attempt(async () => {
      if (draftStartDate === '') {
        await items.update(item.id, { start_at: null }, item.version);
        return;
      }
      // The reader's own zone: a start carries none of its own, so the only honest reading of what
      // somebody typed is the clock they typed it on.
      const at = `${draftStartDate}T${draftStartTime === '' ? '00:00' : draftStartTime}`;
      const instant = new Date(at).toISOString();
      await items.update(item.id, { start_at: instant }, item.version);
    });
  }
</script>

<Stack gap="150">
  <DueDateControl
    label={t('app.due.label')}
    value={due}
    readerZone={zone}
    dateLabel={t('app.due.date')}
    timeLabel={t('app.due.time')}
    allDayLabel={t('app.due.all_day')}
    clearLabel={t('app.due.clear')}
    noDateLabel={t('app.due.none')}
    zoneNoteLabel={zoneNote}
    {disabledReason}
    onChange={setDue}
  />

  <div class="start">
    <Input
      label={t('app.due.start')}
      type="date"
      bind:value={draftStartDate}
      {disabledReason}
      onchange={saveStart}
    />
    <Input
      label={t('app.due.time')}
      type="time"
      bind:value={draftStartTime}
      {disabledReason}
      onchange={saveStart}
    />
    {#if item.start_at}
      <Button
        size="sm"
        tone="secondary"
        {disabledReason}
        onclick={() => {
          draftStartDate = '';
          draftStartTime = '';
          saveStart();
        }}
      >
        {t('app.due.start_clear')}
      </Button>
    {/if}
  </div>

  {#if failure}<p class="failure">{failure}</p>{/if}
</Stack>

<style>
  .start { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
