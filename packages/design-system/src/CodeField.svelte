<!-- SPDX-License-Identifier: Apache-2.0
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
  // **Focus is shown on the place, not around the picture.** A ring around the whole row is a
  // rectangle whose contents are six short rules and five gaps, and it reads as a box drawn around
  // an invisible field - which is what it is, because the field it belongs to is transparent. The
  // ring goes where the next character will land instead, and it is the same ring every other
  // control in this system draws, on a target of the same size. That answers "am I in it" and
  // "where am I in it" with one mark.
  //
  // **The accent and the caret arrive with the focus and leave with it.** An unfocused field that
  // already shows a blinking caret in an accented place is a field claiming to be typed into while
  // somebody is reading a different part of the screen.
  //
  // Six or eight, because the contract allows both (`SignInCompletion.code`, 6..8): an
  // authenticator's code is six digits, and some authenticators are set to eight. A recovery code
  // is not this field's - it is sixteen letters and digits in four groups, pasted with its dashes,
  // and it needs a text keyboard and no length cut. The group is where a human eye breaks a
  // number, which is where an authenticator app breaks it too.

  import Field from './_Field.svelte';

  interface Props {
    label: string;
    hint?: string;
    error?: string;
    isRequired?: boolean;
    /** Ids of anything else that describes the control, appended to the field's own. */
    describedBy?: string;
    /** How many places. Six for an authenticator's code, eight where an authenticator is set so. */
    length?: 6 | 8;
    /** Where the eye breaks the number. `0` draws one run. */
    groupOf?: number;
    value?: string;
    name?: string;
    autocomplete?: 'one-time-code';
    /**
     * Whether to take the focus when this appears.
     *
     * For the one screen where the code is the only thing being asked for and the reader has just
     * been sent there by the step they completed - a second factor's step two, a step-up prompt.
     * Never on a screen where it is one control among several: taking the focus there moves
     * somebody who did not ask to be moved.
     */
    isAutofocused?: boolean;
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
    isAutofocused = false,
  }: Props = $props();

  let field = $state<HTMLInputElement | undefined>(undefined);

  // The attribute is not used: it is honoured once per document load, so it does nothing for a
  // control that appears on the second step of a screen that never reloads. Focusing the element
  // when it arrives is what actually happens there.
  $effect(() => {
    if (isAutofocused) field?.focus();
  });

  // Places, not characters: what is typed is kept as it is, and the picture shows what fits.
  const places = $derived(Array.from({ length }, (_, index) => index));
  const characters = $derived([...value]);
  const cursor = $derived(Math.min(characters.length, length - 1));
  const isFull = $derived(characters.length >= length);
  // Where the eye belongs. The same place as the caret while there is room, and the last one once
  // the code is complete - so the focus mark never disappears at the moment the field is full.
  const active = $derived(isFull ? length - 1 : cursor);
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
        bind:this={field}
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
            data-active={place === active ? '' : undefined}
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

  /* The accent belongs to the focus, not to the value: a field nobody is typing into draws six
     equal places. */
  .code:has(.native:focus) .place[data-active] { border-block-end-color: var(--accent-primary); }

  /* The caret, drawn: a place that is waiting shows where the next character lands. Opacity only,
     so rule 6 holds and a reduced-motion preference can stop it without changing the layout. The
     `pending` role, because a caret is the one thing on this screen that never arrives - it waits
     for as long as somebody is typing. `step-end` rather than the role's easing: a caret blinks,
     it does not fade, and an eased caret reads as a fault. */
  .code:has(.native:focus) .place[data-cursor]::after {
    content: '';
    width: var(--bw-thick);
    height: var(--fs-300);
    background: var(--text-primary);
    animation: caret var(--motion-pending-duration) step-end infinite;
  }

  .code[data-invalid] .place { border-block-end-color: var(--text-danger); }

  /* The ring on the place the next character lands in. One target, the size of any other control
     in this system, rather than a rectangle around the row. */
  .code:has(.native:focus-visible) .place[data-active] {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-sm);
  }

  @keyframes caret {
    50% { opacity: 0; }
  }

  @media (prefers-reduced-motion: reduce) {
    .code .place[data-cursor]::after { animation: none; }
  }

  :global([data-motion='reduced']) .code .place[data-cursor]::after { animation: none; }
</style>
