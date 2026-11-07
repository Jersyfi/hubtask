<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A duration typed in minutes, hours or business days and held in minutes (user-experience.md
  // §5.4); business days only when `day` gives their length. Switching
  // the unit keeps the number the reader sees and changes what it means - the way a person corrects
  // "4 minutes" to "4 hours" - rather than converting it under their eyes (`duration.ts`).

  import { Inline, Input, Select } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { amountIn, minutesOf, unitFor, unitsFor, type DurationUnit } from './duration.ts';

  interface Props {
    label: string;
    hint?: string;
    /** The minutes the server holds, as text; empty for none. */
    minutes: string;
    error?: string;
    isRequired?: boolean;
    /** The minutes of one business day, where the duration is counted by a calendar. */
    day?: number;
  }

  let { label, hint, minutes = $bindable(), error, isRequired = false, day }: Props = $props();

  // Read once from what the form was given: an edit opens in the unit its value fills.
  // svelte-ignore state_referenced_locally
  const opened = unitFor(minutes, day);
  let unit = $state<DurationUnit>(opened);
  // svelte-ignore state_referenced_locally
  let amount = $state(amountIn(minutes, opened, day));
  // A value set from outside - the form reset, another row opened - is shown again.
  // So is the same value under another day length - another calendar chosen.
  let written = minutes;
  // svelte-ignore state_referenced_locally
  let measured = day;
  $effect(() => {
    if (minutes === written && day === measured) return;
    unit = unitFor(minutes, day);
    amount = amountIn(minutes, unit, day);
    written = minutes;
    measured = day;
  });

  function write() {
    minutes = minutesOf(amount, unit, day);
    written = minutes;
  }
</script>

<div class="duration">
  <Inline gap="050" align="end">
    <Input {label} {hint} {error} {isRequired} type="number" min="0" step="any" inputmode="decimal" value={amount}
      oninput={(event: Event) => {
        // From the event: a handler beside a binding may run before the binding has written.
        amount = (event.currentTarget as HTMLInputElement).value;
        write();
      }}
    />
    <Select
      label={t('app.reminders.unit')}
      value={unit}
      options={unitsFor(day).map((each) => ({ value: each, label: t(`app.duration.unit_${each}`) }))}
      onchange={(event: Event) => {
        unit = (event.currentTarget as HTMLSelectElement).value as DurationUnit;
        write();
      }}
    />
  </Inline>
</div>

<style>
  .duration { min-inline-size: 0; }
</style>
