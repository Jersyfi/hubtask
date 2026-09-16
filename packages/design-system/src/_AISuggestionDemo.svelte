<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper: every kind of proposal the server makes, each with the payload the
  // product would render into the slot. The sentences are the application's — resolved message
  // codes, phrased by voice-and-tone.md §7 — and are written out here only because a workbench
  // has no catalogue. The component never writes one.

  import AISuggestion from './AISuggestion.svelte';
  import Button from './Button.svelte';
  import Inline from './Inline.svelte';
  import LabelChip from './LabelChip.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'kinds' }: { mode?: 'kinds' | 'pending' | 'stale' | 'german' } = $props();

  const provenance = 'claude-opus-5 · 8 September 2026, 09:41 · prompt v3';
  const tree = [
    { depth: 0, type: 'Work package', title: 'Collect the figures' },
    { depth: 1, type: 'Activity', title: 'Ask finance for the provisional slide 4 numbers' },
    { depth: 1, type: 'Activity', title: 'Reconcile against last quarter' },
    { depth: 0, type: 'Work package', title: 'Write the narrative' },
  ];
  const provenanceLabel = 'Where this came from';
</script>

{#snippet decide(verb: string)}
  <Button tone="primary">{verb}</Button>
  <Button tone="subtle">Dismiss</Button>
{/snippet}

{#snippet askAgain()}
  <Button tone="secondary">Ask again</Button>
  <Button tone="subtle">Dismiss</Button>
{/snippet}

{#if mode === 'pending'}
  <AISuggestion heading="Suggested breakdown" state="pending" pendingLabel="Suggesting…" />
{:else if mode === 'stale'}
  <AISuggestion
    heading="Suggested title"
    state="stale"
    staleNote="This entry changed since — ask again"
    {provenance}
    {provenanceLabel}
    actions={askAgain}
  >
    <p class="proposed">Prepare the quarterly deck for Thursday’s board meeting</p>
  </AISuggestion>
{:else if mode === 'german'}
  <AISuggestion
    heading="Vorgeschlagene Beschriftungen"
    provenance="claude-opus-5 · 8. September 2026, 09:41 · Prompt v3"
    provenanceLabel="Woher dieser Vorschlag stammt"
  >
    <Inline gap="050">
      <LabelChip name="Finanzen" colorToken="teal" />
      <LabelChip name="Vorstandssitzung" colorToken="violet" />
      <LabelChip name="Vierteljährliche Berichterstattung" colorToken="amber" />
    </Inline>
    {#snippet actions()}
      <Button tone="primary">Beschriftungen hinzufügen</Button>
      <Button tone="subtle">Verwerfen</Button>
    {/snippet}
  </AISuggestion>
{:else}
  <Stack gap="200">
    <AISuggestion heading="Suggested title" {provenance} {provenanceLabel}>
      <p class="proposed">Prepare the quarterly deck for Thursday’s board meeting</p>
      <p class="current">Now: <span>deck thursday</span></p>
      {#snippet actions()}{@render decide('Apply')}{/snippet}
    </AISuggestion>

    <AISuggestion heading="Suggested breakdown" {provenance} {provenanceLabel}>
      <!-- A proposed tree is not a list of entries yet, so no TaskRow and no checkbox: the type
           is said in words beside each title, which is rule 3 applied to a shape. -->
      <ul class="tree">
        {#each tree as node (node.title)}
          <li data-depth={node.depth}><span class="type">{node.type}</span>{node.title}</li>
        {/each}
      </ul>
      {#snippet actions()}{@render decide('Create work packages')}{/snippet}
    </AISuggestion>

    <AISuggestion heading="Suggested labels" {provenance} {provenanceLabel}>
      <Inline gap="050">
        <LabelChip name="Finance" colorToken="teal" />
        <LabelChip name="Board" colorToken="violet" />
      </Inline>
      {#snippet actions()}{@render decide('Add labels')}{/snippet}
    </AISuggestion>

    <AISuggestion heading="Summary suggested" {provenance} {provenanceLabel}>
      <p class="proposed">
        Eleven comments over three days. Finance will have slide 4 by Wednesday noon; the narrative is agreed except the outlook paragraph, which Mara wants softer. Nothing blocks Thursday.
      </p>
      {#snippet actions()}{@render decide('Keep as note')}{/snippet}
    </AISuggestion>

    <AISuggestion heading="Read in German" provenance="claude-opus-5 · 8 September 2026, 09:43 · prompt v1 · display only" {provenanceLabel}>
      <p class="proposed">Das Quartalsdeck für die Vorstandssitzung am Donnerstag vorbereiten</p>
      {#snippet actions()}<Button tone="subtle">Show the original</Button>{/snippet}
    </AISuggestion>

    <AISuggestion heading="Suggested template" {provenance} {provenanceLabel}>
      <p class="proposed">Quarterly board deck — 2 work packages, 5 activities, due anchors relative to the meeting</p>
      {#snippet actions()}{@render decide('Create template')}{/snippet}
    </AISuggestion>
  </Stack>
{/if}

<style>
  .proposed { margin: 0; overflow-wrap: anywhere; }
  .tree { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-050); }
  .tree li { display: flex; gap: var(--sp-100); align-items: baseline; overflow-wrap: anywhere; }
  .tree li[data-depth='1'] { padding-inline-start: var(--sp-300); }
  .type { flex: none; color: var(--text-subtle); font-size: var(--fs-075); }
  .current { margin: var(--sp-050) 0 0; color: var(--text-subtle); font-size: var(--fs-075); }
  .current span { text-decoration: line-through; }
</style>
