<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The add at the end of a list and at the foot of a column (design-system.md §11.8): a door into
  // capture, said quietly in the brand's ink, a button's outline only under the pointer - the same
  // in the list and on the board, so it is recognised wherever entries are.

  import { Icon } from '@hubtask/design-system/components';

  interface Props {
    label: string;
    onclick: () => void;
  }

  const { label, onclick }: Props = $props();
</script>

<button type="button" class="add-row" data-opener="add-entry" {onclick}><Icon name="plus" size="sm" />{label}</button>

<style>
  .add-row {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-100);
    border: 0;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-brand);
    font: inherit;
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    cursor: pointer;
  }

  /* Under the pointer a quiet tint of its own ink, no frame: it says "press me" without standing
     up out of the list or the column. */
  .add-row:hover { background: var(--accent-primary-subtle); }

  .add-row:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
