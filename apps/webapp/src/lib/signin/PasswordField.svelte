<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where a password is *set* - the four screens that do it, drawn once.
  //
  // Setting a password happens on the invitation, on the reset, in the step a changed rule routes
  // somebody into, and on the profile. Those are four doors with four proofs and one field, and
  // the field is the part that is easy to make four times and hard to keep the same four times.
  //
  // **The rules under it are live, and the server is asked sparingly.** Everything the client can
  // decide is decided at every keystroke; everything only the server knows is asked once the local
  // rules hold, once the typing stops, and once per value (`signinrules.svelte.ts`). A person who
  // is still three characters short never costs an Argon2 comparison.
  //
  // **No second field.** The eye replaces "repeat it": NIST 800-63B's reasoning is that a password
  // you can read is one you do not have to type twice, and two fields catch typing mistakes by
  // making everybody type twice.

  import { Input } from '@hubtask/design-system/components';

  import PasswordRules from './PasswordRules.svelte';
  import { signInRules } from '../data/signinrules.svelte.ts';
  import { evaluate, type PasswordContext, type RuleLine } from '../data/signinrules.ts';
  import type { PasswordProof } from '../data/signinrules.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The password being set. Bound, because the screen submits it. */
    value: string;
    label: string;
    /** Who is setting it, for the rules that are about them rather than about the string. */
    context?: PasswordContext;
    /** The proof this screen holds, which is the one the check presents. */
    proof: PasswordProof;
    /** The server's own sentence for a refusal of this field, where there is one. */
    error?: string;
    /**
     * What to say under the field where the installation does not serve the rules.
     *
     * The screens this replaces each said a sentence of their own - "at least twelve characters" -
     * and on a server without the sign-in surface that sentence is still the best there is. The
     * rules list replaces it where there is one and nothing where there is not.
     */
    hint?: string;
    /** Whether the lines have been evaluated into something the caller can act on. */
    onlines?: (lines: readonly RuleLine[]) => void;
  }

  let { value = $bindable(''), label, context = {}, proof, error, hint, onlines }: Props = $props();

  const listId = `rules-${Math.random().toString(36).slice(2, 9)}`;

  const lines = $derived(
    evaluate(signInRules.password, value, context, {
      violations: signInRules.violationsFor(value),
      isAnswered: signInRules.isAnswered(value),
      isChecking: signInRules.isChecking,
      isUnreachable: signInRules.isUnreachable,
    }),
  );

  /** Whether a rule has refused this password: the field is marked, the list says which one. */
  const isRefused = $derived(lines.some((line) => line.state === 'failed'));

  // The ask, driven by the value rather than by an event: a paste, an undo and a keystroke are the
  // same thing to a rule, and only one of the three is a `keyup`.
  $effect(() => {
    const candidate = value;
    signInRules.check(candidate, context, proof);
  });

  $effect(() => {
    onlines?.(lines);
  });

  // Nothing outlives the field it was typed into.
  $effect(() => () => signInRules.forget());
</script>

<div class="field">
  <Input
    {label}
    {error}
    type="password"
    autocomplete="new-password"
    spellcheck={false}
    bind:value
    describedBy={signInRules.wasRead ? listId : undefined}
    isInvalid={isRefused}
    hint={signInRules.wasRead ? undefined : hint}
    revealLabel={t('app.password.reveal')}
    hideLabel={t('app.password.hide')}
    isRequired
  />
  {#if signInRules.wasRead}
    <!-- Only where the rules were answered: a list built from this client's own defaults would be
         a list that says what *it* thinks rather than what the workspace demands. -->
    <PasswordRules {lines} id={listId} />
  {/if}
</div>

<style>
  .field {
    display: grid;
    gap: var(--sp-100);
  }
</style>
