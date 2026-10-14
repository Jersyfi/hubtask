<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one sentence a screen of per-row writes says where the reader may not make them.
  //
  // `WorkspaceGate`'s counterpart for a screen with no form to dim: a list whose rows each carry a
  // switch or a remove. The rows' controls are absent where refused, and this says why once, at the
  // head of the screen, rather than the same sentence under every row - a refusal hidden with no
  // word is the silence `apps/webapp/AGENTS.md` forbids. Nothing while the manifest is unread or
  // the permission is held.

  import { Banner } from '@hubtask/design-system/components';

  import { mayHere } from '../data/capability.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The column of the role matrix the screen's writes ask on the server. */
    permission: string;
  }

  const { permission }: Props = $props();

  const verdict = $derived(mayHere(permission));
</script>

{#if verdict.status === 'refused'}
  <Banner tone="info">{t(verdict.code, verdict.params)}</Banner>
{/if}
