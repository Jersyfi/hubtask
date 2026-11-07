<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import ViewBar, { type View } from './ViewBar.svelte';

  const { mode = 'collection' }: { mode?: 'collection' | 'tool' | 'home' | 'compact' } = $props();

  let views = $state<View[]>(
    untrack(() => mode) === 'tool'
      ? [
          { id: 'schedule', label: 'Schedule', href: '#schedule', icon: 'calendar' },
          { id: 'releases', label: 'Releases', href: '#releases' },
          { id: 'sprints', label: 'Sprints', href: '#sprints', icon: 'repeat' },
        ]
      : untrack(() => mode) === 'home'
        ? [
            { id: 'foryou', label: 'For you', href: '#foryou', count: 4 },
            { id: 'desks', label: 'Desks', href: '#desks' },
            { id: 'reports', label: 'Reports', href: '#reports' },
            { id: 'settings', label: 'Settings', href: '#settings' },
          ]
        : [
            { id: 'board', label: 'Board', href: '#board', icon: 'columns-3' },
            { id: 'sprint', label: 'Sprint 42', href: '#sprint', icon: 'repeat' },
            { id: 'backlog', label: 'Backlog', href: '#backlog', icon: 'square-check' },
            { id: 'list', label: 'List', href: '#list', icon: 'square-check' },
            { id: 'timeline', label: 'Timeline', href: '#timeline', icon: 'calendar' },
            { id: 'time', label: 'Time', href: '#time', icon: 'clock' },
            { id: 'mine', label: 'My open ones', href: '#mine', icon: 'star' },
          ],
  );
  let current = $state(untrack(() => views[0]!.id));

  function reorder(id: string, to: number) {
    const from = views.findIndex((view) => view.id === id);
    const next = [...views];
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved!);
    views = next;
  }
</script>

<ViewBar
  label="Web app views"
  {views}
  {current}
  onnavigate={(id) => (current = id)}
  addLabel={mode === 'collection' ? 'Add a view' : undefined}
  onadd={mode === 'collection' ? () => {} : undefined}
  onreorder={mode === 'collection' ? reorder : undefined}
  isCompact={mode === 'compact'}
  sheetTitle="Views"
  closeLabel="Close"
>
  {#snippet tools()}
    {#if mode === 'collection'}<Button size="sm" tone="secondary" icon="funnel">Filter</Button>{/if}
  {/snippet}
</ViewBar>
<p class="state">On: {current}.</p>

<style>
  .state { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
</style>
