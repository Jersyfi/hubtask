<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Comfortable | Compact* for a calm table (WB-4), in the toolbar: the segmented control every
  // product's list offers, kept on the device through `density.ts` under the list's own key, so a
  // product never builds it again. Not drawn on a phone, where a list is stacked rows and has no
  // density to choose.
  import { untrack } from 'svelte';

  import ViewSwitcher from './ViewSwitcher.svelte';
  import { readDensity, writeDensity, type DensityStore, type ListDensity } from './density.ts';

  interface Props {
    /** The group's name: *Density*. */
    label: string;
    comfortableLabel: string;
    compactLabel: string;
    density?: ListDensity;
    /** The list's key on this device; without one the choice is the caller's alone. */
    list?: string;
    /** Where the device keeps it; `localStorage` by default. */
    store?: DensityStore;
    onchange?: (density: ListDensity) => void;
  }

  const deviceStore = (): DensityStore | undefined => {
    try {
      return typeof localStorage === 'undefined' ? undefined : localStorage;
    } catch {
      return undefined;
    }
  };

  let { label, comfortableLabel, compactLabel, density = $bindable('comfortable'), list, store = deviceStore(), onchange }: Props = $props();

  untrack(() => {
    if (list) density = readDensity(store, list);
  });

  const views = $derived([
    { id: 'comfortable', label: comfortableLabel },
    { id: 'compact', label: compactLabel },
  ]);

  function choose(id: string) {
    const next: ListDensity = id === 'compact' ? 'compact' : 'comfortable';
    density = next;
    if (list) writeDensity(store, list, next);
    onchange?.(next);
  }
</script>

<span class="density"><ViewSwitcher {label} {views} selected={density} onselect={choose} /></span>

<style>
  .density { display: inline-flex; }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .density { display: none; }
  }
</style>
