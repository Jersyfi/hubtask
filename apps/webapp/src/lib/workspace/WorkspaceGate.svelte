<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An area of writes on the workspace, behind the permission its write asks.
  //
  // The administration is read by people who may not change it - the auditor reads every setting
  // and changes none (audit.md §5, UC-AUD-05 checks 2 and 5) - so an area has two parts. What is
  // configured (`children`) stays on the screen for them, readable at full contrast and inert. What
  // would change it (`actions`: an add, a whole creation form; inside a form or a row, a control
  // wrapped in `WriteOnly`) is absent, not dimmed (P-05). The reason is said once, as a `ReadOnlyNotice` at the top of the first area of
  // its permission on the page (`gates.svelte.ts`), with whom to ask; the areas after it say
  // nothing more (UC-WRK-32 check 4).
  //
  // While the manifest is unread nothing is offered: the area, its actions included, stays in
  // place and inert behind `CapabilityGate`'s pending state, so the verdict arriving moves nothing.
  // Once it is known, `CapabilityGate` is not used - its dimmed column is what this replaces.

  import { onDestroy, setContext, type Snippet } from 'svelte';

  import { CapabilityGate, ReadOnlyNotice } from '@hubtask/design-system/components';

  import { mayHere, workspaceRefusal } from '../data/capability.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { enter, GATE, speaks, type GateState } from './gates.svelte.ts';

  interface Props {
    /** The column of the role matrix the write behind this asks on the server. */
    permission: string;
    /** What a reader without the permission still reads. */
    children?: Snippet;
    /** What changes it - absent without the permission. */
    actions?: Snippet;
  }

  const { permission, children, actions }: Props = $props();

  const verdict = $derived(mayHere(permission));
  // svelte-ignore state_referenced_locally
  const place = enter(permission);
  onDestroy(() => place.leave());
  // A form's own save sits inside the form; it asks this gate through `WriteOnly`.
  setContext<GateState>(GATE, {
    get status() {
      return verdict.status;
    },
  });
</script>

{#if verdict.status === 'permitted'}
  {@render children?.()}
  {@render actions?.()}
{:else if verdict.status === 'undetermined'}
  <CapabilityGate status="undetermined" pendingLabel={t('app.gate.deciding')}>
    {@render children?.()}
    {@render actions?.()}
  </CapabilityGate>
{:else}
  {#if speaks(permission, place.ticket)}
    <ReadOnlyNotice text={`${workspaceRefusal(permission)} ${t('app.gate.ask_owner')}`} />
  {/if}
  {#if children}
    <div class="read" inert data-read-only>{@render children()}</div>
  {/if}
{/if}

<style>
  /* Full contrast: a reader without the permission reads it as anybody does. */
  .read { display: contents; }
</style>
