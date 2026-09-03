<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An empty list, and which of the three emptinesses it is.
  //
  // `voice-and-tone.md` §4 is unusually specific here and the component is shaped by it rather
  // than by a picture: an empty list has **three** causes, they mean different things, and one
  // sentence cannot serve all three.
  //
  //   §4.1 nothing has been made yet    - say what this place is for, offer the one action
  //   §4.2 a filter excluded everything - say the filter did it, offer to widen it
  //   §4.3 the emptiness is the good outcome - state it plainly, offer nothing
  //
  // So `kind` is a required prop with no default. A default would be a component quietly choosing
  // one of three meanings on a caller's behalf, and §4.2 names exactly what that costs: "offering
  // to create something when eleven things exist and are hidden is an answer to a question nobody
  // asked."
  //
  // **A failure is not an empty state** (§4.4) - "a failure rendered as 'no results' is a lie the
  // reader acts on" - so there is no `kind` for it here. That is `ErrorState`, and the separation
  // is the rule made structural rather than reviewed.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';

  /** Which of §4's three. Named for the cause, because that is what decides the words. */
  export type Emptiness = 'unused' | 'filtered' | 'settled';

  interface Props {
    kind: Emptiness;
    /** One sentence. Resolved text (ADR-0011), and §4.5: it does not apologise. */
    title: string;
    /** At most one more. Absent is the common case for `settled`. */
    description?: string;
    icon?: IconName;
    /**
     * The one thing to do about it.
     *
     * Refused on `settled`, because §4.3 says to offer nothing there - a queue that has drained
     * does not want a button. It is dropped rather than typed away because the kind is often a
     * value rather than a literal at the call site, and a type could not catch it there.
     */
    action?: Snippet;
  }

  const { kind, title, description, icon, action }: Props = $props();

  const MARK: Record<Emptiness, IconName> = {
    unused: 'plus',
    filtered: 'funnel',
    settled: 'check',
  };

  const offersAction = $derived(kind !== 'settled' && action !== undefined);
</script>

<div class="empty" data-kind={kind}>
  <span class="mark" aria-hidden="true"><Icon name={icon ?? MARK[kind]} /></span>
  <p class="title">{title}</p>
  {#if description}<p class="description">{description}</p>{/if}
  {#if offersAction}<div class="action">{@render action?.()}</div>{/if}
</div>

<style>
  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--sp-150);
    /* Generous, because an empty list has the room and a cramped empty state reads as an error. */
    padding: var(--sp-600) var(--sp-300);
    text-align: center;
  }

  .mark { display: inline-flex; color: var(--text-subtle); }

  /* Rule 3: the three kinds are not told apart by colour. They differ in their mark, their words
     and whether they offer anything, and the mark is what carries it in greyscale. */
  .empty[data-kind='settled'] .mark { color: var(--text-success); }

  .title {
    margin: 0;
    max-width: 48ch;
    color: var(--text-primary);
    font-size: var(--fs-200);
    font-weight: var(--fw-medium);
  }

  .description {
    margin: 0;
    max-width: 56ch;
    color: var(--text-secondary);
    font-size: var(--fs-100);
  }

  .action { margin-top: var(--sp-100); }
</style>
