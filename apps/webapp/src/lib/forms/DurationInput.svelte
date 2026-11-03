<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A duration typed in minutes or hours and held in minutes (user-experience.md §5.4). Switching
  // the unit keeps the number the reader sees and changes what it means - the way a person corrects
  // "4 minutes" to "4 hours" - rather than converting it under their eyes (`duration.ts`).

  import { Inline, Input, Select } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { DURATION_UNITS, amountIn, minutesOf, unitFor, type DurationUnit } from './duration.ts';

  interface Props {
    label: string;
    hint?: string;
    /** The minutes the server holds, as text; empty for none. */
    minutes: string;
    error?: string;
    isRequired?: boolean;
  }

  let { label, hint, minutes = $bindable(), error, isRequired = false }: Props = $props();

  // Read once from what the form was given: an edit opens in the unit its value fills.
  // svelte-ignore state_referenced_locally
  const opened = unitFor(minutes);
  let unit = $state<DurationUnit>(opened);
  // svelte-ignore state_referenced_locally
  let amount = $state(amountIn(minutes, opened));
  // A value set from outside - the form reset, another row opened - is shown again.
  let written = minutes;
  $effect(() => {
    if (minutes === written) return;
    unit = unitFor(minutes);
    amount = amountIn(minutes, unit);
    written = minutes;
  });

  function write() {
    minutes = minutesOf(amount, unit);
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
      options={DURATION_UNITS.map((each) => ({ value: each, label: t(`app.duration.unit_${each}`) }))}
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
