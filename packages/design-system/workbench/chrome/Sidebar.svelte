<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The index. It groups by the segment before the slash in a story's title, which is how
  // `design-system.md` §4's waves reach the sidebar without anybody maintaining a second list.
  import type { LoadedStory, StoryGroup } from '../lib/story.ts';

  interface Props {
    groups: readonly StoryGroup[];
    selected: string | null;
    onselect: (id: string) => void;
  }

  const { groups, selected, onselect }: Props = $props();

  const waves = $derived.by(() => {
    const byWave = new Map<string, StoryGroup[]>();
    for (const group of groups) {
      byWave.set(group.group, [...(byWave.get(group.group) ?? []), group]);
    }
    return [...byWave.entries()];
  });

  const label = (group: StoryGroup, story: LoadedStory) =>
    group.stories.length === 1 && story.name === group.title ? group.title : story.name;
</script>

<nav class="sidebar" aria-label="Stories">
  {#each waves as [wave, members] (wave)}
    <h2>{wave}</h2>
    {#each members as group (group.meta.title)}
      <div class="group">
        <div class="group-head">
          <span class="group-title">{group.title}</span>
          <span class="status" data-status={group.meta.status}>{group.meta.status}</span>
        </div>
        <ul>
          {#each group.stories as story (story.id)}
            <li>
              <button
                type="button"
                class="story"
                aria-current={story.id === selected ? 'true' : undefined}
                onclick={() => onselect(story.id)}
              >
                {label(group, story)}
              </button>
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  {:else}
    <p class="empty">
      No stories yet. <code>src/</code> stays empty until wave 1 builds it deliberately — see
      <code>src/README.md</code>.
    </p>
  {/each}
</nav>

<style>
  .sidebar {
    overflow-y: auto;
    padding: var(--sp-200);
    border-inline-end: 1px solid var(--border-subtle);
    background: var(--bg-surface);
  }

  h2 {
    margin: var(--sp-300) 0 var(--sp-100);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    font-weight: var(--fw-text);
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-subtle);
  }

  h2:first-child {
    margin-top: 0;
  }

  .group {
    margin-bottom: var(--sp-150);
  }

  .group-head {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-050) var(--sp-100);
  }

  .group-title {
    font-weight: var(--fw-semibold);
  }

  /* Rule 3: colour never stands alone. The status is the word, and the tint only repeats it. */
  .status {
    margin-inline-start: auto;
    padding: 0 var(--sp-050);
    border-radius: var(--r-sm);
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    letter-spacing: 0.06em;
    text-transform: uppercase;
    background: var(--label-slate-bg);
    color: var(--label-slate-fg);
  }

  .status[data-status='draft'] {
    background: var(--label-amber-bg);
    color: var(--label-amber-fg);
  }

  .status[data-status='stable'] {
    background: var(--label-green-bg);
    color: var(--label-green-fg);
  }

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .story {
    display: block;
    width: 100%;
    padding: var(--sp-050) var(--sp-100);
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-secondary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .story:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  .story[aria-current='true'] {
    background: var(--accent-primary-subtle);
    color: var(--text-brand);
    font-weight: var(--fw-semibold);
  }

  .empty {
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  code {
    font-family: var(--font-mono);
  }
</style>
