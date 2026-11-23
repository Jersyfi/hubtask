<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ChoiceCards, { type ChoiceCard, type ChoiceCardGroup } from './ChoiceCards.svelte';

  type Mode = 'question' | 'marked' | 'gated' | 'tiles' | 'blank' | 'refused' | 'german' | 'headed';
  const { mode = 'question' }: { mode?: Mode } = $props();

  let choice = $state('');
  let start = $state('empty');
  // The tile stories start from a chosen template, as the draft's state *template chosen* does;
  // the blank one starts from the empty hub, chosen, so its edge is solid.
  let tile = $state('');
  $effect(() => {
    tile = mode === 'blank' ? 'blank' : mode === 'german' ? 'capa' : 'board';
  });

  const question: ChoiceCard[] = [
    { value: 'convert', label: 'Convert to columns', description: 'These places then run without a workflow: every entry stays in the column that showed its status, done stays done.' },
    { value: 'pause', label: 'Only pause', description: 'Everything stays as it is and appears unchanged when you switch on again.' },
  ];

  const starts = $derived<ChoiceCard[]>([
    { value: 'empty', label: 'Empty hub', description: 'Two columns, nothing else.', icon: 'plus' },
    { value: 'board', label: 'Team board', description: 'A board with four columns and a review step.', icon: 'columns-3' },
    {
      value: 'desk',
      label: 'Service desk',
      description: 'A portal, queues and promised response times.',
      icon: 'inbox',
      ...(mode === 'gated' ? { disabledReason: 'Only for hubs that others may see.' } : {}),
    },
  ]);

  const blank: ChoiceCard = { value: 'blank', label: 'Empty hub', summary: 'The workspace’s default: one collection, nothing else set up.', icon: 'plus', isBlank: true };

  const families = $derived<ChoiceCardGroup[]>([
    {
      label: 'Simple',
      options: [
        { value: 'list', label: 'List', summary: 'Entries in one list, done or not.', icon: 'list' },
        { value: 'tasks', label: 'Tasks', summary: 'Entries with someone responsible and a due date.', icon: 'check' },
        { value: 'flow', label: 'Flow', summary: 'Entries that move through statuses from left to right.', icon: 'columns-3' },
      ],
    },
    {
      label: 'Teams',
      options: [
        { value: 'board', label: 'Team board', summary: 'A board with four columns and a review step, for a team that plans in weeks and shows its work.', icon: 'columns-3', chips: ['Boards', 'Plan'] },
        {
          value: 'desk',
          label: 'Service desk',
          summary: 'A portal, queues and promised response times.',
          icon: 'inbox',
          chips: ['Service desk', 'Portal'],
          ...(mode === 'refused' ? { disabledReason: 'Only for hubs that others may see.' } : {}),
        },
        { value: 'meetings', label: 'Meetings', summary: 'An agenda per meeting, decisions and the tasks they leave.', icon: 'calendar' },
      ],
    },
  ]);

  const german: ChoiceCardGroup[] = [
    {
      label: 'Geschäftsprozesse',
      options: [
        {
          value: 'capa',
          label: 'Abweichungen und Korrekturmaßnahmen (CAPA)',
          summary: 'Abweichungen erfassen, Ursachen untersuchen, Korrektur- und Vorbeugemaßnahmen verfolgen und ihre Wirksamkeit prüfen.',
          icon: 'shield',
          chips: ['Abläufe', 'Formulare', 'Berichte'],
        },
        { value: 'contracts', label: 'Vertragsverwaltung', summary: 'Verträge mit Laufzeiten, Kündigungsfristen und Verantwortlichen.', icon: 'file-text', chips: ['Abläufe'] },
      ],
    },
    {
      label: 'Persönlich und Haushalt',
      options: [
        { value: 'household', label: 'Haushaltsplanung', summary: 'Einkäufe, Termine und wiederkehrende Aufgaben im Haushalt.', icon: 'house' },
      ],
    },
  ];
</script>

{#if mode === 'question'}
  <ChoiceCards label="What should happen to the 17 entries?" options={question} bind:value={choice} />
{:else if mode === 'marked' || mode === 'gated'}
  <ChoiceCards label="Start from" options={starts} bind:value={start} />
{:else if mode === 'german'}
  <ChoiceCards label="Wie soll der Hub beginnen?" options={[{ ...blank, label: 'Leerer Hub', summary: 'Die Vorgabe des Arbeitsbereichs: eine Sammlung, sonst nichts eingerichtet.' }]} groups={german} mark="tick" layout="grid" bind:value={tile} />
{:else if mode === 'headed'}
  <!-- The heading stands above a search and a filter, apart from the tiles; it names the group. -->
  <h2 class="heading">How the hub starts</h2>
  <ChoiceCards label="How the hub starts" isLabelHidden options={[blank]} groups={families} mark="tick" layout="grid" bind:value={tile} />
{:else}
  <ChoiceCards label="How should the hub start?" options={[blank]} groups={families} mark="tick" layout="grid" bind:value={tile} />
{/if}

<style>
  .heading { margin: 0 0 var(--sp-150); font-size: var(--fs-200); font-weight: var(--fw-semibold); }
</style>
