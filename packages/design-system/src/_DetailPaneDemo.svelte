<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import DetailPane from './DetailPane.svelte';
  import ListRow from './ListRow.svelte';
  import Menu from './Menu.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'beside' }: { mode?: 'beside' | 'layered' | 'long' } = $props();

  const entries = $derived(
    mode === 'long'
      ? ['Fliesen für die Küche bestellen', 'Elektriker für die Steckdosen buchen', 'Arbeitsplatte ausmessen lassen']
      : ['Order the tiles', 'Book the electrician', 'Measure the worktop'],
  );

  let open = $state<number | null>(1);
  let chosen = $state<string | undefined>(undefined);
</script>

<div class="beside">
  <div class="list">
    <Stack gap="050">
      {#each entries as entry, index (entry)}
        <ListRow onactivate={() => (open = index)} isSelected={open === index}>{entry}</ListRow>
      {/each}
    </Stack>
  </div>
  {#if open !== null}
    <DetailPane
      title={entries[open] ?? ''}
      kind={mode === 'long' ? 'Aufgabe' : 'Task'}
      dismissLabel={mode === 'long' ? 'Details schließen' : 'Close the details'}
      pageLabel={mode === 'long' ? 'Als Seite öffnen' : 'Open as a page'}
      pageHref="/items/1"
      onOpenPage={() => (chosen = 'page')}
      onClose={() => (open = null)}
    >
      <Stack gap="200">
        <p class="notes">
          {mode === 'long'
            ? 'Vor dem Fliesen — die Steckdose muss zuerst versetzt sein.'
            : 'Before the tiling — the socket has to be moved first.'}
        </p>
        {#if mode === 'layered'}
          <Menu label="More" items={[{ id: 'a', label: 'Duplicate' }, { id: 'b', label: 'Move to the trash', isDestructive: true }]} onselect={(id) => (chosen = id)}>
            {#snippet trigger(props)}
              <Button tone="secondary" icon="ellipsis" {...props}>Open a menu inside it</Button>
            {/snippet}
          </Menu>
        {/if}
        <p class="state">Chosen: {chosen ?? 'nothing yet'}</p>
      </Stack>
    </DetailPane>
  {/if}
</div>

<style>
  .beside { display: flex; align-items: stretch; gap: var(--sp-300); min-block-size: calc(var(--layout-appbar-height) * 5); }
  .list { flex: 1; min-width: 0; }
  .notes { margin: 0; color: var(--text-secondary); }
  .state { margin: 0; font-size: var(--fs-075); color: var(--text-subtle); }
</style>
