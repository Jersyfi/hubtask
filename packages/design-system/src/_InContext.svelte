<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A control with a reason where reasons used to break things: in a row of buttons, and in a
  // table row. A story part, shared by every component that takes a reason, and what the
  // workbench layout test in apps/webapp/e2e measures (design-system.md §4).
  //
  // The row is drawn twice - once with the reason, once without - so the test can compare them:
  // a reason takes no space. Two counters say whether trying the unavailable control reached
  // anything: the caller's own handler, and a click handler on the row around it. They are data
  // attributes rather than text, because a number that grows from 9 to 10 would move the page
  // the test is measuring. A driver button switches the subject's state (busy, a gate deciding)
  // where a story needs a change over time; its label never changes, for the same reason.

  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';

  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Inline from './Inline.svelte';
  import Stack from './Stack.svelte';

  interface Subject {
    /** The reason, or nothing in the comparison row. */
    readonly reason: string | undefined;
    /** Counts a call of the caller's own handler. Hand it to the control as its `onclick`. */
    readonly onclick: () => void;
    /** The driver's state: false first, flipped by every press of the driver. */
    readonly isOn: boolean;
  }

  interface Props {
    context: 'row' | 'table';
    reason: string;
    subject: Snippet<[Subject]>;
    /** A button that flips `isOn`, for a story whose subject changes over time. */
    driverLabel?: string;
  }

  const { context, reason, subject, driverLabel }: Props = $props();

  let callerClicks = $state(0);
  let rowClicks = $state(0);
  let isOn = $state(false);

  const onclick = () => {
    callerClicks += 1;
  };

  const countsRowClicks: Attachment<HTMLElement> = (node) => {
    const count = () => {
      rowClicks += 1;
    };
    node.addEventListener('click', count);
    return () => node.removeEventListener('click', count);
  };

  const cases = $derived([
    { id: 'with', reason },
    { id: 'without', reason: undefined },
  ]);
</script>

<div class="contexts" data-caller-clicks={callerClicks} data-row-clicks={rowClicks} data-on={isOn ? '' : undefined}>
  <Stack gap="300">
    {#if driverLabel}
      <div class="driver">
        <Button tone="subtle" size="sm" data-driver="" onclick={() => (isOn = !isOn)}>{driverLabel}</Button>
      </div>
    {/if}
    {#each cases as each (each.id)}
      {#if context === 'row'}
        <div class="row" data-case={each.id} {@attach countsRowClicks}>
          <Inline gap="150" align="center">
            <Button tone="subtle">Cancel</Button>
            {@render subject({ reason: each.reason, onclick, isOn })}
            <Button tone="primary">Save</Button>
            <IconButton icon="ellipsis" label="More actions" />
          </Inline>
        </div>
      {:else}
        <table class="table" data-case={each.id}>
          <tbody>
            <tr {@attach countsRowClicks}>
              <td>Order the tiles</td>
              <td>{@render subject({ reason: each.reason, onclick, isOn })}</td>
              <td class="end"><IconButton icon="ellipsis" label="More actions" size="sm" /></td>
            </tr>
          </tbody>
        </table>
      {/if}
    {/each}
  </Stack>
</div>

<style>
  .table {
    width: 100%;
    border-collapse: collapse;
  }

  .table td {
    padding: var(--sp-100);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    vertical-align: middle;
    text-align: start;
  }

  .table td.end { text-align: end; }
</style>
