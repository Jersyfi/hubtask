<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where a form's sections go (page-system §6, §7.7). In a dialog: one column, labels above the
  // fields, each `FormSection` a titled group, the primary at the dialog's foot. On a page: the
  // page's full width like every other page kind, each section a region of `SettingRow`s - the
  // same look as a settings section, never a narrow column of fields.
  //
  // The place travels to the sections by context, so a section is written once and reads right in
  // either.
  import { setContext, type Snippet } from 'svelte';

  interface Props {
    /** Where the form stands. */
    place: 'dialog' | 'page';
    children: Snippet;
  }

  const { place, children }: Props = $props();

  setContext('form-layout-place', () => place);
</script>

<div class="form-layout" data-place={place}>{@render children()}</div>

<style>
  .form-layout { display: flex; flex-direction: column; min-width: 0; }
  .form-layout[data-place='dialog'] { gap: var(--sp-250); }
  .form-layout[data-place='page'] { gap: var(--sp-300); }
</style>
