<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A form or a section of writes on the workspace, behind the permission its write asks.
  //
  // The administration is read by people who may not change it - the auditor reads every setting
  // and changes none (audit.md §5) - so what is configured stays on the screen, dimmed and inert,
  // with the reason beneath it, rather than disappearing: the reading is the auditor's whole job.
  // The verdict is the manifest's `workspace_permissions`, and nothing is offered before it arrives.

  import type { Snippet } from 'svelte';

  import { CapabilityGate } from '@hubtask/design-system/components';

  import { mayHere, workspaceRefusal } from '../data/capability.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The column of the role matrix the write behind this asks on the server. */
    permission: string;
    children: Snippet;
  }

  const { permission, children }: Props = $props();

  const verdict = $derived(mayHere(permission));
</script>

<CapabilityGate
  status={verdict.status}
  reason={workspaceRefusal(permission)}
  pendingLabel={t('app.gate.deciding')}
>
  {@render children()}
</CapabilityGate>
