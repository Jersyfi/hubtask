<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper. Every string this component draws is resolved text, so the demo is
  // where they are written — and the value is a fixture, because a component that generated one
  // would be a component minting a secret.

  import OneTimeSecret from './OneTimeSecret.svelte';

  const { mode = 'token' }: { mode?: 'token' | 'acknowledge' | 'plain' } = $props();

  // Deliberately unmistakable, and that is not only politeness to a reader. A fixture shaped like
  // a real credential is a fixture the secret scanner reports as a leak — it did, on the first
  // push of this component — and a scanner taught to ignore a path is a scanner that ignores the
  // next thing in it.
  const exampleToken = 'hbt_pat_example_value_not_a_real_token';
  const recovery = 'ZP4T-K9WQ · 3MNB-7XRD · 2LFV-8HGS · 6CJY-1AEU · 5DKW-0QNP';
</script>

{#if mode === 'acknowledge'}
  <OneTimeSecret
    value={recovery}
    label="Recovery codes"
    hint="Five codes, each usable once. They are how you get back in when the phone is gone — save them somewhere that is not the phone."
    revealLabel="Show the codes"
    hideLabel="Hide the codes"
    copyLabel="Copy"
    copiedLabel="Copied"
    acknowledgementLabel="I have saved the recovery codes"
    notAcknowledgedReason="Confirm you have saved the codes first. They cannot be shown again."
    dismissLabel="Done"
  />
{:else if mode === 'plain'}
  <OneTimeSecret
    value="hubtask+jumble-4f19@intake.example.org"
    label="Intake address"
    revealLabel="Show the address"
    hideLabel="Hide the address"
  />
{:else}
  <OneTimeSecret
    value={exampleToken}
    label="Personal access token"
    hint="Shown once. There is no call that answers it again — copy it now, and if you lose it, mint another."
    revealLabel="Show the token"
    hideLabel="Hide the token"
    copyLabel="Copy"
    copiedLabel="Copied"
    dismissLabel="Done"
  />
{/if}
