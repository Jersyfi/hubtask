<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The door to the level above the workspaces, and the clock on it (SI-17, ADR-0070 §4).
  //
  // **`admin:tenants` is carried by no session until somebody raises one.** A registered operator
  // raises **their own** for an hour by passing a fresh step-up, which is the deliberate weakening
  // ADR-0070 §4 writes down: only for a registered operator, only after a fresh proof, only for an
  // hour, only on the session that proved it, and written down.
  //
  // **The remaining time is on the screen.** An hour nobody can see the end of is an hour somebody
  // is surprised by — in the middle of suspending a workspace, which is the worst moment for a
  // refusal nobody expected.
  //
  // **Its end returns the reader to the application.** Not a screen that goes quietly dead: the
  // scope is gone, every read here would be refused, and leaving somebody looking at stale numbers
  // would be worse than moving them.
  //
  // **It does not slide.** Nothing here asks for another hour on the reader's behalf: activity
  // extends a session's own horizon and never this, and a second hour is a second decision.

  import type { Snippet } from 'svelte';
  import { untrack } from 'svelte';

  import { Banner, Button } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import { instance } from '../data/instance.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The screen, drawn only while the session is raised. */
    children: Snippet;
    /** Where the reader goes when the hour ends. The router is the frame's, not this file's. */
    onleave: () => void;
  }

  const { children, onleave }: Props = $props();

  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** Whether this reader raised the session here, so that its end is a change rather than a state. */
  let wasElevated = $state(false);

  $effect(() => {
    // `untrack` for the reason every store here records: the clock writes the store and writing it
    // reads it, so an effect that tracked that read would re-run on its own first tick.
    const stop = untrack(() => instance.open());
    return stop;
  });

  const remaining = $derived(instance.remainingSeconds);
  const isElevated = $derived(instance.isElevated);

  $effect(() => {
    if (isElevated) {
      wasElevated = true;
      return;
    }
    // The hour ran out under somebody who was working. Back to the application, because every read
    // on this screen is refused from this moment and a dead dashboard says nothing about why.
    if (wasElevated) onleave();
  });

  /** The clock as a person reads it: minutes while there are minutes, seconds at the end. */
  const clock = $derived(
    remaining >= 60
      ? t('app.instance.remaining_minutes', { minutes: Math.ceil(remaining / 60) })
      : t('app.instance.remaining_seconds', { seconds: remaining }),
  );

  async function raise(): Promise<void> {
    isWorking = true;
    failure = undefined;
    try {
      await instance.elevate();
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }
</script>

{#if isElevated}
  <!-- The clock, above everything the area draws. `aria-live="polite"` and not `assertive`: it
       changes every minute, and a reader working through a list does not want to be interrupted by
       each one. -->
  <div class="clock" aria-live="polite">
    <span class="mark" aria-hidden="true"></span>
    <span>{t('app.instance.elevated', { clock })}</span>
  </div>
  {@render children()}
{:else}
  <!-- No heading of its own: the screen behind this door already has one, and a page with two
       `<h1>`s is a page a screen reader announces twice. -->
  <div class="door">
    <p class="lead">{t('app.instance.locked_title')}</p>
    <p class="prose">{t('app.instance.locked_intro')}</p>
    {#if failure}
      <!-- The server's own sentence: not an operator, no proof, a proof that did not hold. -->
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}
    <div>
      <Button
        tone="primary"
        isBusy={isWorking}
        busyLabel={t('app.instance.raising')}
        onclick={() => void raise()}
      >
        {t('app.instance.raise')}
      </Button>
    </div>
    <p class="prose quiet">{t('app.instance.locked_cost')}</p>
  </div>
{/if}

<style>
  /* A band rather than a banner: it is true of the whole area for as long as it is drawn, and a
     dismissible thing would be a clock somebody can hide from themselves. */
  .clock {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    margin-block-end: var(--sp-200);
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--status-warning-border);
    border-radius: var(--r-md);
    background: var(--status-warning-surface);
    color: var(--text-warning);
    font-size: var(--fs-100);
  }

  .mark {
    inline-size: var(--sp-100);
    block-size: var(--sp-100);
    border-radius: var(--r-full);
    background: var(--text-warning);
    flex: none;
  }

  .door { display: grid; gap: var(--sp-150); }
  .lead { margin: 0; max-inline-size: 60ch; font-weight: var(--fw-medium); }
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { color: var(--text-secondary); font-size: var(--fs-100); }
</style>
