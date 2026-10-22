<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one sentence a screen of per-row writes says where the reader may not make them.
  //
  // `WorkspaceGate`'s counterpart for a screen with no area to gate: a list whose rows each carry a
  // switch or a remove. The rows' controls are absent where refused, and this says why once, at the
  // head of the screen, rather than the same sentence under every row - a refusal hidden with no
  // word would be the silent ignoring `CapabilityGate` exists to prevent (P-05). It is the same
  // notice a gated area draws, and counts as that permission's one notice on the page
  // (`gates.svelte.ts`). Nothing while the manifest is unread or the permission is held.

  import { onDestroy } from 'svelte';

  import { ReadOnlyNotice } from '@hubtask/design-system/components';

  import { mayHere, workspaceRefusal } from '../data/capability.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { enter, speaks } from './gates.svelte.ts';

  interface Props {
    /** The column of the role matrix the screen's writes ask on the server. */
    permission: string;
  }

  const { permission }: Props = $props();

  const verdict = $derived(mayHere(permission));
  // svelte-ignore state_referenced_locally
  const place = enter(permission);
  onDestroy(() => place.leave());
</script>

{#if verdict.status === 'refused' && speaks(permission, place.ticket)}
  <ReadOnlyNotice text={`${workspaceRefusal(permission)} ${t('app.gate.ask_owner')}`} />
{/if}
