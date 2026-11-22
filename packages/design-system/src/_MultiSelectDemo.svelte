<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Dialog from './Dialog.svelte';
  import InContext from './_InContext.svelte';
  import Input from './Input.svelte';
  import MultiSelect from './MultiSelect.svelte';
  import Stack from './Stack.svelte';
  import { setCloseLabel } from './media.ts';
  import type { ChoiceGroup, ChoiceOption } from './multiselect.ts';

  type Mode =
    | 'empty'
    | 'some'
    | 'all'
    | 'locked'
    | 'refused'
    | 'readonly'
    | 'invalid'
    | 'grouped'
    | 'summary'
    | 'german'
    | 'inline'
    | 'dense'
    | 'dialog'
    | 'row'
    | 'table';

  const { mode = 'some' }: { mode?: Mode } = $props();

  // What the application sets once in its language; a popover becomes a sheet only with it.
  $effect(() => {
    setCloseLabel(mode === 'german' ? 'Schließen' : 'Close');
    return () => setCloseLabel(undefined);
  });

  const always = 'Every entry can be done.';
  const english: readonly ChoiceGroup[] = [
    {
      label: 'Status and time',
      options: [
        { value: 'COMPLETION', label: 'Done or open', disabledReason: always },
        { value: 'DUE_DATE', label: 'Dates', hint: 'A start and a due date' },
        { value: 'REMINDER', label: 'Reminders' },
        { value: 'RECURRENCE', label: 'Repeats' },
      ],
    },
    {
      label: 'People',
      options: [
        { value: 'ASSIGNMENT', label: 'Responsible person', hint: 'One person who sees it through' },
        { value: 'MEMBERS', label: 'Also on it' },
      ],
    },
    {
      label: 'Content',
      options: [
        { value: 'NOTES', label: 'Notes' },
        { value: 'LABELS', label: 'Labels' },
        { value: 'COMMENTS', label: 'Comments' },
        { value: 'COVER', label: 'Cover' },
        { value: 'ATTACHMENTS', label: 'Attachments' },
        { value: 'HISTORY', label: 'History' },
      ],
    },
    {
      label: 'Extensions',
      options: [
        { value: 'BUCKET', label: 'Board column' },
        { value: 'CUSTOM_FIELDS', label: 'Custom fields' },
        { value: 'AMOUNT', label: 'Amount' },
      ],
    },
  ];

  // The fifteen of "Was Aufgabe trägt", as locales/de.json words them.
  const german: readonly ChoiceGroup[] = [
    {
      label: 'Status und Zeit',
      options: [
        { value: 'COMPLETION', label: 'Erledigt oder offen', disabledReason: 'Jeder Eintrag kann erledigt werden.' },
        { value: 'DUE_DATE', label: 'Termine' },
        { value: 'REMINDER', label: 'Erinnerungen' },
        { value: 'RECURRENCE', label: 'Wiederholung' },
      ],
    },
    {
      label: 'Personen',
      options: [
        { value: 'ASSIGNMENT', label: 'Verantwortliche Person' },
        { value: 'MEMBERS', label: 'Auch dabei' },
      ],
    },
    {
      label: 'Inhalt',
      options: [
        { value: 'NOTES', label: 'Notizen' },
        { value: 'LABELS', label: 'Labels' },
        { value: 'COMMENTS', label: 'Kommentare' },
        { value: 'COVER', label: 'Titelbild' },
        { value: 'ATTACHMENTS', label: 'Anhänge' },
        { value: 'HISTORY', label: 'Verlauf' },
      ],
    },
    {
      label: 'Erweiterungen',
      options: [
        { value: 'BUCKET', label: 'Board-Spalte' },
        { value: 'CUSTOM_FIELDS', label: 'Eigene Felder' },
        { value: 'AMOUNT', label: 'Menge' },
      ],
    },
  ];

  /** The same fifteen without groups and without the lock: a flat field. */
  const flat: readonly ChoiceOption[] = english.flatMap((group) => group.options).map(({ value, label, hint }) => ({ value, label, hint }));
  const flatValues = flat.map((each) => each.value);
  const withRefused: readonly ChoiceOption[] = flat.map((each) =>
    each.value === 'BUCKET' ? { ...each, disabledReason: 'Offer the board first: a column belongs to a board.' } : each,
  );

  const levels: readonly ChoiceOption[] = [
    { value: 'HUB', label: 'Hubs' },
    { value: 'COLLECTION', label: 'Collections' },
    { value: 'TASK', label: 'Tasks' },
    { value: 'WORK_PACKAGE', label: 'Work packages', disabledReason: 'Switch tasks on first: a work package sits inside one.' },
    { value: 'ACTIVITY', label: 'Activities' },
  ];

  const views: readonly ChoiceOption[] = [
    { value: 'LIST', label: 'List' },
    { value: 'BOARD', label: 'Board' },
    { value: 'CALENDAR', label: 'Calendar' },
    { value: 'TIMELINE', label: 'Timeline' },
  ];

  const initial: Record<Mode, readonly string[]> = {
    empty: [],
    some: ['DUE_DATE', 'NOTES', 'COMMENTS'],
    all: flatValues,
    locked: ['COMPLETION', 'DUE_DATE', 'ASSIGNMENT', 'NOTES', 'LABELS', 'COMMENTS', 'ATTACHMENTS', 'HISTORY', 'BUCKET'],
    refused: ['DUE_DATE', 'NOTES', 'COMMENTS'],
    readonly: ['DUE_DATE', 'NOTES', 'COMMENTS', 'LABELS'],
    invalid: [],
    grouped: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'NOTES', 'COMMENTS'],
    summary: ['COMPLETION', ...flatValues.filter((value) => value !== 'COMPLETION' && value !== 'AMOUNT')],
    german: ['COMPLETION', 'DUE_DATE', 'ASSIGNMENT', 'MEMBERS', 'NOTES', 'LABELS', 'COMMENTS', 'ATTACHMENTS'],
    inline: ['LIST', 'BOARD'],
    dense: ['HUB', 'COLLECTION', 'TASK'],
    dialog: ['DUE_DATE', 'NOTES'],
    row: ['HUB', 'COLLECTION'],
    table: ['HUB', 'COLLECTION'],
  };

  let selected = $state<readonly string[]>([]);
  let isMarked = $state(false);
  let isEditing = $state(false);
  let title = $state('Release notes');
  $effect(() => {
    selected = initial[mode];
    isMarked = mode === 'invalid';
  });

  const isGerman = $derived(mode === 'german');
  const texts = $derived(
    isGerman
      ? {
          label: 'Was Aufgabe trägt',
          empty: 'Nichts gewählt',
          filter: 'Felder filtern',
          clear: 'Filter leeren',
          noMatch: 'Kein Feld passt zu diesem Filter.',
          all: 'Alle',
          none: 'Keine',
          done: 'Fertig',
          count: (chosen: number, total: number) => `${chosen} von ${total}`,
        }
      : {
          label: 'What a task carries',
          empty: 'Nothing chosen',
          filter: 'Filter the fields',
          clear: 'Clear the filter',
          noMatch: 'No field matches this filter.',
          all: 'All',
          none: 'None',
          done: 'Done',
          count: (chosen: number, total: number) => `${chosen} of ${total}`,
        },
  );

  /** "All 15", or "All except …" with one or two off - the structure panel's sentence. */
  function nearlyAll(chosen: readonly ChoiceOption[], total: number, all: readonly ChoiceOption[]): string | undefined {
    if (chosen.length === total) return `All ${total}`;
    const off = all.filter((each) => !chosen.includes(each));
    if (off.length <= 2) return `All except ${off.map((each) => each.label).join(' and ')}`;
    return undefined;
  }
  const englishAll = english.flatMap((group) => group.options);
</script>

{#snippet fields(extra: { groups?: readonly ChoiceGroup[]; options?: readonly ChoiceOption[]; error?: string; disabledReason?: string; summary?: (chosen: readonly ChoiceOption[], total: number) => string | undefined })}
  <MultiSelect
    label={texts.label}
    groups={extra.groups}
    options={extra.options}
    {selected}
    onChange={(next) => (selected = next)}
    error={extra.error}
    disabledReason={extra.disabledReason}
    summary={extra.summary}
    emptyLabel={texts.empty}
    filterLabel={texts.filter}
    clearLabel={texts.clear}
    noMatchLabel={texts.noMatch}
    allLabel={texts.all}
    noneLabel={texts.none}
    doneLabel={texts.done}
    countLabel={texts.count}
    locale={isGerman ? 'de' : 'en'}
  />
{/snippet}

<div class="demo" data-wide={mode === 'row' || mode === 'table' ? '' : undefined}>
  {#if mode === 'empty' || mode === 'some' || mode === 'all'}
    {@render fields({ options: flat })}
  {:else if mode === 'locked' || mode === 'grouped'}
    {@render fields({ groups: english })}
  {:else if mode === 'summary'}
    {@render fields({ groups: english, summary: (chosen, total) => nearlyAll(chosen, total, englishAll) })}
  {:else if mode === 'german'}
    {@render fields({ groups: german })}
  {:else if mode === 'readonly'}
    {@render fields({ options: flat, disabledReason: 'Locked by the workspace. Ask an administrator to change it.' })}
  {:else if mode === 'invalid'}
    <Stack gap="200">
      {@render fields({ options: flat, error: isMarked && selected.length === 0 ? 'Choose at least one field. An entry with nothing to show is no use.' : undefined })}
      <div><Button onclick={() => (isMarked = selected.length === 0)}>Save</Button></div>
    </Stack>
  {:else if mode === 'refused'}
    {@render fields({ options: withRefused })}
  {:else if mode === 'dense'}
    <MultiSelect
      label="Levels"
      options={levels}
      {selected}
      onChange={(next) => (selected = next)}
      size="sm"
      emptyLabel="No level"
      filterLabel="Filter the levels"
      clearLabel="Clear the filter"
      noMatchLabel="No level matches."
      allLabel="All"
      noneLabel="None"
      doneLabel="Done"
    />
  {:else if mode === 'inline'}
    <MultiSelect
      label="Views offered"
      hint="At least one: a collection opens in one of them."
      options={views}
      {selected}
      onChange={(next) => (selected = next)}
      error={selected.length === 0 ? 'Offer at least one view.' : undefined}
      emptyLabel="No view"
      filterLabel="Filter the views"
      clearLabel="Clear the filter"
      noMatchLabel="No view matches."
      allLabel="All"
      noneLabel="None"
      doneLabel="Done"
    />
  {:else if mode === 'dialog'}
    <Stack gap="200">
      <Button tone="primary" onclick={() => (isEditing = true)}>Edit the step</Button>
      <Dialog title="Edit the step" bind:isOpen={isEditing} dismissLabel="Close">
        {#snippet actions()}
          <Button onclick={() => (isEditing = false)}>Cancel</Button>
          <Button tone="primary" onclick={() => (isEditing = false)}>Save</Button>
        {/snippet}
        <Stack gap="200">
          <Input label="Title" bind:value={title} />
          {@render fields({ options: flat })}
        </Stack>
      </Dialog>
    </Stack>
  {:else}
    <InContext context={mode === 'table' ? 'table' : 'row'} reason="Switch tasks on first: a work package sits inside one.">
      {#snippet subject({ reason, onclick })}
        <MultiSelect
          label="Levels"
          isLabelHidden
          options={levels.map((each) => (each.value === 'WORK_PACKAGE' ? { ...each, disabledReason: reason } : each))}
          {selected}
          onChange={onclick}
          size="sm"
          emptyLabel="No level"
          filterLabel="Filter the levels"
          clearLabel="Clear the filter"
          noMatchLabel="No level matches."
          allLabel="All"
          noneLabel="None"
          doneLabel="Done"
        />
      {/snippet}
    </InContext>
  {/if}
</div>

<style>
  /* A setting's column is about this wide; the field fills what it is given. */
  .demo { max-inline-size: 40ch; }
  .demo[data-wide] { max-inline-size: none; }
</style>
