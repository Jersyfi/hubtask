<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // When an entry is due: a date, or a date and a time in a zone.
  //
  // **All-day is a date and never a midnight.** `due_date_only` is interpreted as a day in the
  // entry's zone (`i18n-l10n.md` §4), and rendering it as 00:00 would turn "Thursday" into an
  // instant that is Wednesday for half the world. So the time is absent rather than zero, and the
  // control does not offer one while all-day is on.
  //
  // **The zone is shown whenever it is not the reader's.** A 09:00 meeting in `America/Sao_Paulo`
  // read in Berlin is 14:00, and saying so is the entire reason the zone is stored beside the
  // instant. The *condition* is this component's — it compares the two — and the *sentence* is the
  // caller's, resolved, because this package writes none (ADR-0011).
  //
  // **Clearing is a control.** An empty date field is ambiguous: it reads as "I have not typed it
  // yet" as easily as "there is no due date", and one of those is a state worth saving. A button
  // says which.
  //
  // The reader's zone is handed in from their account. A component that read the machine's would
  // disagree with the account the person is signed in as, which is the case that matters: a laptop
  // carried to another country.

  import Button from './Button.svelte';
  import Input from './Input.svelte';
  import Switch from './Switch.svelte';
  import type { Disableable } from './control.ts';

  /** A due date as the model holds it: a day, optionally a time, optionally a zone of its own. */
  export interface DueDate {
    /** `YYYY-MM-DD`, in the entry's zone. */
    readonly date: string;
    /** `HH:mm`. Absent means all-day — never `00:00`, which is a different statement. */
    readonly time?: string;
    /** The entry's IANA zone. Absent means it is the reader's. */
    readonly zone?: string;
  }

  interface Props extends Disableable {
    label: string;
    value?: DueDate | null;
    /** The reader's zone, from their account. What the entry's is compared against. */
    readerZone: string;
    /** The names of the two fields and the two controls. Resolved text (ADR-0011). */
    dateLabel: string;
    timeLabel: string;
    allDayLabel: string;
    clearLabel: string;
    /** What "no due date" reads as, where there is none. */
    noDateLabel: string;
    /**
     * What the foreign zone means, as one resolved sentence — "09:00 in São Paulo, 14:00 for you".
     * Shown only when the entry's zone is not the reader's; the component decides *whether*, the
     * caller decides *what it says*.
     */
    zoneNoteLabel?: string;
    onChange?: (value: DueDate | null) => void;
  }

  const {
    label,
    value = null,
    readerZone,
    dateLabel,
    timeLabel,
    allDayLabel,
    clearLabel,
    noDateLabel,
    zoneNoteLabel,
    disabledReason,
    onChange,
  }: Props = $props();

  const unavailable = $derived(disabledReason !== undefined);
  const reasonId = $derived(unavailable ? `reason-${Math.random().toString(36).slice(2, 9)}` : undefined);

  const isAllDay = $derived(value !== null && value.time === undefined);
  /** Foreign means "not the reader's". An entry with no zone of its own is in theirs by definition. */
  const isForeignZone = $derived(value?.zone !== undefined && value.zone !== readerZone);

  function setDate(date: string) {
    if (date === '') return;
    onChange?.({ ...(value ?? {}), date, time: value?.time, zone: value?.zone });
  }

  function setTime(time: string) {
    if (!value) return;
    // An emptied time field is all-day again, which is the one place where empty is unambiguous:
    // the date is still there, so nothing is being cleared.
    onChange?.({ ...value, time: time === '' ? undefined : time });
  }

  function setAllDay(on: boolean) {
    if (!value) return;
    // A default of 09:00 rather than midnight when a day becomes timed: midnight is the boundary
    // two days share, and an entry due "at midnight" is one nobody meant to schedule.
    onChange?.({ ...value, time: on ? undefined : '09:00' });
  }
</script>

<fieldset class="control" aria-describedby={reasonId}>
  <legend>{label}</legend>

  <div class="row">
    <!-- The wave-1 `Input` rather than a raw one: it carries the shell, the focus ring, the
         disabled surface and the label wiring, and a fifth implementation of those in this package
         would be a fifth place to get them wrong. The platform draws the calendar and the clock,
         which is also what makes them work with a screen reader in every locale. -->
    <Input
      label={dateLabel}
      type="date"
      value={value?.date ?? ''}
      {disabledReason}
      onchange={(event) => setDate((event.currentTarget as HTMLInputElement).value)}
    />

    {#if value && !isAllDay}
      <Input
        label={timeLabel}
        type="time"
        value={value.time ?? ''}
        {disabledReason}
        onchange={(event) => setTime((event.currentTarget as HTMLInputElement).value)}
      />
    {/if}
  </div>

  {#if value}
    <Switch
      label={allDayLabel}
      checked={isAllDay}
      disabledReason={disabledReason}
      onchange={(event) => setAllDay((event.currentTarget as HTMLInputElement).checked)}
    />

    {#if isForeignZone}
      <!-- Rule 3: the zone is words, never a colour or an icon on its own. It is the difference
           between an entry a reader can act on and one they will be late for. -->
      <p class="zone">
        <span class="zone-name">{value.zone}</span>
        {#if zoneNoteLabel}<span>{zoneNoteLabel}</span>{/if}
      </p>
    {/if}

    <Button tone="subtle" disabledReason={disabledReason} onclick={() => onChange?.(null)}>
      {clearLabel}
    </Button>
  {:else}
    <p class="none">{noDateLabel}</p>
  {/if}

  {#if unavailable}<p class="reason" id={reasonId}>{disabledReason}</p>{/if}
</fieldset>

<style>
  .control {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    margin: 0;
    border: 0;
    padding: 0;
    min-width: 0;
  }

  legend {
    padding: 0;
    color: var(--text-primary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .zone { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; font-size: var(--fs-075); }

  .zone-name { color: var(--text-primary); font-weight: var(--fw-medium); unicode-bidi: isolate; }

  .zone,
  .none,
  .reason { color: var(--text-subtle); }

  .none,
  .reason { margin: 0; font-size: var(--fs-075); }
</style>
