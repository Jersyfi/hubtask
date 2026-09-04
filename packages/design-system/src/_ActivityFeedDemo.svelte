<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ActivityFeed, { type ActivityStep } from './ActivityFeed.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'history' }: { mode?: 'history' | 'compact' | 'none' } = $props();

  // Every string below arrives resolved. The component is handed sentences, never codes — which is
  // the whole point of the story.
  const history: ActivityStep[] = [
    {
      id: '1',
      sentence: 'You edited this entry',
      when: '4 Sep 2026, 17:39',
      at: '2026-09-04T15:39:41Z',
      // A note carries that it changed and none of its text. That is the model refusing to put
      // user content where it is not needed, and the feed does not go looking for it.
      changes: [{ field: 'notes', detail: 'changed' }],
    },
    {
      id: '2',
      sentence: 'Someone renamed this entry',
      when: '4 Sep 2026, 17:12',
      at: '2026-09-04T15:12:00Z',
      changes: [{ field: 'title', detail: 'Milk → Oat milk' }],
    },
    {
      id: '3',
      sentence: 'An automation marked this entry as done',
      when: '3 Sep 2026, 09:00',
      at: '2026-09-03T07:00:00Z',
      changes: [],
    },
    {
      id: '4',
      sentence: 'Item recurrence skipped',
      when: '2 Sep 2026, 08:00',
      at: '2026-09-02T06:00:00Z',
      changes: [],
    },
  ];

  /** An activity's history: the verb, the actor and the time, and no change set at all. */
  const compact: ActivityStep[] = [
    { id: '1', sentence: 'You created this entry', when: '4 Sep 2026, 17:00', at: '2026-09-04T15:00:00Z', changes: [] },
    { id: '2', sentence: 'You marked this entry as done', when: '4 Sep 2026, 17:05', at: '2026-09-04T15:05:00Z', changes: [] },
  ];

  // A derived rather than a constant: `mode` is a prop, and reading one once in a plain
  // assignment is what the compiler warns about.
  const steps = $derived(mode === 'compact' ? compact : mode === 'none' ? [] : history);
</script>

<Stack gap="200">
  <ActivityFeed
    label="What happened to this entry"
    {steps}
    emptyLabel="Nothing has happened to this entry yet."
  />
</Stack>
