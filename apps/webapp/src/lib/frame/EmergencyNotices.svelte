<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // When the frame tells the reader that a private hub of theirs was opened through the emergency
  // access (ADR-0073 §4, UC-ID-16 check 11): once the app has opened, on whatever view it opened
  // at, and only at a moment nothing else holds them (`mayInterrupt`). Several notices are one
  // dialog. Closed without confirming, they wait for the next opening of the app; confirmed, the
  // bar's mark and the hub's row carry them. No view draws a banner for them.
  //
  // The list is read when the app opens and again after a confirmation; no change record is
  // written for an emergency access, so one opened while the app is open is told at the next
  // opening, at the same kind of moment.

  import { untrack } from 'svelte';

  import { layers } from '@hubtask/design-system/components';
  import type { EmergencyNotice } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { mayReadWorkspace } from '../data/capability.svelte.ts';
  import { emergencyNotices } from '../data/emergencynotices.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { onboarding } from '../onboarding/onboarding.svelte.ts';
  import { productTour } from '../product/producttour.svelte.ts';
  import { tour } from '../tour.svelte.ts';
  import { mayInterrupt } from './emergencynotice.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => untrack(() => emergencyNotices.open()));

  /** The notices on the screen, as they were when the dialog opened. */
  let shown = $state<readonly EmergencyNotice[] | undefined>(undefined);
  $effect(() => {
    emergencyNotices.isShown = shown !== undefined;
  });

  /**
   * Moved by what can end a moment the notice waits for - a layer closing, the focus leaving a
   * field - since neither the register of layers nor the focus is reactive.
   */
  let moved = $state(0);
  /** The field the person typed into; the focus leaving it ends the typing. */
  let typedIn: EventTarget | null = null;

  const isWaiting = $derived(emergencyNotices.waiting.length > 0 && shown === undefined);

  // Typing is followed from the start: what was typed before a notice arrived is typing too.
  $effect(() => {
    const typed = (event: Event) => (typedIn = event.target);
    const left = (event: Event) => {
      if (event.target === typedIn) typedIn = null;
    };
    document.addEventListener('input', typed, true);
    document.addEventListener('focusout', left, true);
    return () => {
      document.removeEventListener('input', typed, true);
      document.removeEventListener('focusout', left, true);
    };
  });

  $effect(() => {
    if (!isWaiting) return;
    // A task later: a focus that leaves a field the screen is just removing does so in the middle
    // of a render, where no state may change.
    const move = () => queueMicrotask(() => moved++);
    const events = ['focusout', 'close', 'toggle', 'pointerup', 'keyup'];
    for (const name of events) document.addEventListener(name, move, true);
    return () => {
      for (const name of events) document.removeEventListener(name, move, true);
    };
  });

  function moment() {
    const read = workspace.state.status;
    // The workspace's answer is waited for where it is read; a reader who may not read it - a
    // member of private hubs only, among them - is never asked for it, and no starter opens for them.
    const isAnswered = read === 'ready' || read === 'failed' || mayReadWorkspace() === false;
    return {
      waiting: emergencyNotices.waiting.length,
      isSettled: actor.account !== undefined && isAnswered,
      isLayerOpen: layers.size > 0 || document.querySelector('dialog[open]') !== null,
      isTyping: typedIn !== null && typedIn === document.activeElement,
      isTourOpen: tour.isOpen || productTour.isOpen,
      isStarterOpen: onboarding.starterIsOpen,
    };
  }

  $effect(() => {
    void moved;
    if (!isWaiting || !mayInterrupt(moment())) return;
    // A task later, and asked again: a layer the same change opens - the starter, once the
    // workspace answered - is open by then, and the notice waits for it instead of covering it.
    const later = setTimeout(() => {
      if (shown === undefined && mayInterrupt(moment())) shown = emergencyNotices.waiting;
    }, 0);
    return () => clearTimeout(later);
  });
</script>

{#if shown}
  {#await import('./EmergencyNoticeDialog.svelte') then { default: NoticeDialog }}
    <NoticeDialog
      notices={shown}
      {onnavigate}
      onclose={() => {
        if (shown) emergencyNotices.seen(shown.map((each) => each.id));
        shown = undefined;
      }}
    />
  {/await}
{/if}
