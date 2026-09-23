<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Banner from './Banner.svelte';
  import PageHeader from './PageHeader.svelte';
  import Stack from './Stack.svelte';
  import ViewSwitcher, { type View } from './ViewSwitcher.svelte';

  const { mode = 'collection' }: { mode?: 'collection' | 'hub' | 'phone' | 'long' } = $props();

  let chosen = $state<string | undefined>(undefined);
  let layout = $state('LIST_COLLAPSED');

  const isLong = $derived(mode === 'long');

  const trail = $derived([
    { id: 'home', label: isLong ? 'Arbeitsbereich' : 'Workspace', href: '/' },
    { id: 'hub', label: isLong ? 'Privat und Familie' : 'Private', href: '/hubs/private' },
    { id: 'collection', label: isLong ? 'Renovierung der Küche im Erdgeschoss' : 'Renovation' },
  ]);

  // The collection's menu, in the three groups the milestone decided: act on the object, set it
  // up, and trash - last and alone. The separators are the groups.
  const menu = $derived([
    { id: 'rename', label: isLong ? 'Umbenennen' : 'Rename', icon: 'pencil' as const },
    { id: 'move', label: isLong ? 'In einen anderen Hub verschieben…' : 'Move to another hub…' },
    { id: 'archive', label: isLong ? 'Archivieren' : 'Archive', icon: 'archive' as const },
    { id: 'up', label: isLong ? 'Nach oben' : 'Move up', icon: 'chevron-up' as const },
    { id: 'down', label: isLong ? 'Nach unten' : 'Move down', icon: 'chevron-down' as const, disabledReason: isLong ? 'Schon die letzte' : 'Already last' },
    { id: 'labels', label: isLong ? 'Labels' : 'Labels', icon: 'tag' as const, hasSeparatorBefore: true },
    { id: 'fields', label: isLong ? 'Eigene Felder' : 'Fields' },
    { id: 'views', label: isLong ? 'Gespeicherte Ansichten' : 'Views' },
    { id: 'templates', label: isLong ? 'Vorlagen' : 'Templates' },
    { id: 'policies', label: isLong ? 'Richtlinien' : 'Policies' },
    { id: 'people', label: isLong ? 'Personen' : 'People', icon: 'users' as const },
    { id: 'trash', label: isLong ? 'In den Papierkorb' : 'Move to the trash', icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true },
  ]);

  const layouts: View[] = [
    { id: 'LIST_COLLAPSED', label: 'List', icon: 'menu' },
    { id: 'KANBAN', label: 'Board', icon: 'bucket' },
    { id: 'TIMELINE', label: 'Timeline', icon: 'calendar' },
  ];
</script>

<Stack gap="300">
  {#if mode === 'hub'}
    <PageHeader
      title="Private"
      subtitle="Everything at home, and the people who help with it."
      breadcrumb={{ trail: trail.slice(0, 2), label: 'Where you are', expandLabel: 'Show every level' }}
      primary={{ label: 'Create a collection', icon: 'plus', onclick: () => (chosen = 'create') }}
      secondary={[{ label: 'Import', icon: 'cloud-upload', onclick: () => (chosen = 'import') }]}
      menu={{ label: 'More for this hub', items: menu.filter((item) => !['labels', 'fields', 'views', 'templates', 'policies'].includes(item.id)), onselect: (id) => (chosen = id) }}
    />
  {:else}
    <PageHeader
      title={isLong ? 'Renovierung der Küche im Erdgeschoss' : 'Renovation'}
      subtitle={isLong ? 'Alles, was vor dem Einzug im September fertig sein muss.' : undefined}
      breadcrumb={{ trail, label: 'Where you are', expandLabel: 'Show every level' }}
      primary={{
        label: isLong ? 'Eintrag anlegen' : 'Create an entry',
        icon: 'plus',
        onclick: () => (chosen = 'create'),
        tour: 'create',
        // The daily way to a template, beside the verb (backlog decision 5); the set-up way is in
        // the page menu below. Folded, this list joins that menu.
        menu: { label: isLong ? 'Weitere Wege' : 'More ways to create', items: [{ id: 'template', label: isLong ? 'Aus einer Vorlage…' : 'From a template…', icon: 'layout-template' }], onselect: (id) => (chosen = id) },
      }}
      secondary={[{ label: isLong ? 'Filtern' : 'Filter', icon: 'funnel', onclick: () => (chosen = 'filter') }]}
      menu={{ label: isLong ? 'Mehr für diese Sammlung' : 'More for this collection', items: menu, onselect: (id) => (chosen = id) }}
      isTitleInBar={mode === 'phone'}
    >
      {#snippet notices()}
        {#if mode === 'collection'}
          <Banner tone="warning" title="Archived above">
            The hub this collection is in was archived, so nothing here can change until it is restored.
          </Banner>
        {/if}
      {/snippet}
      {#snippet views()}
        <ViewSwitcher label="How these entries are shown" views={layouts} bind:selected={layout} />
      {/snippet}
    </PageHeader>
  {/if}
  <p class="state">Chosen: {chosen ?? 'nothing yet'} · layout: {layout}</p>
</Stack>

<style>
  .state { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
</style>
