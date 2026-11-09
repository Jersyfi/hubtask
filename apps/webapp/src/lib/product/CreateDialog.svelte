<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one create dialog of every product (user-experience.md §5.2, UX-2): the title field
  // first, the template gallery where the product has templates, the product's own fields, the
  // *In:* chip, and one primary beside *Cancel*. A product fills it; it draws no order of its own.
  import type { Snippet } from 'svelte';

  import { Button, Dialog, Input, Stack } from '@hubtask/design-system/components';

  import FailureNote from '../frame/FailureNote.svelte';
  import PlaceChip from './PlaceChip.svelte';
  import type { CreatePlace } from './home.ts';
  import type { RenderedProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    title: string;
    cancelLabel: string;
    /** The title field: its label and the text typed into it. */
    nameLabel: string;
    name: string;
    maxlength?: number;
    /** The template gallery, with *Blank* as one of its cards; absent where the product has none. */
    gallery?: Snippet;
    /** What the product asks besides the title, between the gallery and the place. */
    children?: Snippet;
    /** Where the object is made; absent where the place is fixed (a child page under its parent). */
    place?: { readonly current: string; readonly fallback?: string; readonly places: readonly CreatePlace[]; readonly onchoose: (id: string) => void };
    failure?: RenderedProblem | string;
    primary: { readonly label: string; readonly busyLabel?: string; readonly disabledReason?: string; readonly onclick: () => void };
    isWide?: boolean;
    onClose?: () => void;
  }

  let {
    isOpen = $bindable(), title, cancelLabel, nameLabel, name = $bindable(), maxlength,
    gallery, children, place, failure, primary, isWide = false, onClose,
  }: Props = $props();
</script>

<!-- A name, a place and what to start from: Dialog M; with a gallery to pick from, L (page-system §5.1). -->
<Dialog bind:isOpen {title} dismissLabel={cancelLabel} size={isWide ? 'l' : 'm'} {onClose}
  primary={{ label: primary.label, busyLabel: primary.busyLabel, disabledReason: primary.disabledReason, onclick: primary.onclick }}>
  <Stack gap="200">
    <Input label={nameLabel} bind:value={name} {maxlength} />
    {#if gallery}{@render gallery()}{/if}
    {#if children}{@render children()}{/if}
    {#if place}
      <PlaceChip current={place.current} fallback={place.fallback} places={place.places} onchoose={place.onchoose} />
    {/if}
    {#if failure}<FailureNote problem={failure} />{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{cancelLabel}</Button>
  {/snippet}
</Dialog>
