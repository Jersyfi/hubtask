<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The ⋯ of one entry, wherever the entry is drawn (UC-WRK-28 check 1).
  //
  // The items are `entryMenu`'s answer, handed in; this draws the trigger and is what a right click,
  // the context-menu key and a long press open (`entryGestures.ts`), through `open`. One component
  // so every surface names the menu the same way - "Actions for …" - and opens it the same way.

  import { IconButton, Menu, type MenuItem } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The entry's title, for the menu's and the button's name. */
    title: string;
    items: readonly MenuItem[];
    onselect: (id: string) => void;
    /**
     * Called before the menu opens, however it is opened: what a surface that does not hold the
     * entry's level reads it with (D6), so the moves can say whether they are possible.
     */
    onopening?: () => void;
  }

  const { title, items, onselect, onopening }: Props = $props();

  let menu = $state<ReturnType<typeof Menu> | undefined>(undefined);
  let isOpen = $state(false);

  /** Opens it from outside the button, with the first item focused. */
  export function open(index = 0) {
    onopening?.();
    menu?.open(index);
  }

  const label = $derived(t('app.entry_menu.label', { title }));
</script>

{#if items.length > 0}
  <Menu bind:this={menu} bind:isOpen {label} {items} placement={{ side: 'block-end', align: 'end' }} {onselect}>
    {#snippet trigger(props)}
      <IconButton
        icon="ellipsis"
        {label}
        size="sm"
        {...props}
        onclick={() => {
          if (!isOpen) onopening?.();
          (props.onclick as () => void)();
        }}
        onkeydown={(event: KeyboardEvent) => {
          if (!isOpen) onopening?.();
          (props.onkeydown as (event: KeyboardEvent) => void)(event);
        }}
      />
    {/snippet}
  </Menu>
{/if}
