<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Which language an entry is written in (`content_language`, i18n-l10n.md §6 line 9).
  //
  // The list is the installation's `text_languages` - what its PostgreSQL can index - and the
  // contract stores a language outside it too: the entry is then found by words rather than by
  // stems, which is complete and is not an error. So the picker restates that tolerance: the
  // installation's languages to choose from, and a typed tag for one it does not list. A tag is
  // data the way a title is - never a message code, never translated.

  import { untrack } from 'svelte';

  import { Input, Select } from '@hubtask/design-system/components';

  interface Props {
    /** The languages the installation indexes, from the manifest. */
    languages: readonly string[];
    /** The tag, or empty for "none stated". Bound. */
    value: string;
    label: string;
    hint?: string;
    otherLabel: string;
    tagLabel: string;
    tagHint: string;
    size?: 'sm' | 'md';
  }

  let { languages, value = $bindable(''), label, hint, otherLabel, tagLabel, tagHint, size = 'md' }: Props = $props();

  /** The choice that means "a language the list does not carry". A value no tag can be. */
  const OTHER = '*';

  // A tag the list does not carry is the "other" choice with the tag typed beside it. Two
  // effects, each reading one side and writing the other only when they disagree, so that a
  // value filled from a read (an entry that arrives after the form) is followed by the fields,
  // and a choice made in the fields is written to the value - and neither loops on the other.
  let chosen = $state('');
  let typed = $state('');

  const produced = () => (chosen === OTHER ? typed.trim() : chosen);

  $effect(() => {
    const outside = value;
    untrack(() => {
      if (produced() === outside) return;
      if (outside !== '' && !languages.includes(outside)) {
        chosen = OTHER;
        typed = outside;
      } else {
        chosen = outside;
        typed = '';
      }
    });
  });
  $effect(() => {
    const next = produced();
    untrack(() => {
      if (value !== next) value = next;
    });
  });

  const options = $derived([
    ...languages.map((tag) => ({ value: tag, label: tag })),
    { value: OTHER, label: otherLabel },
  ]);
</script>

<div class="picker">
  <Select {label} {hint} {size} bind:value={chosen} {options} />
  {#if chosen === OTHER}
    <Input label={tagLabel} hint={tagHint} {size} bind:value={typed} autocomplete="off" spellcheck={false} />
  {/if}
</div>

<style>
  .picker { display: flex; flex-direction: column; gap: var(--sp-100); }
</style>
