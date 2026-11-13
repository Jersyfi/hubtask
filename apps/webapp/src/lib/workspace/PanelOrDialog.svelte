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

  import { Button, Dialog, type DialogAction } from '@hubtask/design-system/components';

  interface Props {
    /** Drawn inside a *Structure* tab rather than as a dialog of its own. */
    isEmbedded?: boolean;
    /** The dialog's rung (page-system §5.1): a list to pick from with search is L. */
    size?: 's' | 'm' | 'l';
    title: string;
    isOpen: boolean;
    dismissLabel?: string;
    onClose?: () => void;
    actions?: Snippet;
    /** The one primary (page-system §5.1): the dialog's own, or last in the embedded section's acts. */
    primary?: DialogAction;
    children: Snippet;
  }

  let { isEmbedded = false, size = 'l', title, isOpen = $bindable(false), dismissLabel, onClose, actions, primary, children }: Props = $props();
</script>

{#if isEmbedded}
  {#if isOpen}
    <section class="embedded" aria-label={title}>
      {@render children()}
      {#if actions || primary}
        <div class="actions">
          {#if actions}{@render actions()}{/if}
          {#if primary}<Button tone={primary.tone ?? 'primary'} isBusy={primary.isBusy} busyLabel={primary.busyLabel} disabledReason={primary.disabledReason} onclick={primary.onclick}>{primary.label}</Button>{/if}
        </div>
      {/if}
    </section>
  {/if}
{:else}
  <Dialog {size} {title} bind:isOpen {dismissLabel} {onClose} {actions} {primary}>
    {@render children()}
  </Dialog>
{/if}

<style>
  .embedded { display: flex; flex-direction: column; gap: var(--sp-300); }
  .actions { display: flex; flex-wrap: wrap; justify-content: end; gap: var(--sp-150); }
</style>
