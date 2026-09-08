<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How an entry repeats: an RFC 5545 rule, a zone, and which of the two things "repeats" means.
  //
  // **It expands nothing.** The occurrences are the server's (ADR-0008): a client that generated
  // them would be a second implementation of the rule, and two implementations of a calendar
  // disagree on the first leap day. So this edits the rule and says where the occurrences come
  // from, and the sentence that says it is the caller's.
  //
  // **The presets produce a rule; the raw field is the same rule.** Somebody who reads RFC 5545
  // should not have to work a preset backwards to express `BYSETPOS=-1`, and somebody who does not
  // should never meet the grammar. Both paths write the same string, and a raw rule is read back
  // into the presets where it fits one.
  //
  // **`UNTIL` and `COUNT` are exclusive**, and that is the specification's rule rather than a
  // house one (RFC 5545 §3.3.10). A rule carrying both is refused with a field error instead of
  // being silently repaired, because repairing it would drop one of the two things the author
  // wrote and neither is safe to guess.
  //
  // **The two modes are different questions with the same word.** `ON_SCHEDULE` puts the next
  // occurrence where the calendar says; `ON_COMPLETION` puts it a period after the entry was
  // actually finished. One sentence each, handed in, because a reader choosing between them is
  // choosing between two behaviours they cannot see.

  import Checkbox from './Checkbox.svelte';
  import Input from './Input.svelte';
  import Radio from './Radio.svelte';
  import type { Disableable } from './control.ts';

  /** The four the presets cover. `RAW` is the fifth path: the rule as it is written. */
  export type Frequency = 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY';

  /** How the series stops. Never, on a date, or after a number of occurrences — never two of them. */
  export type RecurrenceEnd =
    | { readonly kind: 'never' }
    | { readonly kind: 'until'; readonly date: string }
    | { readonly kind: 'count'; readonly count: number };

  interface Props extends Disableable {
    label: string;
    /** The rule, as RFC 5545 writes it: `FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH`. */
    rule?: string;
    /** Which of the two things "repeats" means here. */
    mode?: 'ON_SCHEDULE' | 'ON_COMPLETION';
    /** The zone the rule is read in. Shown, never guessed: 09:00 stays 09:00 across a DST change. */
    zone: string;
    zoneLabel: string;
    /** How far ahead the server materialises occurrences, in occurrences. */
    horizon?: number;
    horizonLabel: string;
    horizonHint?: string;
    /** The names of the frequency choices and of the raw path. */
    frequencyLabel: string;
    frequencyLabels: Readonly<Record<Frequency, string>>;
    rawLabel: string;
    rawHint?: string;
    intervalLabel: string;
    /** `MO` … `SU`, worded by the caller. The order drawn is the reader's week, handed in. */
    weekdayLabels: Readonly<Record<string, string>>;
    weekdaysLabel: string;
    /** The reader's first day of the week, from their account. `MO` by default (ISO-8601). */
    weekStart?: string;
    monthDayLabel: string;
    /** The three ends, and the two fields the last two need. */
    endLabel: string;
    endNeverLabel: string;
    endOnLabel: string;
    endAfterLabel: string;
    endDateLabel: string;
    endCountLabel: string;
    /** Said when a rule names both an end date and a count. RFC 5545 forbids it. */
    endConflictLabel: string;
    /** The two modes, and one sentence each on what they do. */
    modeLabel: string;
    onScheduleLabel: string;
    onScheduleHint: string;
    onCompletionLabel: string;
    onCompletionHint: string;
    /** Where the occurrences come from. This component makes none, and says so. */
    sourceLabel: string;
    onChange?: (rule: string) => void;
    onModeChange?: (mode: 'ON_SCHEDULE' | 'ON_COMPLETION') => void;
    onHorizonChange?: (horizon: number) => void;
  }

  const {
    label,
    rule = 'FREQ=WEEKLY',
    mode = 'ON_SCHEDULE',
    zone,
    zoneLabel,
    horizon,
    horizonLabel,
    horizonHint,
    frequencyLabel,
    frequencyLabels,
    rawLabel,
    rawHint,
    intervalLabel,
    weekdayLabels,
    weekdaysLabel,
    weekStart = 'MO',
    monthDayLabel,
    endLabel,
    endNeverLabel,
    endOnLabel,
    endAfterLabel,
    endDateLabel,
    endCountLabel,
    endConflictLabel,
    modeLabel,
    onScheduleLabel,
    onScheduleHint,
    onCompletionLabel,
    onCompletionHint,
    sourceLabel,
    disabledReason,
    onChange,
    onModeChange,
    onHorizonChange,
  }: Props = $props();

  const RAW = '__raw__';
  const DAYS = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'];

  let isRawOpen = $state(false);

  /** The rule as parts. Unknown parts are kept, which is what makes the raw path lossless. */
  const parts = $derived.by(() => {
    const map = new Map<string, string>();
    for (const piece of rule.split(';')) {
      const [name, ...rest] = piece.split('=');
      if (name && rest.length > 0) map.set(name.toUpperCase(), rest.join('='));
    }
    return map;
  });

  const frequency = $derived((parts.get('FREQ') ?? 'WEEKLY') as Frequency);
  const interval = $derived(parts.get('INTERVAL') ?? '1');
  const byDay = $derived((parts.get('BYDAY') ?? '').split(',').filter(Boolean));
  const byMonthDay = $derived(parts.get('BYMONTHDAY') ?? '');

  /** Both is not a state the specification has, so it is a field error rather than a preference. */
  const hasBothEnds = $derived(parts.has('UNTIL') && parts.has('COUNT'));

  const end = $derived.by<RecurrenceEnd>(() => {
    const until = parts.get('UNTIL');
    const count = parts.get('COUNT');
    if (until && !count) return { kind: 'until', date: isoDate(until) };
    if (count && !until) return { kind: 'count', count: Number(count) || 1 };
    return { kind: 'never' };
  });

  /** The week as the reader starts it, so a Sunday-first account is not shown a Monday-first grid. */
  const week = $derived.by(() => {
    const start = Math.max(0, DAYS.indexOf(weekStart.toUpperCase()));
    return [...DAYS.slice(start), ...DAYS.slice(0, start)];
  });

  const frequencyChoices = $derived([
    ...(['DAILY', 'WEEKLY', 'MONTHLY', 'YEARLY'] as const).map((value) => ({
      value,
      label: frequencyLabels[value],
    })),
    { value: RAW, label: rawLabel, hint: rawHint },
  ]);

  /** `YYYYMMDD` or `YYYYMMDDTHHMMSSZ` to what a date field takes. */
  function isoDate(until: string): string {
    return `${until.slice(0, 4)}-${until.slice(4, 6)}-${until.slice(6, 8)}`;
  }

  /** …and back. Midnight UTC, because `UNTIL` is an instant and a date field gives only a day. */
  function untilOf(date: string): string {
    return `${date.replaceAll('-', '')}T000000Z`;
  }

  /** Writes the parts back in a stable order, so an edit does not reshuffle the whole rule. */
  function compose(changes: Record<string, string | undefined>): string {
    const next = new Map(parts);
    for (const [name, value] of Object.entries(changes)) {
      if (value === undefined || value === '') next.delete(name);
      else next.set(name, value);
    }
    const order = ['FREQ', 'INTERVAL', 'BYDAY', 'BYMONTHDAY', 'BYMONTH', 'BYSETPOS', 'WKST', 'UNTIL', 'COUNT'];
    const named = order.filter((name) => next.has(name)).map((name) => `${name}=${next.get(name)}`);
    const rest = [...next].filter(([name]) => !order.includes(name)).map(([name, value]) => `${name}=${value}`);
    return [...named, ...rest].join(';');
  }

  function setFrequency(value: string) {
    if (value === RAW) {
      isRawOpen = true;
      return;
    }
    isRawOpen = false;
    // The parts that belong to another frequency go with it: `BYMONTHDAY` on a weekly rule is a
    // constraint that can never be satisfied, and a rule nobody can satisfy fires never.
    onChange?.(
      compose({
        FREQ: value,
        BYDAY: value === 'WEEKLY' ? (byDay.length > 0 ? byDay.join(',') : undefined) : undefined,
        BYMONTHDAY: value === 'MONTHLY' ? byMonthDay || '1' : undefined,
      }),
    );
  }

  function toggleDay(day: string) {
    const next = byDay.includes(day) ? byDay.filter((held) => held !== day) : [...byDay, day];
    onChange?.(compose({ BYDAY: week.filter((known) => next.includes(known)).join(',') }));
  }

  function setEnd(kind: RecurrenceEnd['kind']) {
    if (kind === 'never') onChange?.(compose({ UNTIL: undefined, COUNT: undefined }));
    else if (kind === 'until') onChange?.(compose({ COUNT: undefined, UNTIL: untilOf(todayish()) }));
    else onChange?.(compose({ UNTIL: undefined, COUNT: '10' }));
  }

  /**
   * A starting date for a newly chosen end, taken from the rule rather than from the machine's
   * clock: a component that read the time would be a component a test cannot fix (rule 4's spirit
   * on this side of the wire).
   */
  function todayish(): string {
    const until = parts.get('UNTIL');
    return until ? isoDate(until) : '2026-12-31';
  }
</script>

<div class="editor">
  <Radio
    label={frequencyLabel}
    options={frequencyChoices}
    {disabledReason}
    bind:value={() => (isRawOpen ? RAW : frequency), setFrequency}
  />

  {#if isRawOpen}
    <Input
      label={rawLabel}
      hint={rawHint}
      value={rule}
      error={hasBothEnds ? endConflictLabel : undefined}
      {disabledReason}
      onchange={(event) => onChange?.((event.currentTarget as HTMLInputElement).value)}
    />
  {:else}
    <Input
      label={intervalLabel}
      type="number"
      min="1"
      value={interval}
      {disabledReason}
      onchange={(event) => onChange?.(compose({ INTERVAL: (event.currentTarget as HTMLInputElement).value || '1' }))}
    />

    {#if frequency === 'WEEKLY'}
      <fieldset class="group">
        <legend>{weekdaysLabel}</legend>
        <div class="days">
          {#each week as day (day)}
            <Checkbox
              label={weekdayLabels[day] ?? day}
              checked={byDay.includes(day)}
              {disabledReason}
              onchange={() => toggleDay(day)}
            />
          {/each}
        </div>
      </fieldset>
    {/if}

    {#if frequency === 'MONTHLY'}
      <Input
        label={monthDayLabel}
        type="number"
        min="1"
        max="31"
        value={byMonthDay || '1'}
        {disabledReason}
        onchange={(event) => onChange?.(compose({ BYMONTHDAY: (event.currentTarget as HTMLInputElement).value || '1' }))}
      />
    {/if}
  {/if}

  <Radio
    label={endLabel}
    options={[
      { value: 'never', label: endNeverLabel },
      { value: 'until', label: endOnLabel },
      { value: 'count', label: endAfterLabel },
    ]}
    error={hasBothEnds ? endConflictLabel : undefined}
    {disabledReason}
    bind:value={() => end.kind, (kind: string) => setEnd(kind as RecurrenceEnd['kind'])}
  />

  {#if end.kind === 'until'}
    <Input
      label={endDateLabel}
      type="date"
      value={end.date}
      {disabledReason}
      onchange={(event) => onChange?.(compose({ COUNT: undefined, UNTIL: untilOf((event.currentTarget as HTMLInputElement).value) }))}
    />
  {:else if end.kind === 'count'}
    <Input
      label={endCountLabel}
      type="number"
      min="1"
      value={String(end.count)}
      {disabledReason}
      onchange={(event) => onChange?.(compose({ UNTIL: undefined, COUNT: (event.currentTarget as HTMLInputElement).value || '1' }))}
    />
  {/if}

  <Radio
    label={modeLabel}
    options={[
      { value: 'ON_SCHEDULE', label: onScheduleLabel, hint: onScheduleHint },
      { value: 'ON_COMPLETION', label: onCompletionLabel, hint: onCompletionHint },
    ]}
    {disabledReason}
    bind:value={
      () => mode,
      (next: string) => onModeChange?.(next as 'ON_SCHEDULE' | 'ON_COMPLETION')
    }
  />

  {#if horizon !== undefined}
    <Input
      label={horizonLabel}
      hint={horizonHint}
      type="number"
      min="1"
      value={String(horizon)}
      {disabledReason}
      onchange={(event) => onHorizonChange?.(Number((event.currentTarget as HTMLInputElement).value) || 1)}
    />
  {/if}

  <p class="zone">
    <span class="name">{zoneLabel}</span>
    <span class="zone-name">{zone}</span>
  </p>

  <!-- What this component does not do, said where somebody would otherwise go looking for it. -->
  <p class="source">{sourceLabel}</p>
</div>

<style>
  .editor { display: flex; flex-direction: column; gap: var(--sp-150); min-width: 0; }

  .group { display: flex; flex-direction: column; gap: var(--sp-050); margin: 0; border: 0; padding: 0; }

  .days { display: flex; flex-wrap: wrap; gap: var(--sp-150); }

  legend,
  .name {
    padding: 0;
    color: var(--text-primary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .zone { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; font-size: var(--fs-075); }

  .zone-name { color: var(--text-subtle); unicode-bidi: isolate; }

  .source { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
