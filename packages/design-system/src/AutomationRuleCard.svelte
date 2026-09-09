<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One rule, as somebody scanning a list of them needs it.
  //
  // **A card is not an editor.** It renders no condition expression and no action parameters: what
  // a rule *does* in detail is a screen of its own, and a card that tried to show it would be a
  // card that is unreadable for the nine rules either side of it. What belongs here is what
  // somebody is scanning for — which rule this is, what starts it, how much it does, whether it is
  // on, and whose rights it acts with.
  //
  // **`run_as` is on the card rather than in the detail**, because it is the answer to "what can
  // this rule reach". A rule can never do more than the account it runs as (`automation.md` §2),
  // so the account is half of what the rule means — the same reasoning that puts the scope on
  // `RoleBadge`.
  //
  // **The failure count is shown before it becomes a surprise.** Consecutive failures disable a
  // rule by themselves, and a rule that switched itself off overnight with no warning anywhere is
  // the support thread this line exists to prevent. The sentence is the caller's, because how many
  // are left is arithmetic with a plural in it and plurals belong to the client's renderer.

  import Badge from './Badge.svelte';
  import Icon from './Icon.svelte';
  import Stack from './Stack.svelte';

  /** One labelled fact about the rule. Both halves resolved text (ADR-0011). */
  export interface RuleFact {
    readonly label: string;
    readonly value: string;
  }

  interface Props {
    /** The rule's name. Data the tenant wrote, rendered as text. */
    name: string;
    /** Where the rule's own screen is. Absent leaves the name as plain text. */
    href?: string;
    /** What starts it, in words — never the raw trigger kind. */
    trigger: RuleFact;
    /** How much it does. A resolved count, because a plural is the renderer's job. */
    actions: RuleFact;
    /** The account it acts as. */
    runAs: RuleFact;
    /** Whether it is switched on. */
    isEnabled: boolean;
    /** The word for that state, resolved. */
    stateLabel: string;
    /**
     * The consecutive failures, in the caller's sentence, where there are any. Absent means none —
     * a card that said "0 failures" would be a card drawing attention to nothing.
     */
    failureLabel?: string;
  }

  const { name, href, trigger, actions, runAs, isEnabled, stateLabel, failureLabel }: Props =
    $props();
</script>

<article class="card">
  <Stack gap="150">
    <div class="head">
      <h3 class="name">
        {#if href}
          <a {href}>{name}</a>
        {:else}
          {name}
        {/if}
      </h3>
      <!-- Rule 3 again: `success` carries a tick, so "on" is legible without the colour. Off is
           neutral and carries no mark, because nothing is wrong with a rule somebody switched
           off — it is a state, not a warning. -->
      <Badge tone={isEnabled ? 'success' : 'neutral'}>{stateLabel}</Badge>
    </div>

    <dl class="facts">
      {#each [trigger, actions, runAs] as fact (fact.label)}
        <div class="fact">
          <dt>{fact.label}</dt>
          <dd>{fact.value}</dd>
        </div>
      {/each}
    </dl>

    {#if failureLabel}
      <p class="failures">
        <Icon name="triangle-alert" size="sm" />
        <span>{failureLabel}</span>
      </p>
    {/if}
  </Stack>
</article>

<style>
  .card {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    min-width: 0;
  }

  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .name {
    margin: 0;
    flex: 1 1 auto;
    min-width: 0;
    font-family: var(--font-display);
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    overflow-wrap: anywhere;
  }

  .name a { color: var(--text-primary); }

  .name a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  /* A grid rather than four flex rows: the labels line up, which is what makes a stack of these
     scannable down the column instead of read one card at a time. */
  .facts { margin: 0; display: grid; gap: var(--sp-050); }

  .fact { display: flex; flex-wrap: wrap; gap: var(--sp-100); min-width: 0; }

  .fact dt { color: var(--text-subtle); font-size: var(--fs-075); min-inline-size: 8ch; }

  .fact dd { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); min-width: 0; overflow-wrap: anywhere; }

  .failures {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    color: var(--text-warning);
    font-size: var(--fs-075);
  }
</style>
