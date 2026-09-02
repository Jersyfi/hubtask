<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The sheet. Underscored, so check-stories reads it as a part rather than as a component §4
  // owes an entry (ADR-0037).

  import Icon from './Icon.svelte';
  import Stack from './Stack.svelte';
  import { BASE_ICONS, CUSTOM_ICONS, type IconName } from './icons/index.ts';

  interface Props {
    /** `all` is the sheet, `custom` the domain marks alone, `text` an icon inside a sentence. */
    mode?: 'all' | 'custom' | 'text';
    size?: 'sm' | 'md';
  }

  const { mode = 'all', size = 'md' }: Props = $props();

  const base = Object.keys(BASE_ICONS) as IconName[];
  const custom = Object.keys(CUSTOM_ICONS) as IconName[];
</script>

{#if mode === 'text'}
  <Stack gap="200">
    <p class="prose">
      An icon is a word in a sentence: it takes the colour of the text around it
      <Icon name="tag" size="sm" /> and the size of it, so a <Icon name="bell" size="sm" /> reminder
      sits on the same line as everything else without pushing it apart.
    </p>
    <p class="prose muted">
      In muted text it is muted too <Icon name="clock" size="sm" />, because nothing here names a
      colour — that is what <code>currentColor</code> buys.
    </p>
    <p class="prose danger">
      And in a danger message it is the danger colour <Icon name="triangle-alert" size="sm" />.
    </p>
  </Stack>
{:else}
  <Stack gap="300">
    {#if mode !== 'custom'}
      <Stack gap="150">
        <h3 class="heading">Base — Lucide, {base.length} declared</h3>
        <div class="sheet">
          {#each base as name (name)}
            <figure class="cell">
              <Icon {name} {size} />
              <figcaption>{name}</figcaption>
            </figure>
          {/each}
        </div>
      </Stack>
    {/if}

    <Stack gap="150">
      <h3 class="heading">Ours — {custom.length} domain marks</h3>
      <div class="sheet">
        {#each custom as name (name)}
          <figure class="cell">
            <Icon {name} {size} />
            <figcaption>{name}</figcaption>
          </figure>
        {/each}
      </div>
    </Stack>
  </Stack>
{/if}

<style>
  .heading {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    text-transform: uppercase;
  }

  .sheet {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(14ch, 1fr));
    gap: var(--sp-100);
  }

  .cell {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    margin: 0;
    padding: var(--sp-100);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
  }

  figcaption {
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    overflow-wrap: anywhere;
  }

  .prose {
    max-width: 56ch;
    margin: 0;
    color: var(--text-primary);
    font-size: var(--fs-100);
    line-height: var(--lh-normal);
  }

  .prose.muted { color: var(--text-subtle); }
  .prose.danger { color: var(--text-danger); }

  code {
    font-family: var(--font-mono);
    font-size: var(--fs-075);
  }
</style>
