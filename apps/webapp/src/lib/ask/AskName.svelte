<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The name a chip shows for a place or a person, read from the place or the account itself; one
  // this reader cannot read is said by the fallback.
  import { untrack } from 'svelte';

  import { resource } from '../data/resource.svelte.ts';

  interface Props {
    path: string;
    field: string;
    fallback: string;
  }

  const { path, field, fallback }: Props = $props();

  const read = resource<Record<string, unknown>>(untrack(() => path));
  const name = $derived(read.state.status === 'ready' && typeof read.state.data[field] === 'string' ? (read.state.data[field] as string) : fallback);
</script>

{name}
