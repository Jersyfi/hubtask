<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // When to be reminded, on which channels, and who is reminded.
  //
  // **Two paths, one specification.** A reminder is either relative to the due date — `REL:-PT1H`,
  // an ISO 8601 duration before it — or an absolute instant, `ABS:` and a local time. The presets
  // are the first path with the arithmetic already done, and they are **client vocabulary**: "an
  // hour before" is a phrase, `-PT1H` is the value, and the phrases are handed in because this
  // package writes no text (ADR-0011).
  //
  // **The entry's zone decides when it fires; the recipient's decides how it reads**
  // (`i18n-l10n.md` §4). That is a server decision and nothing here computes it — an absolute
  // instant is entered as a local time and the caller attaches the zone, because a component that
  // resolved one would be resolving it in the wrong zone half the time.
  //
  // **A channel this client does not know is kept.** The list comes from the installation, and one
  // whose name this client has no phrase for is shown by its value rather than dropped: tolerance
  // towards unknown values is binding, and a channel nobody can see is a channel nobody can turn
  // off.
  //
  // **Nobody named is not nobody.** An empty recipient list means the assignee and the members,
  // resolved when the reminder fires — which is a sentence a reader needs, because "nobody" is
  // what an empty list looks like.

  import type { Snippet } from 'svelte';

  import Checkbox from './Checkbox.svelte';
  import Input from './Input.svelte';
  import Radio, { type RadioOption } from './Radio.svelte';
  import Select from './Select.svelte';
  import type { Disableable } from './control.ts';

  /** How long before the due date. The units a person actually thinks in. */
  export type OffsetUnit = 'M' | 'H' | 'D' | 'W';

  /** One channel the installation offers. `label` absent means this client has no phrase for it. */
  export interface ReminderChannel {
    readonly value: string;
    readonly label?: string;
    readonly disabledReason?: string;
  }

  interface Props extends Disableable {
    label: string;
    /** The specification, as `REL:<duration>` or `ABS:<local time>`. Null means no reminder yet. */
    value?: string | null;
    /**
     * The common offsets, as this client words them — `{ value: 'REL:-PT1H', label: 'An hour
     * before' }`. Empty is allowed: the free entry is always there.
     */
    presets?: readonly RadioOption[];
    /** What the free-entry option is called, and the two paths inside it. */
    customLabel: string;
    beforeLabel: string;
    atLabel: string;
    /** The names of the free-entry fields. */
    amountLabel: string;
    unitLabel: string;
    dateLabel: string;
    timeLabel: string;
    /** The four units, worded by the caller. Keyed by the unit letter the duration uses. */
    unitLabels: Readonly<Record<OffsetUnit, string>>;
    /** What the group of channels is called, and which the installation offers. */
    channelsLabel: string;
    channels: readonly ReminderChannel[];
    selectedChannels?: readonly string[];
    /** Said beside a channel this client has no phrase for. */
    unknownChannelLabel: string;
    /** What the recipients are called, and who they are when nobody is named. */
    recipientsLabel: string;
    defaultRecipientsLabel: string;
    /** F3-07's picker, handed in. Absent means the caller is not offering a choice yet. */
    recipients?: Snippet;
    onChange?: (value: string) => void;
    onChannelsChange?: (channels: readonly string[]) => void;
  }

  const {
    label,
    value = null,
    presets = [],
    customLabel,
    beforeLabel,
    atLabel,
    amountLabel,
    unitLabel,
    dateLabel,
    timeLabel,
    unitLabels,
    channelsLabel,
    channels,
    selectedChannels = [],
    unknownChannelLabel,
    recipientsLabel,
    defaultRecipientsLabel,
    recipients,
    disabledReason,
    onChange,
    onChannelsChange,
  }: Props = $props();

  const CUSTOM = '__custom__';

  const isPreset = $derived(value !== null && presets.some((preset) => preset.value === value));
  const isAbsolute = $derived(value?.startsWith('ABS:') === true);

  /** What the free-entry duration currently says, read back so the fields show what is stored. */
  const parsed = $derived.by(() => {
    const match = /^REL:-P(?:T(\d+)([MH])|(\d+)([DW]))$/.exec(value ?? '');
    if (!match) return { amount: '1', unit: 'H' as OffsetUnit };
    return {
      amount: match[1] ?? match[3] ?? '1',
      unit: ((match[2] ?? match[4]) as OffsetUnit) ?? 'H',
    };
  });

  const absolute = $derived.by(() => {
    const [date = '', time = ''] = (value?.startsWith('ABS:') ? value.slice(4) : '').split('T');
    return { date, time: time.slice(0, 5) };
  });

  const choices = $derived([...presets, { value: CUSTOM, label: customLabel }]);
  const chosen = $derived(isPreset ? (value as string) : CUSTOM);

  const unitOptions = $derived(
    (['M', 'H', 'D', 'W'] as const).map((unit) => ({ value: unit, label: unitLabels[unit] })),
  );

  function relative(amount: string, unit: OffsetUnit): string {
    // At least one: a reminder zero units before the due date is the due date, and the `REL:`
    // grammar has no way to say "at the time" that is not `-PT0M`, which reads as a mistake.
    const count = Math.max(1, Math.round(Number(amount) || 1));
    return unit === 'M' || unit === 'H' ? `REL:-PT${count}${unit}` : `REL:-P${count}${unit}`;
  }

  function absoluteAt(date: string, time: string): string {
    // Local, and without a zone: the caller attaches the entry's, because that is what decides
    // when it fires and this component cannot know it.
    return `ABS:${date}T${time || '09:00'}`;
  }
</script>

<div class="editor">
  <Radio
    {label}
    options={choices}
    {disabledReason}
    bind:value={
      () => chosen,
      (next: string) => onChange?.(next === CUSTOM ? relative(parsed.amount, parsed.unit) : next)
    }
  />

  {#if !isPreset}
    <Radio
      label={customLabel}
      options={[
        { value: 'REL', label: beforeLabel },
        { value: 'ABS', label: atLabel },
      ]}
      {disabledReason}
      bind:value={
        () => (isAbsolute ? 'ABS' : 'REL'),
        (next: string) =>
          onChange?.(
            next === 'ABS' ? absoluteAt(absolute.date, absolute.time) : relative(parsed.amount, parsed.unit),
          )
      }
    />

    <div class="row">
      {#if isAbsolute}
        <Input
          label={dateLabel}
          type="date"
          value={absolute.date}
          {disabledReason}
          onchange={(event) => onChange?.(absoluteAt((event.currentTarget as HTMLInputElement).value, absolute.time))}
        />
        <Input
          label={timeLabel}
          type="time"
          value={absolute.time}
          {disabledReason}
          onchange={(event) => onChange?.(absoluteAt(absolute.date, (event.currentTarget as HTMLInputElement).value))}
        />
      {:else}
        <Input
          label={amountLabel}
          type="number"
          min="1"
          value={parsed.amount}
          {disabledReason}
          onchange={(event) => onChange?.(relative((event.currentTarget as HTMLInputElement).value, parsed.unit))}
        />
        <Select
          label={unitLabel}
          options={unitOptions}
          value={parsed.unit}
          {disabledReason}
          onchange={(event) => onChange?.(relative(parsed.amount, (event.currentTarget as HTMLSelectElement).value as OffsetUnit))}
        />
      {/if}
    </div>
  {/if}

  <fieldset class="group">
    <legend>{channelsLabel}</legend>
    {#each channels as channel (channel.value)}
      <div class="channel">
        <Checkbox
          label={channel.label ?? channel.value}
          checked={selectedChannels.includes(channel.value)}
          disabledReason={channel.disabledReason ?? disabledReason}
          onchange={() =>
            onChannelsChange?.(
              selectedChannels.includes(channel.value)
                ? selectedChannels.filter((held) => held !== channel.value)
                : [...selectedChannels, channel.value],
            )}
        />
        {#if !channel.label}
          <!-- Kept rather than dropped: a channel nobody can see is a channel nobody can turn off. -->
          <span class="note">{unknownChannelLabel}</span>
        {/if}
      </div>
    {/each}
  </fieldset>

  <div class="recipients">
    <p class="name">{recipientsLabel}</p>
    {#if recipients}
      {@render recipients()}
    {:else}
      <p class="note">{defaultRecipientsLabel}</p>
    {/if}
  </div>
</div>

<style>
  .editor { display: flex; flex-direction: column; gap: var(--sp-150); min-width: 0; }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .group,
  .recipients { display: flex; flex-direction: column; gap: var(--sp-050); min-width: 0; }

  .group { margin: 0; border: 0; padding: 0; }

  legend,
  .name {
    margin: 0;
    padding: 0;
    color: var(--text-primary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .channel { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); }

  .note { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
