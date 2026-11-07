<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The timer on an entry (UC-TIM-01 check 8): the person's one timer, the same on every device.
  // Running here, it shows its minutes and stops into the day it started; running on another
  // entry, it says so and stops there; idle, it starts here with a kind of work. Mounted by the
  // entry's page only where the time module is on.
  //
  // **Online only** (UC-TIM-01 *Where it ends*): offline the controls stay and say why.

  import { untrack } from 'svelte';
  import { Button, Icon, Inline, Select } from '@hubtask/design-system/components';
  import type { TimeTimer, WorkType, WorkTypeList } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { TIMER, WORK_TYPES, elapsedMinutes, minutesText, time, weekStartOf } from '../data/time.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { dateIn } from '../i18n/zone.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    itemId: string;
    onnavigate: (path: string) => void;
  }

  const { itemId, onnavigate }: Props = $props();

  const timer = resource<TimeTimer>(TIMER);
  const kinds = resource<WorkTypeList>(WORK_TYPES);
  const current = $derived(timer.state.status === 'ready' ? timer.state.data : undefined);
  const offered = $derived<WorkType[]>(kinds.state.status === 'ready' ? kinds.state.data.data.filter((each) => !each.archived) : []);
  const isHere = $derived(current?.running === true && current.entry_id === itemId);
  const isElsewhere = $derived(current?.running === true && current.entry_id !== itemId);

  let kind = $state('');
  $effect(() => {
    const first = offered[0]?.key;
    if (first && !untrack(() => kind)) kind = first;
  });

  let now = $state(Date.now());
  $effect(() => {
    if (!current?.running) return;
    const tick = setInterval(() => (now = Date.now()), 15_000);
    return () => clearInterval(tick);
  });
  const elapsed = $derived(current?.running ? elapsedMinutes(current.started_at, now) : 0);

  let isBusy = $state(false);
  let failure = $state<string | undefined>(undefined);
  const offline = $derived(live.state === 'offline' ? t('app.time.offline') : undefined);

  async function run(work: () => Promise<unknown>, done: string) {
    isBusy = true;
    failure = undefined;
    try {
      await work();
      toast.show({ text: done, tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
      now = Date.now();
    }
  }

  function start() {
    void run(() => time.startTimer({ entry_id: itemId, work_type: kind || undefined }), t('app.time.timer_started'));
  }

  function discard() {
    void run(() => time.discardTimer(), t('app.time.timer_discarded'));
  }

  function stop() {
    const zone = actor.zone;
    const day = current?.started_at ? dateIn(current.started_at, zone) : undefined;
    void run(() => time.stopTimer(zone, day ? weekStartOf(day) : undefined), t('app.time.timer_stopped', { minutes: minutesText(Math.max(1, elapsed)) }));
  }
</script>

{#if current}
  <div class="timer" data-timer={isHere ? 'here' : isElsewhere ? 'elsewhere' : 'idle'}>
    {#if isHere}
      <Inline gap="100" align="center">
        <span class="running" role="status"><Icon name="clock" size="sm" />{t('app.time.timer_running', { minutes: minutesText(elapsed) })}</span>
        <Button size="sm" icon="square" {isBusy} busyLabel={t('app.time.stop')} disabledReason={offline} onclick={stop}>{t('app.time.stop')}</Button>
        <Button size="sm" tone="subtle" icon="x" disabledReason={offline} onclick={discard}>{t('app.time.discard')}</Button>
      </Inline>
    {:else if isElsewhere}
      <Inline gap="100" align="center">
        <span class="quiet"><Icon name="clock" size="sm" />{t('app.time.timer_elsewhere', { minutes: minutesText(elapsed) })}</span>
        {#if current.entry_id}
          <Button size="sm" tone="subtle" onclick={() => onnavigate(`/items/${current.entry_id}`)}>{t('app.time.timer_open')}</Button>
        {/if}
        <Button size="sm" tone="secondary" {isBusy} busyLabel={t('app.time.stop')} disabledReason={offline} onclick={stop}>{t('app.time.stop')}</Button>
        <Button size="sm" tone="subtle" icon="x" disabledReason={offline} onclick={discard}>{t('app.time.discard')}</Button>
      </Inline>
    {:else}
      <Inline gap="100" align="end">
        {#if offered.length > 1}
          <Select label={t('app.time.work_type')} size="sm" bind:value={kind} options={offered.map((each) => ({ value: each.key, label: each.name }))} />
        {/if}
        <Button size="sm" tone="secondary" icon="play" {isBusy} busyLabel={t('app.time.start')} disabledReason={offline ?? (offered.length === 0 ? t('app.time.no_work_types') : undefined)} onclick={start}>{t('app.time.start')}</Button>
      </Inline>
    {/if}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  </div>
{/if}

<style>
  .timer { display: flex; flex-direction: column; gap: var(--sp-050); }
  .running, .quiet { display: inline-flex; align-items: center; gap: var(--sp-050); font-size: var(--fs-075); font-variant-numeric: tabular-nums; }
  .running { color: var(--text-primary); font-weight: var(--fw-medium); }
  .quiet { color: var(--text-secondary); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
