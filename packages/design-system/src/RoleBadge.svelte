<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A role, and where it was granted.
  //
  // **The two travel together, and that is the whole reason this exists.** A role applies downwards
  // from its scope — tenant, hub, collection, or one entry (`domain-model.md` §3.2) — so `ADMIN`
  // says nothing until it says *where*. A badge that showed the role alone would produce exactly
  // the misreading `PermissionMatrix` is built to prevent: somebody reading "Admin" beside a name
  // in a collection and understanding it as the workspace.
  //
  // **There is no colour per role, deliberately.** `Badge`'s tones mean something — a state that is
  // fine, one that is not, one that failed — and a role is not a state. Seven roles would need
  // seven colours the palette does not have, invented here, meaningless to a first-time reader and
  // wrong the day the installation adds an eighth. What distinguishes the roles is the word, which
  // is why it is the word that is emphasised and the scope that is quiet beside it.
  //
  // **Neither string is written here.** The role's name and the phrase for where it was granted are
  // both resolved text (ADR-0011) — including the grammar that joins them, which is the catalogue's
  // job and not this component's: "in Marketing" reads one way in English and another in every
  // language with cases, and a component that inserted a preposition would be a component writing
  // a sentence.

  import type { ControlSize } from './control.ts';

  interface Props {
    /** The role as this installation words it. Resolved text — never the enum value. */
    label: string;
    /**
     * Where the grant was made, as a phrase the caller resolved: "in Marketing", "this workspace",
     * "on this entry". Absent where the scope is not in question — a list that is already one
     * collection's members says it once, in its heading, rather than on every row.
     */
    scopeLabel?: string;
    size?: ControlSize;
  }

  const { label, scopeLabel, size = 'md' }: Props = $props();
</script>

<span class="role" data-size={size}>
  <span class="name">{label}</span>
  {#if scopeLabel}
    <!-- A rule rather than a character: a `·` between the two would be read out by a screen
         reader as punctuation in the middle of a phrase, and would need a direction in RTL that a
         border does not. -->
    <span class="scope">{scopeLabel}</span>
  {/if}
</span>

<style>
  .role {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-025) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    font-size: var(--fs-075);
    min-width: 0;
  }

  .role[data-size='sm'] { padding: 0 var(--sp-050); }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  .scope {
    padding-inline-start: var(--sp-100);
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-secondary);
    min-width: 0;
    overflow-wrap: anywhere;
  }
</style>
