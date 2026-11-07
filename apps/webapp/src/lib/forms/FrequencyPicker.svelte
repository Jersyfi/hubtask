<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How often a job runs, as choices - daily, weekly on a day, monthly on a date, at a time - with
  // the RFC 5545 rule behind *Advanced* (user-experience.md §5.4). Both paths write the same rule;
  // a rule the choices cannot say exactly is shown as *Its own rule* with the raw field open, and
  // is never rewritten until somebody picks a choice (`frequency.ts`).

  import { Disclosure, Input, Select, Stack } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { FREQUENCIES, WEEKDAYS, frequencyOf, ruleOf, type Frequency, type FrequencyKind, type Weekday } from './frequency.ts';

  interface Props {
    label: string;
    hint?: string;
    /** The rule, RFC 5545 without a start: `FREQ=DAILY;BYHOUR=3;BYMINUTE=0`. */
    rule: string;
    /** The hint of the raw field - what the server reads the rule as. */
    rawHint?: string;
    error?: string;
    onchange?: (rule: string) => void;
  }

  let { label, hint, rule = $bindable(), rawHint, error, onchange }: Props = $props();

  const read = $derived(frequencyOf(rule));
  // What a choice starts from when the held rule is not one: three at night, when nobody works.
  let last = $state<Frequency>({ freq: 'DAILY', weekday: 'MO', monthDay: 1, hour: 3, minute: 0 });
  const shown = $derived(read ?? last);
  let isAdvancedOpen = $state(false);
  $effect(() => {
    if (!read && rule.trim() !== '') isAdvancedOpen = true;
  });

  const pad = (value: number) => String(value).padStart(2, '0');
  const time = $derived(`${pad(shown.hour)}:${pad(shown.minute)}`);
  // A day past the 28th skips the months without one; offered only when the held rule has it.
  const monthDays = $derived([...new Set([...Array.from({ length: 28 }, (_, at) => at + 1), shown.monthDay])].sort((a, b) => a - b));

  function choose(next: Partial<Frequency>) {
    const choice = { ...shown, ...next };
    last = choice;
    rule = ruleOf(choice);
    onchange?.(rule);
  }

  const picked = (event: Event) => (event.currentTarget as HTMLSelectElement).value;
</script>

<fieldset class="frequency">
  <legend class="label">{label}</legend>
  {#if hint}<p class="hint">{hint}</p>{/if}
  <Stack gap="100">
    <Select
      label={t('app.recurrence.frequency')}
      value={read ? read.freq : 'RAW'}
      options={[
        ...FREQUENCIES.map((each) => ({ value: each, label: t(`app.recurrence.freq_${each}`) })),
        ...(read ? [] : [{ value: 'RAW', label: t('app.frequency.custom') }]),
      ]}
      {error}
      onchange={(event: Event) => {
        const value = picked(event);
        if (value !== 'RAW') choose({ freq: value as FrequencyKind });
      }}
    />
    {#if read?.freq === 'WEEKLY'}
      <Select
        label={t('app.frequency.weekday')}
        value={read.weekday}
        options={WEEKDAYS.map((each) => ({ value: each, label: t(`app.recurrence.weekday_${each}`) }))}
        onchange={(event: Event) => choose({ weekday: picked(event) as Weekday })}
      />
    {/if}
    {#if read?.freq === 'MONTHLY'}
      <Select
        label={t('app.recurrence.month_day')}
        value={String(read.monthDay)}
        options={monthDays.map((each) => ({ value: String(each), label: String(each) }))}
        onchange={(event: Event) => choose({ monthDay: Number(picked(event)) })}
      />
    {/if}
    {#if read}
      <Input
        label={t('app.frequency.at')}
        type="time"
        value={time}
        onchange={(event: Event) => {
          const [hour, minute] = (event.currentTarget as HTMLInputElement).value.split(':').map(Number);
          if (Number.isInteger(hour) && Number.isInteger(minute)) choose({ hour, minute });
        }}
      />
    {/if}
    <Disclosure label={t('app.frequency.advanced')} tone="quiet" bind:isOpen={isAdvancedOpen}>
      <Input
        label={t('app.recurrence.raw')}
        hint={rawHint}
        value={rule}
        oninput={(event: Event) => {
          // From the event: a handler beside a binding may run before the binding has written.
          rule = (event.currentTarget as HTMLInputElement).value;
          onchange?.(rule);
        }}
      />
    </Disclosure>
  </Stack>
</fieldset>

<style>
  .frequency { display: grid; gap: var(--sp-050); min-inline-size: 0; margin: 0; padding: 0; border: 0; }
  .label { padding: 0; color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }
  .hint { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
