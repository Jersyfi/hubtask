<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A family of templates, as a create dialog and the catalogue draw it.
  import Button from './Button.svelte';
  import TemplateCard from './TemplateCard.svelte';
  import TemplateGallery from './TemplateGallery.svelte';

  const { mode = 'family' }: { mode?: 'family' | 'long' | 'choice' } = $props();
</script>

{#if mode === 'family'}
  <TemplateGallery label="Service">
    <TemplateCard name="ITIL service desk" summary="Incidents, requests and changes with their SLAs and a portal." products={['Service desk', 'Assets']}>
      {#snippet action()}<Button size="sm" tone="secondary">Take</Button>{/snippet}
    </TemplateCard>
    <TemplateCard name="Customer support" summary="Questions from customers, answered in a queue." products={['Service desk']} note="Update available: 2 → 3" isNoteNews>
      {#snippet action()}<Button size="sm">Update</Button>{/snippet}
    </TemplateCard>
    <TemplateCard name="Internal IT" note="In use" />
  </TemplateGallery>
{:else if mode === 'choice'}
  <TemplateGallery label="Templates">
    {#each [{ name: 'Blank', summary: '' }, { name: 'Retrospective', summary: 'Went well, to improve, actions' }, { name: 'Brainstorm', summary: 'A topic with ideas around it' }] as { name, summary }, i (name)}
      <TemplateCard {name} {summary} isChosen={i === 1}>
        {#snippet preview()}
          <svg viewBox="0 0 120 60" width="100%" height="100%"><rect x={10 + i * 30} y="10" width="30" height="40" rx="3" fill="currentColor" opacity="0.3" /></svg>
        {/snippet}
        {#snippet action()}<Button size="sm" tone={i === 1 ? 'primary' : 'secondary'} aria-pressed={i === 1}>{i === 1 ? 'Chosen' : 'Choose'}</Button>{/snippet}
      </TemplateCard>
    {/each}
  </TemplateGallery>
{:else}
  <TemplateGallery label="Vorlagen für Dienstleistungen und Kundenbetreuung">
    <TemplateCard name="Einkaufs-, Urlaubs- und Reisegenehmigungen" summary="Anträge, die eine Zustimmung brauchen, mit ihren Genehmigungsstufen und Fristen." products={['Servicedesk', 'Zeiterfassung', 'Ressourcen']}>
      {#snippet action()}<Button size="sm" tone="secondary">Übernehmen</Button>{/snippet}
    </TemplateCard>
  </TemplateGallery>
{/if}
