<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A dialog, or the same content as a tab of the *Structure* panel (ux-inventory.md §E4).
  //
  // The labels, the fields, the templates, the automation and the people of a collection are its
  // *Structure* tabs; the people are also opened from the head's faces, as a dialog. One component
  // per subject draws both, so a tab and a dialog cannot drift apart: embedded, the content stands
  // in the tab while it is the open one, under the tab's own name, and its actions follow it.

  import type { Snippet } from 'svelte';

  import { Dialog } from '@hubtask/design-system/components';

  interface Props {
    /** Drawn inside a *Structure* tab rather than as a dialog of its own. */
    isEmbedded?: boolean;
    title: string;
    isOpen: boolean;
    dismissLabel?: string;
    onClose?: () => void;
    actions?: Snippet;
    children: Snippet;
  }

  let { isEmbedded = false, title, isOpen = $bindable(false), dismissLabel, onClose, actions, children }: Props = $props();
</script>

{#if isEmbedded}
  {#if isOpen}
    <section class="embedded" aria-label={title}>
      {@render children()}
      {#if actions}<div class="actions">{@render actions()}</div>{/if}
    </section>
  {/if}
{:else}
  <Dialog {title} bind:isOpen {dismissLabel} {onClose} {actions}>
    {@render children()}
  </Dialog>
{/if}

<style>
  .embedded { display: flex; flex-direction: column; gap: var(--sp-300); }
  .actions { display: flex; flex-wrap: wrap; justify-content: end; gap: var(--sp-150); }
</style>
