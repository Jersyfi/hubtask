<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A short code, typed into one field that is drawn as its places.
  //
  // **One native input, always.** The familiar alternative - one box per digit, each stealing the
  // focus from the last - breaks the three things a person actually does with a code: pasting it,
  // correcting it with Backspace, and hearing it read. A screen reader on six inputs announces
  // "edit, 1 of 6" six times and never the code; this announces one field with one name. The
  // places below are a *picture* of that field: `aria-hidden`, inert, and painted behind it.
  //
  // The caret is drawn rather than shown, because a caret in a transparent field sits between the
  // places instead of in one. The place being typed carries the accent *and* the caret - rule 3
  // again: the state is never colour alone.
  //
  // Six or eight, because the contract allows both (`SignInCompletion.code`, 6..8) and a TOTP code
  // is six while a recovery code may be eight. The group is where a human eye breaks a number,
  // which is where an authenticator app breaks it too.

  import Field from './_Field.svelte';

  interface Props {
    label: string;
    hint?: string;
    error?: string;
    isRequired?: boolean;
    /** Ids of anything else that describes the control, appended to the field's own. */
    describedBy?: string;
    /** How many places. Six for an authenticator's code, eight for a recovery code. */
    length?: 6 | 8;
    /** Where the eye breaks the number. `0` draws one run. */
    groupOf?: number;
    value?: string;
    name?: string;
    autocomplete?: 'one-time-code';
  }

  let {
    label,
    hint,
    error,
    isRequired = false,
    describedBy,
    length = 6,
    groupOf = 3,
    value = $bindable(''),
    name,
    autocomplete = 'one-time-code',
  }: Props = $props();

  // Places, not characters: what is typed is kept as it is, and the picture shows what fits.
  const places = $derived(Array.from({ length }, (_, index) => index));
  const characters = $derived([...value]);
  const cursor = $derived(Math.min(characters.length, length - 1));
  const isFull = $derived(characters.length >= length);
</script>

<Field {label} {hint} {error} {isRequired} {describedBy}>
  {#snippet children({ id, describedBy: described, invalid })}
    <div class="code" data-invalid={invalid ? '' : undefined}>
      <input
        {id}
        {name}
        {autocomplete}
        class="native"
        type="text"
        inputmode="numeric"
        maxlength={length}
        spellcheck="false"
        autocapitalize="off"
        autocorrect="off"
        bind:value
        required={isRequired}
        aria-invalid={invalid ? 'true' : undefined}
        aria-describedby={described}
      />
      <span class="places" aria-hidden="true">
        {#each places as place (place)}
          {#if groupOf > 0 && place > 0 && place % groupOf === 0}
            <span class="gap"></span>
          {/if}
          <span
            class="place"
            data-filled={characters[place] !== undefined ? '' : undefined}
            data-cursor={!isFull && place === cursor ? '' : undefined}
          >
            {characters[place] ?? ''}
          </span>
        {/each}
      </span>
    </div>
  {/snippet}
</Field>

<style>
  .code {
    position: relative;
    display: inline-flex;
    border-radius: var(--r-md);
  }

  /* On top and transparent: it takes every click, every key and every paste, and shows nothing.
     Never `display: none` and never `aria-hidden` - it *is* the control. */
  .native {
    position: absolute;
    inset: 0;
    width: 100%;
    border: 0;
    background: transparent;
    color: transparent;
    caret-color: transparent;
    font: inherit;
    /* The one place a length in `ch` is right: it is a measure in characters, which is what this
       field is made of, and no token can express "as wide as the picture behind me". */
    letter-spacing: 1ch;
    text-align: center;
  }

  .native:focus { outline: none; }
  /* The ring belongs to the picture, which is what a person sees. */
  .native::selection { background: var(--accent-primary-subtle); }

  .places {
    display: flex;
    align-items: flex-end;
    /* Wide enough that two underlines read as two places. At four pixels they joined into one
       long rule and the field looked like a line with a break in it. */
    gap: var(--sp-100);
    /* A picture of a control never takes the click meant for it. */
    pointer-events: none;
  }

  .gap { inline-size: var(--sp-200); flex: none; }

  .place {
    display: flex;
    align-items: center;
    justify-content: center;
    /* A fixed measure rather than a minimum: the places are a picture of one field and a place
       that shrank to its (empty) content would draw an underline of a different length than the
       one beside it. */
    inline-size: var(--density-control-md-min);
    block-size: var(--density-control-md-min);
    flex: none;
    border-block-end: var(--bw-thick) solid var(--border-default);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--fs-400);
    font-variant-numeric: tabular-nums;
    line-height: var(--lh-tight);
  }

  .place[data-cursor] { border-block-end-color: var(--accent-primary); }

  /* The caret, drawn: a place that is waiting shows where the next character lands. Opacity only,
     so rule 6 holds and a reduced-motion preference can stop it without changing the layout. The
     `pending` role, because a caret is the one thing on this screen that never arrives - it waits
     for as long as somebody is typing. `step-end` rather than the role's easing: a caret blinks,
     it does not fade, and an eased caret reads as a fault. */
  .place[data-cursor]::after {
    content: '';
    width: var(--bw-thick);
    height: var(--fs-300);
    background: var(--text-primary);
    animation: caret var(--motion-pending-duration) step-end infinite;
  }

  .code[data-invalid] .place { border-block-end-color: var(--text-danger); }

  .code:has(.native:focus-visible) .places {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-sm);
  }

  @keyframes caret {
    50% { opacity: 0; }
  }

  @media (prefers-reduced-motion: reduce) {
    .place[data-cursor]::after { animation: none; }
  }

  :global([data-motion='reduced']) .place[data-cursor]::after { animation: none; }
</style>
