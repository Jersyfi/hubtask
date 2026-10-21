<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one reason a composite control states for all of its parts (design-system.md §4).
  //
  // Underscored: a part, drawn in a composite's heading - its legend - by `DueDateControl`,
  // `RecurrenceEditor`, `ReminderEditor`, `CustomFieldRenderer`, `UploadField` and
  // `AssigneeControl`. The composite registers the reason with `provideReasonGroup` (reason.ts),
  // so each part that is off for the same reason draws nothing of its own and points at this one;
  // a reason said up to ten times becomes one.
  //
  // The mark is always drawn and only seen while the composite is unavailable, so the heading has
  // the same size either way. The bubble comes from the mark, and shows whenever a part of the
  // composite is hovered, focused or tried; the guard stands over every part at once, in the
  // capture phase on the composite, so a part's own handler never hears a try.

  import Reason from './_Reason.svelte';
  import Icon from './Icon.svelte';

  interface Props {
    /** The id the parts' `aria-describedby` names - the one handed to `provideReasonGroup`. */
    id: string;
    /** The reason, or nothing while the composite is available. */
    text: string | undefined;
    /** The composite: what its parts' hovers, focus and tries reach. */
    container: HTMLElement | null;
  }

  const { id, text, container }: Props = $props();

  let mark = $state<HTMLElement | null>(null);
</script>

<span class="mark" data-reason-mark="" data-off={text === undefined ? '' : undefined} aria-hidden="true" bind:this={mark}>
  <Icon name="ban" size="sm" />
</span>
{#if text !== undefined}
  <Reason
    {id}
    {text}
    control={container}
    anchor={mark}
    guardOptions={{ isPressRefused: true, isEditRefused: true, isEveryKeyRefused: true }}
  />
{/if}

<style>
  .mark {
    display: inline-flex;
    vertical-align: middle;
    margin-inline-start: var(--sp-050);
    color: var(--text-subtle);
  }

  .mark[data-off] { visibility: hidden; }
</style>
