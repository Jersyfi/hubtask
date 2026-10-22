<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A few words about the control under the pointer.
  //
  // **It is a description, not a name.** `aria-describedby`, set on the control itself rather than
  // on the wrapper - a description announced for a wrapper is a description nobody hears. An
  // icon-only control still needs its own name through `VisuallyHidden`; a tooltip that was the
  // name would vanish for anyone who never hovers.
  //
  // **The bubble is the reason's.** `_Reason.svelte` draws it and decides when it shows and what
  // closes it, and the same part says why an unavailable control cannot be used - so a tooltip and
  // a reason cannot disagree about hover, focus, tap, the grace or `Escape`. Here it guards
  // nothing: the control it describes works.
  //
  // **It is not in the layer register.** `layers.ts` says why: a tooltip paints above a dialog and
  // is dismissed by the pointer, the blur or the `Escape` that concerns it, so putting it in the
  // register would let it swallow the `Escape` meant for the dialog underneath.

  import type { Snippet } from 'svelte';

  import Reason from './_Reason.svelte';
  import type { Placement } from './anchor.ts';

  interface Props {
    /** The description. Resolved text (ADR-0011), and short: this is not a place for a paragraph. */
    text: string;
    /** Where it sits. Logical, so it follows the writing direction (ADR-0039). */
    placement?: Placement;
    /** The control being described. */
    children: Snippet;
  }

  const { text, placement = { side: 'block-start', align: 'start' }, children }: Props = $props();

  // Svelte's own id, not a random one: unique per instance and the same on every render, so a
  // prerendered page is identical between builds and a hydrated one matches its markup.
  const instanceId = $props.id();
  const id = `tooltip-${instanceId}`;

  let wrapper = $state<HTMLElement | null>(null);
  const control = $derived((wrapper?.firstElementChild as HTMLElement | null | undefined) ?? null);

  /**
   * The described control is the first element inside the wrapper, and the attribute is set on it
   * rather than rendered here: the control is the caller's markup, and there is no way to hand it
   * a prop from out here. Set for as long as the tooltip exists, not only while it shows.
   */
  $effect(() => {
    const node = control;
    if (!node) return;
    node.setAttribute('aria-describedby', id);
    return () => node.removeAttribute('aria-describedby');
  });
</script>

<span class="wrapper" bind:this={wrapper}>
  {@render children()}
  <Reason {id} {text} {control} {placement} isGuarding={false} />
</span>

<style>
  .wrapper { display: inline-flex; }
</style>
