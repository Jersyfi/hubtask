<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Whether this entry repeats, and how.
  //
  // **No series is not an error.** `GET` answers `404` for an entry with none, and the contract
  // says that is the "none" state — so the panel draws "this entry does not repeat" and offers to
  // make it, while an actual failure still reads as one.
  //
  // **Only a `TASK` carries a series**, and the matrix says why: a series applies to the whole
  // subtree. A work package and an activity get the gate with the server's own code.
  //
  // **Setting and changing are the same call.** `PUT` is one document (D-04), because a rule is one
  // thing an entry either carries or does not.
  //
  // **Removing it leaves every occurrence standing**, and the dialog says so before anybody
  // presses: those are ordinary entries and somebody's work. Skipping is different again — it
  // moves the bookkeeping past the *next* one, which has not been made yet.

  import { untrack } from 'svelte';

  import {
    Button,
    CapabilityGate,
    Dialog,
    Inline,
    RecurrenceEditor,
    Skeleton,
    Stack,
  } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { supports } from '../data/capability.svelte.ts';
  import { series } from '../data/reminders.svelte.ts';
  import { belongsToSeries, occurrenceSourceOf, splitEnd, withEnd } from '../data/reminders.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { item }: { item: WorkItem } = $props();

  const capability = $derived(supports(item.type, 'RECURRENCE'));

  $effect(() => {
    const wanted = item.id;
    return untrack(() => series.open(wanted));
  });

  const rule = $derived(series.of(item.id));
  const hasNone = $derived(series.hasNone(item.id));
  const isReading = $derived(series.isReading(item.id));
  const readFailure = $derived(series.failureOf(item.id));

  /**
   * The entry this one repeats from, from the row itself.
   *
   * `recurrence_source_id` is written only on a copy the materialisation made, so it says both
   * that this is an occurrence and which entry to link to — where the `404` from
   * `/items/{id}/recurrence` said only the first half, and left nothing to link to.
   */
  const source = $derived(occurrenceSourceOf(item));

  /**
   * An occurrence: it belongs to a series and is not the entry the series belongs to.
   *
   * The source alone would answer it. The `404` is kept in the condition for an entry restored
   * from an archive taken before the source column existed: it belongs to a series, has no rule of
   * its own, and is an occurrence with nothing to link to.
   */
  const isOccurrence = $derived(Boolean(source) || (hasNone && belongsToSeries(item)));

  /** The zone the rule is read in. The entry's due zone where it has one, the reader's otherwise. */
  const zone = $derived(rule?.time_zone ?? item.due_time_zone ?? actor.zone);

  let draft = $state('FREQ=WEEKLY');
  let mode = $state<'ON_SCHEDULE' | 'ON_COMPLETION'>('ON_SCHEDULE');
  let horizon = $state(90);
  let isEditing = $state(false);
  let isRemoving = $state(false);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let notice = $state<string | undefined>(undefined);

  // Seeded from the rule when one arrives, so opening the editor on an existing series starts from
  // what it is rather than from a default.
  $effect(() => {
    if (!rule) return;
    // Composed back together for the editor: it writes an end into the rule, and the API stores it
    // beside one. `splitEnd` takes them apart again on the way out.
    draft = withEnd(rule.rrule, rule);
    mode = (rule.mode as 'ON_SCHEDULE' | 'ON_COMPLETION') ?? 'ON_SCHEDULE';
    horizon = rule.horizon_days ?? 90;
  });

  /**
   * The weekday names, keyed as RFC 5545 keys them.
   *
   * The order the editor draws them in is the reader's week, which it takes from `weekStart` — the
   * account's own, mapped to the two letters the rule uses.
   */
  const WEEKDAYS = {
    MO: t('app.recurrence.weekday_MO'),
    TU: t('app.recurrence.weekday_TU'),
    WE: t('app.recurrence.weekday_WE'),
    TH: t('app.recurrence.weekday_TH'),
    FR: t('app.recurrence.weekday_FR'),
    SA: t('app.recurrence.weekday_SA'),
    SU: t('app.recurrence.weekday_SU'),
  };

  const weekStart = $derived(
    actor.weekStart === 'SUNDAY' ? 'SU' : actor.weekStart === 'SATURDAY' ? 'SA' : 'MO',
  );

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    isSaving = true;
    failure = undefined;
    notice = undefined;
    try {
      await work();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  function save() {
    void attempt(async () => {
      // The end travels beside the rule rather than inside it: this API refuses a rule that
      // carries `UNTIL` or `COUNT` — `recurrence.rrule_carries_end` — and takes the two fields.
      const { rrule, ends_at, max_count } = splitEnd(draft);
      await series.set(
        item.id,
        { rrule, time_zone: zone, mode: mode as never, horizon_days: horizon, ends_at, max_count },
        rule?.version,
      );
      isEditing = false;
    });
  }

  function remove() {
    if (!rule) return;
    void attempt(async () => {
      await series.remove(item.id, rule.version);
      isRemoving = false;
    });
  }

  function skip() {
    void attempt(async () => {
      await series.skip(item.id, crypto.randomUUID());
      notice = t('app.recurrence.skipped');
    });
  }
</script>

<CapabilityGate
  status={capability.status}
  reason={capability.status === 'refused' ? t(capability.code, capability.params) : undefined}
  pendingLabel={t('app.reminders.deciding')}
>
  <Stack gap="150">
    {#if isReading}
      <div aria-busy="true"><Skeleton lines={2} /></div>
    {:else if readFailure}
      <p class="failure">{renderProblem(readFailure, messages).message}</p>
    {:else if isOccurrence}
      <p class="quiet">{t('app.recurrence.occurrence')}</p>
      {#if source}
        <div>
          <a href={`/items/${source}`}>{t('app.recurrence.open_source')}</a>
        </div>
      {/if}
    {:else if hasNone && !isEditing}
      <p class="quiet">{t('app.recurrence.none')}</p>
      <div>
        <Button size="sm" tone="secondary" onclick={() => (isEditing = true)}>
          {t('app.recurrence.set')}
        </Button>
      </div>
    {:else}
      {#if rule && !isEditing}
        <p class="rule">{rule.rrule}</p>
        <Inline gap="100">
          <Button size="sm" tone="secondary" onclick={() => (isEditing = true)}>
            {t('app.entries.edit')}
          </Button>
          <Button size="sm" tone="secondary" isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={skip}>
            {t('app.recurrence.skip')}
          </Button>
          <Button size="sm" tone="secondary" onclick={() => (isRemoving = true)}>
            {t('app.recurrence.remove')}
          </Button>
        </Inline>
      {/if}

      {#if isEditing}
        <RecurrenceEditor
          label={t('app.recurrence.editor')}
          rule={draft}
          {mode}
          {zone}
          zoneLabel={t('app.recurrence.zone')}
          {horizon}
          horizonLabel={t('app.recurrence.horizon')}
          horizonHint={t('app.recurrence.horizon_hint')}
          frequencyLabel={t('app.recurrence.frequency')}
          frequencyLabels={{
            DAILY: t('app.recurrence.freq_DAILY'),
            WEEKLY: t('app.recurrence.freq_WEEKLY'),
            MONTHLY: t('app.recurrence.freq_MONTHLY'),
            YEARLY: t('app.recurrence.freq_YEARLY'),
          }}
          rawLabel={t('app.recurrence.raw')}
          rawHint={t('app.recurrence.raw_hint')}
          intervalLabel={t('app.recurrence.interval')}
          weekdayLabels={WEEKDAYS}
          weekdaysLabel={t('app.recurrence.weekdays')}
          {weekStart}
          monthDayLabel={t('app.recurrence.month_day')}
          endLabel={t('app.recurrence.end')}
          endNeverLabel={t('app.recurrence.end_never')}
          endOnLabel={t('app.recurrence.end_on')}
          endAfterLabel={t('app.recurrence.end_after')}
          endDateLabel={t('app.recurrence.end_date')}
          endCountLabel={t('app.recurrence.end_count')}
          endConflictLabel={t('app.recurrence.end_conflict')}
          modeLabel={t('app.recurrence.mode')}
          onScheduleLabel={t('app.recurrence.on_schedule')}
          onScheduleHint={t('app.recurrence.on_schedule_hint')}
          onCompletionLabel={t('app.recurrence.on_completion')}
          onCompletionHint={t('app.recurrence.on_completion_hint')}
          sourceLabel={t('app.recurrence.source')}
          onChange={(next) => (draft = next)}
          onModeChange={(next) => (mode = next)}
          onHorizonChange={(next) => (horizon = next)}
        />

        <!-- A rule the server cannot read comes back as a field error on `/rrule`, which is the
             raw field: the reader typed it, so that is where it belongs. -->
        {#if failure?.fields.get('/rrule')}
          <p class="failure">{failure.fields.get('/rrule')}</p>
        {/if}

        <Inline gap="100">
          <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={save}>
            {t('app.recurrence.save')}
          </Button>
          <Button tone="secondary" onclick={() => (isEditing = false)}>
            {t('app.workspace.cancel')}
          </Button>
        </Inline>
      {/if}
    {/if}

    {#if notice}<p class="quiet">{notice}</p>{/if}
    {#if failure && !failure.fields.get('/rrule')}<p class="failure">{failure.message}</p>{/if}
  </Stack>
</CapabilityGate>

<Dialog
  bind:isOpen={isRemoving}
  title={t('app.recurrence.remove_title')}
  dismissLabel={t('app.workspace.cancel')}
>
  <Stack gap="150">
    <p class="quiet">{t('app.recurrence.remove_explains')}</p>
    <Inline gap="100">
      <Button tone="danger" isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={remove}>
        {t('app.recurrence.remove')}
      </Button>
      <Button tone="secondary" onclick={() => (isRemoving = false)}>{t('app.workspace.cancel')}</Button>
    </Inline>
  </Stack>
</Dialog>

<style>
  .rule { margin: 0; font-family: var(--font-mono); }

  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
