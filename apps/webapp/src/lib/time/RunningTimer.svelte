<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The reader's one timer in the head of *Time* (user-experience.md §7.6, UC-TIM-01 checks 3 and
  // 8): idle it starts on no entry; running it ticks with *Stop* and *Discard*; left running past
  // a day, a stop now would be refused, so it offers *Record until…* - a time on the start's day,
  // prefilled at the usual end of work - with *Discard* the second choice.

  import { untrack } from 'svelte';
  import { Banner, Button, Dialog, Icon, IconButton, Inline, Input, Stack } from '@hubtask/design-system/components';
  import type { CapacityList, TimeTimer, WorkTypeList } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { live } from '../data/live.svelte.ts';
  import { CAPACITIES, TIMER, WORK_TYPES, elapsedMinutes, minutesText, ranTooLong, time, usualEnd, weekStartOf } from '../data/time.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatPlainDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { dateIn, instantOf, timeIn } from '../i18n/zone.ts';
  import { renderProblem } from '../problem.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { DEFAULT_KINDS, owesDefaultKinds } from './kinds.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }
  const { onnavigate }: Props = $props();

  let timer = $state<TimeTimer | undefined>(undefined);
  let kinds = $state<WorkTypeList['data'] | undefined>(undefined);
  let capacities = $state<CapacityList['data']>([]);
  $effect(() => untrack(() => {
    const stops = [
      engine.subscribe<TimeTimer>({ path: TIMER }, (next) => { if (next.status === 'ready') timer = next.data; }),
      engine.subscribe<WorkTypeList>({ path: WORK_TYPES }, (next) => { if (next.status === 'ready') kinds = next.data.data; }),
      engine.subscribe<CapacityList>({ path: CAPACITIES }, (next) => { if (next.status === 'ready') capacities = next.data.data; }),
    ];
    return () => stops.forEach((stop) => stop());
  }));

  let now = $state(Date.now());
  $effect(() => {
    if (!timer?.running) return;
    const tick = setInterval(() => (now = Date.now()), 15_000);
    return () => clearInterval(tick);
  });
  const zone = $derived(actor.zone);
  const elapsed = $derived(timer?.running ? elapsedMinutes(timer.started_at, now) : 0);
  const isTooLong = $derived(timer?.running === true && ranTooLong(elapsed));
  const startDay = $derived(timer?.started_at ? dateIn(timer.started_at, zone) : undefined);
  const startClock = $derived(timer?.started_at ? timeIn(timer.started_at, zone) : undefined);
  const offline = $derived(live.state === 'offline' ? t('app.time.offline') : undefined);
  const firstKind = $derived(kinds?.find((each) => !each.archived)?.key);
  // A workspace without a kind of work gets the usual ones from the first reader who may write
  // them, so the timer starts on first use (§4); a failure leaves the reason on the button.
  let isSeeding = false;
  $effect(() => {
    if (isSeeding || !owesDefaultKinds(kinds, holdsHere('STRUCTURE') === true)) return;
    isSeeding = true;
    void untrack(async () => {
      try {
        for (const [at, kind] of DEFAULT_KINDS.entries()) {
          await time.defineWorkType(kind.key, { name: t(kind.code), billable: kind.billable, position: at + 1 });
        }
      } catch {
        // Kept quiet: the button says why it cannot start, and Time's settings add them by hand.
      }
    });
  });

  let isBusy = $state(false);
  let failure = $state<string | undefined>(undefined);
  async function run(work: () => Promise<unknown>, done: string): Promise<boolean> {
    isBusy = true;
    failure = undefined;
    try {
      await work();
      toast.show({ text: done, tone: 'success' });
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    } finally {
      isBusy = false;
      now = Date.now();
    }
  }

  const start = () => void run(() => time.startTimer({ work_type: firstKind }), t('app.time.timer_started'));
  const stop = () => void run(() => time.stopTimer(zone, startDay ? weekStartOf(startDay) : undefined), t('app.time.timer_stopped', { minutes: minutesText(Math.max(1, elapsed)) }));
  async function discard() {
    if (await run(() => time.discardTimer(), t('app.time.timer_discarded'))) isRecordingUntil = false;
  }

  // *Record until…*
  let isRecordingUntil = $state(false);
  let until = $state('');
  let untilError = $state<string | undefined>(undefined);
  function openUntil() {
    failure = undefined;
    untilError = undefined;
    until = (timer?.started_at ? usualEnd(timer.started_at, zone, capacities)?.time : undefined) ?? '17:00';
    isRecordingUntil = true;
  }
  async function recordUntil() {
    const day = startDay;
    if (!day || !startClock || !/^\d{2}:\d{2}$/u.test(until) || until <= startClock) {
      untilError = t('app.time.until_after_start', { start: startClock ?? '' });
      return;
    }
    const at = instantOf(day, until, zone);
    const minutes = at && timer?.started_at ? Math.ceil((Date.parse(at) - Date.parse(timer.started_at)) / 60_000) : 0;
    untilError = undefined;
    const ok = await run(
      () => time.stopTimer(zone, weekStartOf(day), at),
      t('app.time.recorded_on', { minutes: minutesText(Math.max(1, minutes)), day: formatPlainDay(day, messages.locale) }),
    );
    if (ok) isRecordingUntil = false;
  }
</script>

{#if timer}
  <span class="timer" data-timer={timer.running ? (isTooLong ? 'too-long' : 'running') : 'idle'}>
    {#if !timer.running}
      <Button size="sm" tone="secondary" icon="play" {isBusy} busyLabel={t('app.time.start')}
        disabledReason={offline ?? (firstKind ? undefined : t('app.time.no_work_types'))} onclick={start}>{t('app.time.start')}</Button>
    {:else if isTooLong}
      <span class="running warn" role="status"><Icon name="clock" size="sm" />{t('app.time.timer_too_long', { day: formatPlainDay(startDay ?? '', messages.locale), start: startClock ?? '' })}</span>
      <Button size="sm" tone="primary" disabledReason={offline} onclick={openUntil}>{t('app.time.record_until')}</Button>
    {:else}
      <span class="running" role="status"><Icon name="clock" size="sm" />{t('app.time.timer_running', { minutes: minutesText(elapsed) })}</span>
      {#if timer.entry_id}
        <!-- The entry by name where the reader may still read it (user-experience.md §7.6). -->
        <Button size="sm" tone="subtle" onclick={() => onnavigate(`/items/${timer?.entry_id}`)}>
          {timer.entry_title ? t('app.time.timer_on', { title: timer.entry_title }) : t('app.time.timer_open')}</Button>
      {/if}
      <Button size="sm" tone="secondary" icon="square" {isBusy} busyLabel={t('app.time.stop')} disabledReason={offline} onclick={stop}>{t('app.time.stop')}</Button>
      <IconButton size="sm" icon="x" label={t('app.time.discard')} disabledReason={offline} onclick={() => void discard()} />
    {/if}
    {#if failure && !isRecordingUntil}<span class="failure" role="alert">{failure}</span>{/if}
  </span>
{/if}

{#if isRecordingUntil}
  <Dialog bind:isOpen={isRecordingUntil} title={t('app.time.record_until_title')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.time.record_until_hint', { day: formatPlainDay(startDay ?? '', messages.locale), start: startClock ?? '' })}</p>
      <Input label={t('app.time.until')} type="time" bind:value={until} error={untilError} />
      {#if failure}<Banner tone="danger">{failure}</Banner>{/if}
      <Inline justify="end" gap="100">
        <Button tone="subtle" {isBusy} busyLabel={t('app.time.discard')} onclick={() => void discard()}>{t('app.time.discard')}</Button>
        <Button {isBusy} busyLabel={t('app.time.record')} onclick={() => void recordUntil()}>{t('app.time.record')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .timer { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .running { display: inline-flex; align-items: center; gap: var(--sp-050); font-size: var(--fs-075); font-variant-numeric: tabular-nums; font-weight: var(--fw-medium); }
  .warn { color: var(--text-warning); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { color: var(--text-danger); font-size: var(--fs-075); }
</style>
