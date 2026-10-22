<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A control inside a gated area that changes something - a form's save, a row's remove - where
  // it cannot move out to the area's `actions` because it belongs inside the form or the row. It
  // follows the enclosing `WorkspaceGate`: drawn where the reader may change the area, absent where
  // they may not (P-05), and drawn while the verdict is unknown, when the whole area is inert.

  import { getContext, type Snippet } from 'svelte';

  import { GATE, type GateState } from './gates.svelte.ts';

  interface Props {
    children: Snippet;
  }

  const { children }: Props = $props();

  const gate = getContext<GateState | undefined>(GATE);
</script>

{#if gate === undefined || gate.status !== 'refused'}
  {@render children()}
{/if}
